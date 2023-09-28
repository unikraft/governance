// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package pr

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	git "github.com/go-git/go-git/v5"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"kraftkit.sh/cmdfactory"
	kitcfg "kraftkit.sh/config"
	"kraftkit.sh/log"

	"github.com/unikraft/governance/internal/cmdutils"
	"github.com/unikraft/governance/internal/config"
	"github.com/unikraft/governance/internal/ghapi"
	"github.com/unikraft/governance/internal/ghpr"
)

type Merge struct {
	ApproverComments   []string `long:"approver-comments" env:"GOVERN_APPROVER_COMMENTS" usage:"Regular expression that an approver writes"`
	ApproverTeams      []string `long:"approver-teams" env:"GOVERN_APPROVER_TEAMS" usage:"The GitHub team that the approver must be a part of to be considered an approver"`
	ApproveStates      []string `long:"approve-states" env:"GOVERN_APPROVE_STATES" usage:"The state of the GitHub approval from the assignee" default:"approve"`
	BaseBranch         string   `long:"base" env:"GOVERN_BASE" usage:"Set the base branch name that the PR will be rebased onto"`
	Branch             string   `long:"branch" env:"GOVERN_BRANCH" usage:"Set the branch to merge into"`
	CommitterEmail     string   `long:"committer-email" short:"e" env:"GOVERN_COMMITTER_EMAIL" usage:"Set the Git committer author's email"`
	CommitterName      string   `long:"committer-name" short:"n" env:"GOVERN_COMMITTER_NAME" usage:"Set the Git committer author's name"`
	IgnoreLabels       []string `long:"ignore-labels" env:"GOVERN_IGNORE_LABELS" usage:"Ignore the PR if it has any of these labels"`
	IgnoreStates       []string `long:"ignore-states" env:"GOVERN_IGNORE_STATES" usage:"Ignore the PR if it has any of these states"`
	Labels             []string `long:"labels" env:"GOVERN_LABELS" usage:"The PR must have these labels to be considered mergable"`
	MinApprovals       int      `long:"min-approvals" env:"GOVERN_MIN_APPROVALS" usage:"Minimum number of approvals required to be considered mergable" default:"1"`
	MinReviews         int      `long:"min-reviews" env:"GOVERN_MIN_REVIEWS" usage:"Minimum number of reviews a PR requires to be considered mergable" default:"1"`
	NoAutoTrailerPatch bool     `long:"no-auto-trailer-patch" env:"GOVERN_NO_AUTO_TRAILE" usage:"Do not apply inferred trailers from mergability check to each commit"`
	NoCheckMergable    bool     `long:"no-check-mergable" env:"GOVERN_NO_CHECK_MERGABLE" usage:"Do not run a check to test whether the PR meets merge conditions"`
	NoConflicts        bool     `long:"no-conflicts" env:"GOVERN_NO_CONFLICTS" usage:"Pull request must not have any conflicts"`
	NoDraft            bool     `long:"no-draft" env:"GOVERN_NO_DRAFT" usage:"Pull request must not be in a draft state"`
	NoRespectAssignees bool     `long:"no-respect-assignees" env:"GOVERN_NO_RESPECT_ASSIGNEES" usage:"Whether the PR's assignees should be not considered approvers even if they are not part of a team/codeowner"`
	NoRespectReviewers bool     `long:"no-respect-reviewers" env:"GOVERN_NO_RESPECT_REVIEWERS" usage:"Whether the PR's requested reviewers review should not be considered even if they are not part of a team/codeowner"`
	Push               bool     `long:"push" env:"GOVERN_PUSH" usage:"Following the merge push to the remote"`
	Repo               string   `long:"repo" short:"p" env:"GOVERN_REPO" usage:"Apply patches to the following local repository"`
	ReviewerComments   []string `long:"reviewer-comments" env:"GOVERN_REVIEWER_COMMENTS" usage:"Regular expression that a reviewer writes"`
	ReviewerTeams      []string `long:"reviewer-teams" env:"GOVERN_REVIEWER_TEAMS" usage:"The GitHub team that the reviewer must be a part to be considered a reviewer"`
	ReviewStates       []string `long:"review-states" env:"GOVERN_REVIEW_STATES" usage:"The state of the GitHub approval from the reivewer"`
	States             []string `long:"states" env:"GOVERN_STATES" usage:"Consider the PR mergable if it has one of these supplied states"`
	Trailers           []string `long:"trailer" short:"t" env:"GOVERN_TRAILER" usage:"Append additional Git trailers to each git commit message"`
}

