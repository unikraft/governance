// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

// Package ghpr is an abstraction around GitHub's Pull Request.
package ghpr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	gitplumbing "github.com/go-git/go-git/v5/plumbing"
	gitobject "github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/google/go-github/v32/github"
	"github.com/sirupsen/logrus"
	kitcfg "kraftkit.sh/config"
	"kraftkit.sh/log"

	"github.com/unikraft/governance/internal/config"
	"github.com/unikraft/governance/internal/ghapi"
	"github.com/unikraft/governance/internal/patch"
)

type PullRequest struct {
	client     *ghapi.GithubClient
	pr         *github.PullRequest
	patches    []*patch.Patch
	baseBranch string
	workdir    string
	localRepo  string
	ghOrg      string
	ghRepo     string
	ghPrId     int
}

// NewPullRequestFromID fetches information about a pull request via GitHub as
// well as preparing the pull request as a series of patches that can be parsed
// internally.
func NewPullRequestFromID(ctx context.Context, client *ghapi.GithubClient, ghOrg, ghRepo string, ghPrId int, opts ...PullRequestOption) (*PullRequest, error) {
	var err error

	pr := PullRequest{
		client: client,
		ghOrg:  ghOrg,
		ghRepo: ghRepo,
		ghPrId: ghPrId,
	}

	for _, opt := range opts {
		if err := opt(&pr); err != nil {
			return nil, fmt.Errorf("could not apply option: %w", err)
		}
	}

	ghOrigin := fmt.Sprintf("https://github.com/%s/%s.git", ghOrg, ghRepo)

	if pr.workdir == "" {
		pr.workdir, err = os.MkdirTemp("", "governctl-pr-check-patch-*")
		if err != nil {
			return nil, fmt.Errorf("could not create temporary directory: %w", err)
		}
	}

	pr.localRepo = filepath.Join(pr.workdir, fmt.Sprintf("%s-pr-%d", ghRepo, ghPrId))

	if os.Getenv("GITHUB_ACTIONS") == "yes" {
		pr.localRepo = os.Getenv("GITHUB_WORKSPACE")
	}

	var repo *git.Repository

	// Check if we have a copy of the repo locally, we'll use it to obtain the
	// list of patches that need to be checked.
	if _, err := os.Stat(pr.localRepo); os.IsNotExist(err) {
		log.G(ctx).
			WithField("from", ghOrigin).
			WithField("to", pr.localRepo).
			Info("cloning git repository")

		copts := &git.CloneOptions{
			URL: ghOrigin,
			Auth: &http.BasicAuth{
				Username: kitcfg.G[config.Config](ctx).GithubUser,
				Password: kitcfg.G[config.Config](ctx).GithubToken,
			},
		}

		if pr.BaseBranch() != "" {
			copts.ReferenceName = gitplumbing.ReferenceName(pr.BaseBranch())
		}
		repo, err = git.PlainClone(pr.localRepo, false, copts)
		if err != nil {
			return nil, fmt.Errorf("could not clone repository: %w", err)
		}
	} else {
		repo, err = git.PlainOpen(pr.localRepo)
		if err != nil {
			return nil, fmt.Errorf("could not open repository: %w", err)
		}
	}

	repoConfig, err := repo.Config()
	if err != nil {
		return nil, fmt.Errorf("could not repo config: %w", err)
	}

	if pr.baseBranch == "" {
		pr.baseBranch = repoConfig.Init.DefaultBranch
	}

	// Attempt to gather the default branch by looking at the list of available
	// branches, if there is only one, this is likely the default based on the
	// original clone.
	if pr.baseBranch == "" && len(repoConfig.Branches) == 1 {
		for _, b := range repoConfig.Branches {
			pr.baseBranch = b.Name
			break
		}
	}

	baseBranch, err := repo.Branch(pr.baseBranch)
	if err != nil {
		return nil, fmt.Errorf("could not get base branch '%s': %w", pr.baseBranch, err)
	}

	baseRef, err := repo.Reference(baseBranch.Merge, true)
	if err != nil {
		return nil, fmt.Errorf("could not base reference: %w", err)
	}

	refname := fmt.Sprintf("refs/pull/%d/head", ghPrId)

	log.G(ctx).Info("fetching pull request details")

	if err := repo.Fetch(&git.FetchOptions{
		RefSpecs: []gitconfig.RefSpec{
			gitconfig.RefSpec(fmt.Sprintf("%s:%s", refname, refname)),
		},
		Auth: &http.BasicAuth{
			Username: kitcfg.G[config.Config](ctx).GithubUser,
			Password: kitcfg.G[config.Config](ctx).GithubToken,
		},
	}); err != nil && !strings.Contains(err.Error(), "already up-to-date") {
		return nil, fmt.Errorf("could not fetch pull request '%s': %w", refname, err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("could not get repository work tree: %w", err)
	}

	log.G(ctx).Info("checking out pull request locally")

	if err := w.Checkout(&git.CheckoutOptions{
		Branch: gitplumbing.ReferenceName(refname),
	}); err != nil {
		return nil, fmt.Errorf("could not checkout branch '%s': %w", refname, err)
	}

	log.G(ctx).Infof("rebasing pull request's branch on to '%s' branch", pr.baseBranch)

	rebase := exec.Command(
		"git",
		"-C", pr.localRepo,
		"rebase",
		"--merge",
		"--force-rebase",
		fmt.Sprintf("origin/%s", pr.baseBranch),
	)
	rebase.Stdout = log.G(ctx).WriterLevel(logrus.ErrorLevel)
	if rebase.Run(); err != nil {
		return nil, fmt.Errorf("could not rebase: %w", err)
	}

	log.G(ctx).Info("generating patch files")

	prHead, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("could not get HEAD: %w", err)
	}

	itr, err := repo.Log(&git.LogOptions{
		From: prHead.Hash(),
	})
	if err != nil {
		return nil, fmt.Errorf("could not get log: %w", err)
	}

	pr.pr, err = pr.client.GetPullRequest(ctx, ghOrg, ghRepo, ghPrId)
	if err != nil {
		return nil, fmt.Errorf("could not get pull request: %w", err)
	}

	stopErr := errors.New("stop")
	var prevCommit *gitobject.Commit

	totalCommits := 0

	pr.patches = make([]*patch.Patch, 0)

	if err := itr.ForEach(func(commit *gitobject.Commit) error {
		if prevCommit == nil {
			prevCommit = commit
			return nil
		}

		totalCommits++

		p, err := patch.NewPatchFromCommits(ctx, pr.localRepo, prevCommit, commit)
		if err != nil {
			return err
		}

		p.Filename = strings.ReplaceAll(p.Title, " ", "-")
		p.Filename = strings.ReplaceAll(p.Filename, ":", "")
		p.Filename = strings.ReplaceAll(p.Filename, ".", "")
		p.Filename = strings.ReplaceAll(p.Filename, "/", "-")
		p.Filename = strings.ReplaceAll(p.Filename, "?", "")
		p.Filename = strings.ReplaceAll(p.Filename, "`", "")
		p.Filename = filepath.Join(pr.workdir, fmt.Sprintf("%s-pr-%d-%d-%s.patch", ghRepo, ghPrId, totalCommits, p.Filename))

		pr.patches = append(pr.patches, p)

		if commit.Hash == baseRef.Hash() || totalCommits >= *pr.pr.Commits {
			return stopErr
		}

		prevCommit = commit
		return nil
	}); err != nil && !errors.Is(err, stopErr) {
		return nil, fmt.Errorf("could not iterate over log error: %w", err)
	}

	return &pr, nil
}

// LocalRepo is the path on disk to a copy of the pull request.
func (pr *PullRequest) LocalRepo() string {
	return pr.localRepo
}

// Patches contains the list of commits that this pull request consists of which
// are based on top of branch the pull request wishes to merge into.
func (pr *PullRequest) Patches() []*patch.Patch {
	return pr.patches
}

// Workdir is the parent directory that the pull request has been cloned into
// and is a space where content can be temporarily stored.
func (pr *PullRequest) Workdir() string {
	return pr.workdir
}

// Metadata is auxiliary information related to the pull request, e.g. author,
// date, etc.
func (pr *PullRequest) Metadata() *github.PullRequest {
	return pr.pr
}

// BaseBranch is the branch that the PR intends to merge into.
func (pr *PullRequest) BaseBranch() string {
	return pr.baseBranch
}
