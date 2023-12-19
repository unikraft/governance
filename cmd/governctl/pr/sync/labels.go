// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package sync

import (
	"context"
	"fmt"
	"os"
	"path"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/spf13/cobra"
	"github.com/waigani/diffparser"
	"kraftkit.sh/cmdfactory"
	kitcfg "kraftkit.sh/config"
	"kraftkit.sh/log"

	"github.com/unikraft/governance/internal/cmdutils"
	"github.com/unikraft/governance/internal/config"
	"github.com/unikraft/governance/internal/ghapi"
	"github.com/unikraft/governance/internal/label"
	"github.com/unikraft/governance/utils"
)

type Labels struct {
	LabelsDir string `long:"labels-dir" usage:"Path to the labels definition directory." default:".github/labels"`
}

func NewLabels() *cobra.Command {
	cmd, err := cmdfactory.New(&Labels{}, cobra.Command{
		Use:   "labels [OPTIONS] ORG/REPO/PRID",
		Short: "Synchronise a pull request's labels",
		Args:  cmdutils.OrgRepoAndPullRequestNumber(),
		Annotations: map[string]string{
			cmdfactory.AnnotationHelpGroup: "pr",
		},
	})
	if err != nil {
		panic(err)
	}

	return cmd
}

func (opts *Labels) Run(ctx context.Context, args []string) error {
	var err error

	ghOrg, ghRepo, ghPrId, err := cmdutils.ParseOrgRepoAndPullRequestArgs(args)
	if err != nil {
		return err
	}

	ghOrigin := fmt.Sprintf("https://github.com/%s/%s.git", ghOrg, ghRepo)

	ghClient, err := ghapi.NewGithubClient(
		ctx,
		kitcfg.G[config.Config](ctx).GithubToken,
		kitcfg.G[config.Config](ctx).GithubSkipSSL,
		kitcfg.G[config.Config](ctx).GithubEndpoint,
	)
	if err != nil {
		return err
	}

	log.G(ctx).
		WithField("pr_id", ghPrId).
		Info("getting pull request details")

	pr, err := ghClient.GetPullRequest(ctx, ghRepo, ghRepo, ghPrId)
	if err != nil {
		return fmt.Errorf("could not get pull request")
	}

	if *pr.State == "closed" {
		return fmt.Errorf("pull request is closed")
	}

	tempDir := kitcfg.G[config.Config](ctx).TempDir
	if tempDir == "" {
		tempDir, err = os.MkdirTemp("", "governctl-pr-sync-labels-*")
		if err != nil {
			return fmt.Errorf("could not create temporary directory: %w", err)
		}

		defer func() {
			os.RemoveAll(tempDir)
		}()
	}

	localRepo := path.Join(tempDir, ghRepo)

	if os.Getenv("GITHUB_ACTIONS") == "yes" {
		localRepo = os.Getenv("GITHUB_WORKSPACE")
	}

	// Check if we have a copy of the repo locally, but equally retrieve a copy if
	// we do not have it so that we can
	if _, err := os.Stat(localRepo); os.IsNotExist(err) {
		log.G(ctx).
			WithField("from", ghOrigin).
			WithField("to", localRepo).
			Infof("cloning git repository")

		if _, err := git.PlainClone(localRepo, false, &git.CloneOptions{
			URL: ghOrigin,
			Auth: &http.BasicAuth{
				Username: kitcfg.G[config.Config](ctx).GithubUser,
				Password: kitcfg.G[config.Config](ctx).GithubToken,
			},
		}); err != nil {
			return fmt.Errorf("could not clone repository: %s", err)
		}
	}

	labels, err := label.NewListOfLabelsFromPath(
		ghClient,
		ghOrg,
		path.Join(localRepo, opts.LabelsDir),
	)
	if err != nil {
		return fmt.Errorf("could not populate repos: %s", err)
	}

	// Retrieve a list of modified files in this PR
	localDiffFile := path.Join(
		tempDir,
		fmt.Sprintf("%s-%d.diff", ghRepo, ghPrId),
	)

	if _, err := os.Stat(localDiffFile); os.IsNotExist(err) {
		log.G(ctx).
			WithField("from", *pr.DiffURL).
			WithField("to", localDiffFile).
			Infof("saving diff")

		if err = utils.DownloadFile(localDiffFile, *pr.DiffURL); err != nil {
			return fmt.Errorf("could not download pull request diff: %s", err)
		}
	}

	log.G(ctx).
		WithField("file", localDiffFile).
		Infof("reading diff")

	d, err := os.ReadFile(localDiffFile)
	if err != nil {
		return fmt.Errorf("could not read diff file diff: %s", err)
	}

	diff, err := diffparser.Parse(string(d))
	if err != nil {
		return fmt.Errorf("could not parse diff from pull request: %s", err)
	}

	var labelsToAdd []string

	for _, f := range diff.Files {
		log.G(ctx).
			WithField("file", f.NewName).
			Info("checking diff")

		// Determine the labels to add based on the changed files
		for _, label := range labels {
			if containsStr(labelsToAdd, label.Name) {
				continue
			}

			if len(f.OrigName) > 0 && label.AppliesTo(ghRepo, f.OrigName) {
				labelsToAdd = append(labelsToAdd, label.Name)
			}

			if !containsStr(labelsToAdd, label.Name) && len(f.NewName) > 0 && label.AppliesTo(ghRepo, f.NewName) {
				labelsToAdd = append(labelsToAdd, label.Name)
			}
		}
	}

	if len(labelsToAdd) > 0 {
		log.G(ctx).
			WithField("repo", ghRepo).
			WithField("pr_id", ghPrId).
			WithField("labels", labelsToAdd).
			Infof("setting labels on pull request")

		if !kitcfg.G[config.Config](ctx).DryRun {
			if err := ghClient.AddLabelsToPr(ctx, ghOrg, ghRepo, ghPrId, labelsToAdd); err != nil {
				return fmt.Errorf("could not add labels to repo: %w", err)
			}
		}
	}

	return nil
}