func NewMerge() *cobra.Command {
	cmd, err := cmdfactory.New(&Merge{}, cobra.Command{
		Use:   "merge [OPTIONS] ORG/REPO/PRID",
		Short: "Merge a pull request",
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

func (opts *Merge) Run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	ghOrg, ghRepo, ghPrId, err := cmdutils.ParseOrgRepoAndPullRequestArgs(args)
	if err != nil {
		return err
	}

	ghClient, err := ghapi.NewGithubClient(
		ctx,
		kitcfg.G[config.Config](ctx).GithubToken,
		kitcfg.G[config.Config](ctx).GithubSkipSSL,
		kitcfg.G[config.Config](ctx).GithubEndpoint,
	)
	if err != nil {
		return err
	}

	pull, err := ghpr.NewPullRequestFromID(ctx,
		ghClient,
		ghOrg,
		ghRepo,
		ghPrId,
		ghpr.WithBaseBranch(opts.BaseBranch),
		ghpr.WithWorkdir(kitcfg.G[config.Config](ctx).TempDir),
	)
	if err != nil {
		return fmt.Errorf("could not prepare pull request: %w", err)
	}

	defer func() {
		// If the user has not specified a temporary directory which will have been
		// passed as the working directory, a temporary one will have been generated.
		// This isn't a "neat" way of cleaning up.
		if kitcfg.G[config.Config](ctx).TempDir == "" {
			log.G(ctx).WithField("path", pull.Workdir()).Info("removing")
			os.RemoveAll(pull.Workdir())
		}
	}()

	if !opts.NoCheckMergable {
		log.G(ctx).Info("checking if the pull request satisfies merge requirements")
		mergable, results, err := pull.SatisfiesMergeRequirements(ctx,
			ghpr.WithApproverComments(opts.ApproverComments...),
			ghpr.WithApproverTeams(opts.ApproverTeams...),
			ghpr.WithApproveStates(opts.ApproveStates...),
			ghpr.WithIgnoreLabels(opts.IgnoreLabels...),
			ghpr.WithIgnoreStates(opts.IgnoreStates...),
			ghpr.WithLabels(opts.Labels...),
			ghpr.WithMinApprovals(opts.MinApprovals),
			ghpr.WithMinReviews(opts.MinReviews),
			ghpr.WithNoConflicts(opts.NoConflicts),
			ghpr.WithNoDraft(opts.NoDraft),
			ghpr.WithNoRespectAssignees(opts.NoRespectAssignees),
			ghpr.WithNoRespectReviewers(opts.NoRespectReviewers),
			ghpr.WithReviewerComments(opts.ReviewerComments...),
			ghpr.WithReviewerTeams(opts.ReviewerTeams...),
			ghpr.WithReviewStates(opts.ReviewStates...),
			ghpr.WithStates(opts.States...),
		)
		if err != nil {
			return fmt.Errorf("pull request is not mergable: %w", err)
		} else if !mergable {
			return fmt.Errorf("pull request is not mergable")
		}

		if !opts.NoAutoTrailerPatch {
			for k, trailers := range results {
				r := []rune(k)
				trailerName := strings.ReplaceAll(string(append([]rune{unicode.ToUpper(r[0])}, r[1:]...)), "_", "-")

				for _, trailer := range trailers {
					opts.Trailers = append(opts.Trailers,
						fmt.Sprintf("%s: %s", trailerName, trailer),
					)
				}
			}
		}
	}

	opts.Trailers = append(opts.Trailers,
		fmt.Sprintf("GitHub-Closes: #%d", ghPrId),
	)

	if os.Getenv("GITHUB_ACTIONS") == "yes" {
		opts.Trailers = append(opts.Trailers,
			"Tested-by: GitHub Actions <monkey+github-actions@unikraft.io>",
		)
	}

	tempDir := kitcfg.G[config.Config](ctx).TempDir
	if tempDir == "" {
		tempDir, err = os.MkdirTemp("", "governctl-pr-merge-*")
		if err != nil {
			return fmt.Errorf("could not create temporary directory: %w", err)
		}

		defer func() {
			os.RemoveAll(tempDir)
		}()
	}

	if opts.Repo == "" {
		opts.Repo = filepath.Join(tempDir, fmt.Sprintf("unikraft-pr-%d-patched", ghPrId))

		log.G(ctx).
			WithField("from", *pull.Metadata().Base.Repo.CloneURL).
			WithField("to", opts.Repo).
			Info("cloning fresh repository")

		if _, err := git.PlainClone(opts.Repo, false, &git.CloneOptions{
			URL: *pull.Metadata().Base.Repo.CloneURL,
		}); err != nil {
			log.G(ctx).Fatalf("could not clone repository: %s", err)
			os.Exit(1)
		}
	}

	if opts.CommitterName != "" {
		cmd := exec.Command("git", "-C", opts.Repo, "config", "user.name", opts.CommitterName)
		cmd.Stderr = log.G(ctx).WriterLevel(logrus.ErrorLevel)
		cmd.Stdout = log.G(ctx).WriterLevel(logrus.DebugLevel)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("could not apply patch: %w", err)
		}
	}

	if opts.CommitterEmail != "" {
		cmd := exec.Command("git", "-C", opts.Repo, "config", "user.email", opts.CommitterEmail)
		cmd.Stderr = log.G(ctx).WriterLevel(logrus.ErrorLevel)
		cmd.Stdout = log.G(ctx).WriterLevel(logrus.DebugLevel)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("could not apply patch: %w", err)
		}
	}

	for _, patch := range pull.Patches() {
		log.G(ctx).
			WithField("title", patch.Title).
			Info("generating patch")

		patch.Trailers = append(patch.Trailers, opts.Trailers...)

		cmd := exec.Command("git", "-C", opts.Repo, "am", "--3way")
		cmd.Stdin = bytes.NewReader(patch.Bytes())
		cmd.Stderr = log.G(ctx).WriterLevel(logrus.ErrorLevel)
		cmd.Stdout = log.G(ctx).WriterLevel(logrus.DebugLevel)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("could not apply patch: %w", err)
		}
	}

	if opts.Push {
		log.G(ctx).Info("pushing to remote")

		cmd := exec.Command(
			"git",
			"-C", opts.Repo,
			"remote", "add", "patched",
			fmt.Sprintf("https://%s:%s@github.com/%s/%s.git",
				kitcfg.G[config.Config](ctx).GithubUser,
				kitcfg.G[config.Config](ctx).GithubToken,
				ghOrg,
				ghRepo,
			))
		cmd.Stderr = log.G(ctx).WriterLevel(logrus.ErrorLevel)
		cmd.Stdout = log.G(ctx).WriterLevel(logrus.DebugLevel)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("could not apply patch: %w", err)
		}

		if !kitcfg.G[config.Config](ctx).DryRun {
			cmd = exec.Command(
				"git",
				"-C", opts.Repo,
				"push", "-u", "patched",
			)
			cmd.Stderr = log.G(ctx).WriterLevel(logrus.ErrorLevel)
			cmd.Stdout = log.G(ctx).WriterLevel(logrus.DebugLevel)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("could not apply patch: %w", err)
			}
		}
	}

	return nil
}