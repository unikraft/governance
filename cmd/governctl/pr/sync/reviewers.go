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
	"github.com/hairyhenderson/go-codeowners"
	"github.com/spf13/cobra"
	"github.com/waigani/diffparser"
	"kraftkit.sh/cmdfactory"
	kitcfg "kraftkit.sh/config"
	"kraftkit.sh/log"

	"github.com/unikraft/governance/internal/cmdutils"
	"github.com/unikraft/governance/internal/config"
	"github.com/unikraft/governance/internal/ghapi"
	"github.com/unikraft/governance/internal/pair"
	"github.com/unikraft/governance/internal/repo"
	"github.com/unikraft/governance/internal/team"
	"github.com/unikraft/governance/utils"
)

type Reviewers struct {
	NumMaintainers int `long:"num-maintainers" short:"A" usage:"Number of maintainers for the PR" default:"1"`
	NumReviewers   int `long:"num-reviewers" short:"R" usage:"Number of reviewers for the PR" default:"1"`

	ghClient           *ghapi.GithubClient
	maintainerWorkload map[string]int
	reviewerWorkload   map[string]int
}

func NewReviewers() *cobra.Command {
	cmd, err := cmdfactory.New(&Reviewers{}, cobra.Command{
		Use:   "reviewers [OPTIONS] ORG/REPO/PRID",
		Short: "Synchronise a pull request's assignees (maintainers) and reviewers",
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

func (opts *Reviewers) Run(ctx context.Context, args []string) error {
	var err error

	opts.ghClient, err = ghapi.NewGithubClient(
		ctx,
		kitcfg.G[config.Config](ctx).GithubToken,
		kitcfg.G[config.Config](ctx).GithubSkipSSL,
		kitcfg.G[config.Config](ctx).GithubEndpoint,
	)
	if err != nil {
		return err
	}

	ghOrg, ghRepo, ghPrId, err := cmdutils.ParseOrgRepoAndPullRequestArgs(args)
	if err != nil {
		return err
	}

	repos, err := repo.NewListOfReposFromPath(
		opts.ghClient,
		ghOrg,
		kitcfg.G[config.Config](ctx).ReposDir,
	)
	if err != nil {
		return fmt.Errorf("could not populate repos: %w", err)
	}

	pr, err := opts.ghClient.GetPullRequest(ctx, ghRepo, ghRepo, ghPrId)
	if err != nil {
		return fmt.Errorf("could not get pull request")
	}

	if *pr.State == "closed" {
		return fmt.Errorf("pull request is closed")
	}

	ghOrigin := fmt.Sprintf("https://github.com/%s/%s.git", ghOrg, ghRepo)

	teams, err := team.NewListOfTeamsFromPath(
		opts.ghClient,
		ghOrg,
		kitcfg.G[config.Config](ctx).TeamsDir,
	)
	if err != nil {
		return err
	}

	opts.maintainerWorkload = make(map[string]int)
	opts.reviewerWorkload = make(map[string]int)

	log.G(ctx).Info("reversing the relationship between teams and organization repos")

	teamMap := make(map[string]*team.Team)

	for _, t := range teams {
		for _, r := range t.Repositories {
			// Only select teams that are responsible for the input repository.
			if !r.NameEquals(ghRepo) {
				continue
			}

			if _, ok := teamMap[r.Fullname()]; ok {
				teamMap[t.Fullname()] = t
			} else {
				// Use thhe the initialised repository
				r2 := repo.FindRepoByName(r.Fullname(), repos)
				if r2 != nil {
					r = *r2
				}

				teamMap[r.Fullname()] = t
			}
		}

		// Populate global lists of workloads for both maintainers and reviewers
		for _, m := range t.Maintainers {
			if _, ok := opts.maintainerWorkload[m.Github]; !ok {
				opts.maintainerWorkload[m.Github] = 0
			}
		}

		for _, m := range t.Reviewers {
			if _, ok := opts.reviewerWorkload[m.Github]; !ok {
				opts.reviewerWorkload[m.Github] = 0
			}
		}
	}

	log.G(ctx).Info("determining the workload of all maintainers and reviewers")

	prs, err := opts.ghClient.ListOpenPullRequests(
		ctx,
		ghOrg,
		ghRepo,
	)
	if err != nil {
		return fmt.Errorf("could not retrieve pull requests: %w", err)
	}

	for _, pr := range prs {
		if *pr.State != "open" {
			continue
		}

		maintainers, err := opts.ghClient.GetMaintainersOnPr(
			ctx,
			ghOrg,
			ghRepo,
			*pr.Number,
		)
		if err != nil {
			return fmt.Errorf("could not get maintainers on pull requests: %w", err)
		}

		for _, maintainer := range maintainers {
			if _, ok := opts.maintainerWorkload[maintainer]; !ok {
				opts.maintainerWorkload[maintainer] = 0
			}

			opts.maintainerWorkload[maintainer]++
		}

		reviewers, err := opts.ghClient.GetReviewersOnPr(
			ctx,
			ghOrg,
			ghRepo,
			*pr.Number,
		)
		if err != nil {
			return fmt.Errorf("could not get reviewers on pull requests: %w", err)
		}

		for _, reviewer := range reviewers {
			if _, ok := opts.reviewerWorkload[reviewer]; !ok {
				opts.reviewerWorkload[reviewer] = 0
			}

			opts.reviewerWorkload[reviewer]++
		}

		log.G(ctx).
			WithField("reviewers", reviewers).
			WithField("maintainers", maintainers).
			WithField("pr_id", *pr.Number).
			Info("checked open pr")
	}

	for maintainer, workload := range opts.maintainerWorkload {
		log.G(ctx).
			WithField("maintainer", maintainer).
			WithField("workload", workload).
			Info("workload")
	}

	for reviewer, workload := range opts.reviewerWorkload {
		log.G(ctx).
			WithField("reviewer", reviewer).
			WithField("workload", workload).
			Info("workload")
	}

	log.G(ctx).
		WithField("pr_id", ghPrId).
		Info("getting pull request details")

	localRepo := path.Join(kitcfg.G[config.Config](ctx).TempDir, ghRepo)

	if os.Getenv("GITHUB_ACTIONS") == "yes" {
		localRepo = os.Getenv("GITHUB_WORKSPACE")
	}

	// Check if we have a copy of the repo locally, we'll use it in the next
	// step when checking CODEOWNERS
	if _, err := os.Stat(localRepo); os.IsNotExist(err) {
		log.G(ctx).
			WithField("from", ghOrigin).
			WithField("to", localRepo).
			Info("cloning git repository")
		_, err := git.PlainClone(localRepo, false, &git.CloneOptions{
			URL: ghOrigin,
			Auth: &http.BasicAuth{
				Username: kitcfg.G[config.Config](ctx).GithubUser,
				Password: kitcfg.G[config.Config](ctx).GithubToken,
			},
		})
		if err != nil {
			return fmt.Errorf("could not clone repository: %w", err)
		}
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

	log.G(ctx).Info("retrieving list of modified files")

	localDiffFile := path.Join(tempDir, fmt.Sprintf("%s-%d.diff", ghRepo, ghPrId))

	if _, err := os.Stat(localDiffFile); os.IsNotExist(err) {
		log.G(ctx).
			WithField("from", *pr.DiffURL).
			WithField("to", localDiffFile).
			Info("saving")

		if err = utils.DownloadFile(localDiffFile, *pr.DiffURL); err != nil {
			return fmt.Errorf("could not download pull request: %w", err)
		}
	}

	d, err := os.ReadFile(localDiffFile)
	if err != nil {
		return fmt.Errorf("could not read diff file for request: %w", err)
	}

	log.G(ctx).Info("parsing diff")

	diff, err := diffparser.Parse(string(d))
	if err != nil {
		return fmt.Errorf("could not parse diff from pull request: %w", err)
	}

	// Does this repository use CODEOWNERS? If so, determine the teams based on
	// the changed file.
	co, err := codeowners.NewCodeowners(localRepo)
	if err == nil {
		log.G(ctx).Info("parsing repository CODEOWNERS")

		for _, f := range diff.Files {
			var owners []string
			if len(f.OrigName) > 0 {
				owners = append(owners, co.Owners(f.OrigName)...)
			}
			if len(f.NewName) > 0 {
				owners = append(owners, co.Owners(f.NewName)...)
			}

			for _, o := range owners {
				codeTeam := team.FindTeamByName(o, teams)
				if codeTeam == nil {
					continue
				}

				// Add the team to the repository
				if _, ok := teamMap[codeTeam.Fullname()]; !ok {
					log.G(ctx).
						WithField("team", codeTeam.Fullname()).
						Info("adding extra team from CODEOWNERS...")

					teamMap[codeTeam.Fullname()] = codeTeam
				}
			}
		}
	}

	var maintainers []string
	var reviewers []string

	// Go through all calculated teams and add memebers as potential
	// candidates for reviewers and maintainers
	for _, t := range teamMap {
		for _, m := range t.Maintainers {
			// Don't add duplicates
			if containsStr(maintainers, m.Github) {
				continue
			}

			// Do not add the PR author
			if m.Github == *pr.User.Login {
				continue
			}

			maintainers = append(maintainers, m.Github)
		}

		for _, m := range t.Reviewers {
			// Don't add duplicates
			if containsStr(reviewers, m.Github) {
				continue
			}

			// Do not add the PR author
			if m.Github == *pr.User.Login {
				continue
			}

			reviewers = append(reviewers, m.Github)
		}
	}

	return opts.updatePrWithPossibleMaintainersAndReviewers(
		ctx,
		ghOrg,
		ghRepo,
		ghPrId,
		maintainers,
		reviewers,
	)
}

func (opts *Reviewers) popLeastStressedMaintainer(subset []string) string {
	maintainers := make(map[string]int)

	for _, username := range subset {
		if _, ok := opts.maintainerWorkload[username]; !ok {
			opts.maintainerWorkload[username] = 0
		}

		maintainers[username] = opts.maintainerWorkload[username]
	}

	sorted := pair.RankByWorkload(maintainers)
	least := sorted[0].Key
	opts.maintainerWorkload[least]++

	return least
}

func (opts *Reviewers) popLeastStressedReviewer(subset []string) string {
	reviewers := make(map[string]int)

	for _, username := range subset {
		if _, ok := opts.reviewerWorkload[username]; !ok {
			opts.reviewerWorkload[username] = 0
		}

		reviewers[username] = opts.reviewerWorkload[username]
	}

	sorted := pair.RankByWorkload(reviewers)

	least := sorted[0].Key
	opts.reviewerWorkload[least]++
	return least
}

func (opts *Reviewers) updatePrWithPossibleMaintainersAndReviewers(ctx context.Context, org, repo string, prId int, possibleMaintainers []string, possibleReviewers []string) error {
	log.G(ctx).
		WithField("repo", repo).
		WithField("pr_id", prId).
		// WithField("maintainers", possibleMaintainers).
		// WithField("reviewers", possibleReviewers).
		Infof("assigning reviewer(s) and maintainer(s) to pull request...")

	if len(possibleMaintainers) == 0 {
		return fmt.Errorf("could not assign reviewers as none provided")
	}
	if len(possibleReviewers) == 0 {
		return fmt.Errorf("could not assign reviewers as none provided")
	}

	maintainers, err := opts.ghClient.GetMaintainersOnPr(ctx, org, repo, prId)
	if err != nil {
		return err
	}

	if len(maintainers) == 0 {
		for i := 0; i < opts.NumMaintainers; i++ {
			m := opts.popLeastStressedMaintainer(possibleMaintainers)
			maintainers = append(maintainers, m)

			log.G(ctx).
				WithField("maintainer", m).
				Info("assigning maintainer")
		}

		if !kitcfg.G[config.Config](ctx).DryRun {
			err := opts.ghClient.AddMaintainersToPr(ctx, org, repo, prId, maintainers)
			if err != nil {
				return fmt.Errorf("could not add maintainers to repo=%s pr_id=%d: %s", repo, prId, err)
			}
		}
	}

	// Remove assigned maintainers from list of possible reviewers (in case there
	// are any overlaps as we cannot have the same reviewer and approver).
	for _, maintainer := range maintainers {
		for i, reviewer := range possibleReviewers {
			if reviewer == maintainer {
				possibleReviewers = append(possibleReviewers[:i], possibleReviewers[i+1:]...)
			}
		}
	}

	log.G(ctx).
		WithField("repo", repo).
		WithField("pr_id", prId).
		WithField("maintainers", maintainers).
		Info("assigning maintainers")

	var reviewers []string

	// Run a check to see if the PR has already received reviews
	r, _ := opts.ghClient.GetReviewUsersOnPr(ctx, org, repo, prId)
	if len(r) > 0 {
		reviewers = append(reviewers, r...)
	}

	r, err = opts.ghClient.GetReviewersOnPr(ctx, org, repo, prId)
	if err != nil {
		return err
	}
	if len(r) > 0 {
		reviewers = append(reviewers, r...)
	}

	if len(reviewers) == 0 {
		for i := len(reviewers); i < opts.NumReviewers; i++ {
			r := opts.popLeastStressedReviewer(possibleReviewers)
			reviewers = append(reviewers, r)

			log.G(ctx).
				WithField("reviewer", r).
				Info("assigning reviewer")
		}

		if !kitcfg.G[config.Config](ctx).DryRun && len(reviewers) > 0 {
			err := opts.ghClient.AddReviewersToPr(ctx, org, repo, prId, reviewers)
			if err != nil {
				return fmt.Errorf("could not add reviewer: %w", err)
			}
		}
	}

	return nil
}

func containsStr(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
