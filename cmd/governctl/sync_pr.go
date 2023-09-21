// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/google/go-github/v32/github"
	"github.com/hairyhenderson/go-codeowners"
	"github.com/spf13/cobra"
	"github.com/waigani/diffparser"
	git "gopkg.in/src-d/go-git.v4"
	"kraftkit.sh/cmdfactory"
	kitcfg "kraftkit.sh/config"
	"kraftkit.sh/log"

	"github.com/unikraft/governance/internal/config"
	"github.com/unikraft/governance/internal/ghapi"
	"github.com/unikraft/governance/internal/label"
	"github.com/unikraft/governance/internal/pair"
	"github.com/unikraft/governance/internal/repo"
	"github.com/unikraft/governance/internal/team"
	"github.com/unikraft/governance/utils"
)

type pullRequest struct {
	pr    *github.PullRequest
	repo  repo.Repository
	teams map[string]*team.Team
}

type repoTeams struct {
	repo  repo.Repository
	teams map[string]*team.Team
}

type SyncPR struct {
	NumMaintainers int  `long:"num-maintainers" short:"A" usage:"Number of maintainers for the PR" default:"1"`
	NumReviewers   int  `long:"num-reviewers" short:"R" usage:"Number of reviewers for the PR" default:"1"`
	NoLabels       bool `long:"no-labels" usage:"Do not set labels on this PR"`

	maintainerWorkload map[string]int
	reviewerWorkload   map[string]int
	repoDirs           map[string]string
	numMaintainers     int
	numReviewers       int
	repo               string
	prId               int
	ghApi              *ghapi.GithubClient
	teams              []*team.Team
	repos              []*repo.Repository
	labels             []label.Label
}

func NewSyncPR() *cobra.Command {
	cmd, err := cmdfactory.New(&SyncPR{}, cobra.Command{
		Use:   "sync-pr [OPTIONS] [REPO [PRID]]",
		Short: "Synchronise one or many Pull Requests",
		Args:  cobra.MaximumNArgs(2),
		Annotations: map[string]string{
			cmdfactory.AnnotationHelpGroup: "main",
		},
	})
	if err != nil {
		panic(err)
	}

	return cmd
}

func (opts *SyncPR) Pre(cmd *cobra.Command, args []string) error {
	var err error
	ctx := cmd.Context()

	opts.ghApi, err = ghapi.NewGithubClient(
		kitcfg.G[config.Config](ctx).GithubOrg,
		kitcfg.G[config.Config](ctx).GithubToken,
		kitcfg.G[config.Config](ctx).GithubSkipSSL,
		kitcfg.G[config.Config](ctx).GithubEndpoint,
	)
	if err != nil {
		return err
	}

	opts.teams, err = team.NewListOfTeamsFromPath(
		opts.ghApi,
		kitcfg.G[config.Config](ctx).GithubOrg,
		kitcfg.G[config.Config](ctx).TeamsDir,
	)
	if err != nil {
		return fmt.Errorf("could not populate teams: %s", err)
	}

	opts.repos, err = repo.NewListOfReposFromPath(
		opts.ghApi,
		kitcfg.G[config.Config](ctx).GithubOrg,
		kitcfg.G[config.Config](ctx).ReposDir,
	)
	if err != nil {
		return fmt.Errorf("could not populate repos: %s", err)
	}

	opts.labels, err = label.NewListOfLabelsFromPath(
		opts.ghApi,
		kitcfg.G[config.Config](ctx).GithubOrg,
		kitcfg.G[config.Config](ctx).LabelsDir,
	)
	if err != nil {
		return fmt.Errorf("could not populate repos: %s", err)
	}

	opts.maintainerWorkload = make(map[string]int)
	opts.reviewerWorkload = make(map[string]int)
	opts.repoDirs = make(map[string]string)

	return nil
}

func (opts *SyncPR) Run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	if len(args) > 0 {
		// Check to determine if provided argument is a local git folder
		if _, err := os.Stat(args[0]); !os.IsNotExist(err) {
			basename := filepath.Base(args[0])
			r := repo.FindRepoByName(basename, opts.repos)
			if r == nil {
				// If we can't figure out the repo based on its folder name, let's try
				// and see if its a Git repo and determine its name by its remotes
				d, err := git.PlainOpen(args[0])
				if err != nil {
					log.G(ctx).Fatalf("unknown repo: %s", args[0])
					os.Exit(1)
				}

				// No remotes
				remotes, err := d.Remotes()
				if err != nil {
					log.G(ctx).Fatalf("unknown repo: %s", args[0])
					os.Exit(1)
				}

				// No or too many remotes
				if len(remotes) == 0 || len(remotes) > 1 {
					log.G(ctx).Fatalf("unknown repo: %s", args[0])
					os.Exit(1)
				}

				// No or too many URLs on remote
				config := remotes[0].Config()
				if len(config.URLs) > 1 {
					log.G(ctx).Fatalf("unknown repo: %s", args[0])
					os.Exit(1)
				}

				// Invalid remote URL
				uri, err := url.ParseRequestURI(config.URLs[0])
				if err != nil {
					log.G(ctx).Fatalf("unknown repo: %s", args[0])
					os.Exit(1)
				}

				r = repo.FindRepoByName(filepath.Base(uri.Path), opts.repos)
				if r == nil {
					log.G(ctx).Fatalf("unknown repo: %s", args[0])
					os.Exit(1)
				}
			}

			opts.repo = r.Fullname()
			opts.repoDirs[r.Fullname()] = args[0]

			// Check to determine if provided argument is a remote git repo
		} else if uri, err := url.ParseRequestURI(args[0]); err == nil && uri.Scheme != "" && uri.Host != "" {
			basename := filepath.Base(uri.Path)
			r := repo.FindRepoByName(basename, opts.repos)
			if r == nil {
				log.G(ctx).Fatalf("unknown repo: %s", args[0])
				os.Exit(1)
			}

			localRepo := path.Join(kitcfg.G[config.Config](ctx).TempDir, basename)

			if _, err := os.Stat(localRepo); os.IsNotExist(err) {
				log.G(ctx).Debugf("Cloning remote git repository: %s to %s", args[0], localRepo)
				_, err := git.PlainClone(localRepo, false, &git.CloneOptions{
					URL: args[0],
				})
				if err != nil {
					log.G(ctx).Fatalf("could not clone repository: %s", err)
					os.Exit(1)
				}
			}

			opts.repo = r.Fullname()
			opts.repoDirs[r.Fullname()] = localRepo

		} else {
			repo := repo.FindRepoByName(args[0], opts.repos)
			if repo == nil {
				log.G(ctx).Fatalf("unknown repo: %s", args[0])
				os.Exit(1)
			}

			opts.repo = args[0]
		}
	}

	if len(args) > 1 {
		i, err := strconv.Atoi(args[1])
		if err != nil {
			log.G(ctx).Fatalf("Could not convert PRID to integer: %s", err)
			os.Exit(1)
		}

		opts.prId = i
	}

	teams, err := team.NewListOfTeamsFromPath(
		opts.ghApi,
		kitcfg.G[config.Config](ctx).GithubOrg,
		kitcfg.G[config.Config](ctx).TeamsDir,
	)
	if err != nil {
		log.G(ctx).Fatalf("could not parse teams: %s", err)
		os.Exit(1)
	}

	log.G(ctx).Debug("Reversing the relationship between teams and repos...")
	repoTeamsMap := make(map[string]repoTeams)
	for _, t := range teams {
		for _, r := range t.Repositories {
			// Do not parse repositories if we requested a specific and it does not
			// match
			if len(opts.repo) > 0 && !r.NameEquals(opts.repo) {
				continue
			}

			if repoTeam, ok := repoTeamsMap[r.Fullname()]; ok {
				repoTeam.teams[t.Fullname()] = t
			} else {
				// Use thhe the initialised repository
				r2 := repo.FindRepoByName(r.Fullname(), opts.repos)
				if r2 != nil {
					r = *r2
				}

				repoTeamsMap[r.Fullname()] = repoTeams{
					repo: r,
					teams: map[string]*team.Team{
						t.Fullname(): t,
					},
				}
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

	log.G(ctx).Debugf("Determining the workload of all maintainers and reviewers...")
	for _, r := range repoTeamsMap {
		// Get a list of all open PRs
		prs, err := opts.ghApi.ListOpenPullRequests(ctx, r.repo.Fullname())
		if err != nil {
			log.G(ctx).Fatalf("could not retrieve pull requests: %s", err)
			os.Exit(1)
		}

		for _, pr := range prs {
			maintainers, err := opts.ghApi.GetMaintainersOnPr(ctx, r.repo.Fullname(), *pr.Number)
			if err != nil {
				log.G(ctx).Fatalf("could not get maintainers on pull requests: %s", err)
				os.Exit(1)
			}

			for _, maintainer := range maintainers {
				if _, ok := opts.maintainerWorkload[maintainer]; !ok {
					opts.maintainerWorkload[maintainer] = 0
				}

				opts.maintainerWorkload[maintainer]++
			}

			reviewers, err := opts.ghApi.GetReviewersOnPr(ctx, r.repo.Fullname(), *pr.Number)
			if err != nil {
				log.G(ctx).Fatalf("could not get reviewers on pull requests: %s", err)
				os.Exit(1)
			}

			for _, reviewer := range reviewers {
				if _, ok := opts.reviewerWorkload[reviewer]; !ok {
					opts.reviewerWorkload[reviewer] = 0
				}

				opts.reviewerWorkload[reviewer]++
			}
		}
	}

	relevantPrs := make(map[string]map[int]*pullRequest)

	log.G(ctx).Debugf("Calculating lists of potential reviewers and maintainers...")
	for _, r := range repoTeamsMap {
		// Get a list of all open PRs
		prs, err := opts.ghApi.ListOpenPullRequests(ctx, r.repo.Fullname())
		if err != nil {
			log.G(ctx).Fatalf("could not retrieve pull requests: %s", err)
			os.Exit(1)
		}

		for _, pr := range prs {
			if opts.prId > 0 && *pr.Number != opts.prId {
				continue
			}

			// Ignore draft PRs
			if *pr.Draft {
				continue
			}

			if _, ok := relevantPrs[r.repo.Fullname()]; !ok {
				relevantPrs[r.repo.Fullname()] = make(map[int]*pullRequest)
			}

			relevantPrs[r.repo.Fullname()][*pr.Number] = &pullRequest{
				pr:    pr,
				repo:  r.repo,
				teams: r.teams,
			}
		}
	}

	if len(relevantPrs) == 0 {
		log.G(ctx).Fatalf("could not match pull request(s)")
		os.Exit(1)
	}

	for repoName, prs := range relevantPrs {
		if len(prs) == 0 && opts.prId > 0 {
			log.G(ctx).Fatalf("could not find pr with id=%d for repo=%s", opts.prId, repoName)
			os.Exit(1)

		} else if len(prs) == 0 {
			log.G(ctx).
				WithField("repo", repoName).
				Infof("no open pull requests")
			continue
		}

		localRepo, ok := opts.repoDirs[repoName]
		if !ok {
			localRepo = path.Join(kitcfg.G[config.Config](ctx).TempDir, repoName)
			opts.repoDirs[repoName] = localRepo
		}

		// Check if we have a copy of the repo locally, we'll use it in the next
		// step when checking CODEOWNERS
		if _, err := os.Stat(localRepo); os.IsNotExist(err) {
			r := repo.FindRepoByName(repoName, opts.repos)
			log.G(ctx).Debugf("Cloning remote git repositeory: %s to %s", r.Origin, localRepo)
			_, err := git.PlainClone(localRepo, false, &git.CloneOptions{
				URL: r.Origin,
			})
			if err != nil {
				log.G(ctx).Fatalf("could not clone repository: %s", err)
				os.Exit(1)
			}
		}

		// Does this repository use CODEOWNERS? Prepare a way to check for files
		// in a PR if possible
		co, useCodeownersErr := codeowners.NewCodeowners(localRepo)

		// Parse each pull request
		for prId, pr := range prs {
			var maintainers []string
			var reviewers []string

			log.G(ctx).
				WithField("repo", pr.repo.Fullname()).
				Debugf("Repo uses CODEOWNERS")

			// Retrieve a list of modofied files in this PR
			localDiffFile := path.Join(kitcfg.G[config.Config](ctx).TempDir, fmt.Sprintf("%s-%d.diff",
				pr.repo.Fullname(),
				prId,
			))

			if _, err := os.Stat(localDiffFile); os.IsNotExist(err) {
				log.G(ctx).Debugf("Saving %s to %s...", *pr.pr.DiffURL, localDiffFile)
				err = utils.DownloadFile(localDiffFile, *pr.pr.DiffURL)
				if err != nil {
					log.G(ctx).Fatalf("could not download pull request on repo=%s with pr_id=%d diff: %s", pr.repo.Fullname(), prId, err)
					os.Exit(1)
				}
			}

			d, err := ioutil.ReadFile(localDiffFile)
			if err != nil {
				log.G(ctx).Fatalf("could not read diff file for request on repo=%s with pr_id=%d diff: %s", pr.repo.Fullname(), prId, err)
				os.Exit(1)
			}

			diff, err := diffparser.Parse(string(d))
			if err != nil {
				log.G(ctx).Fatalf("could not parse diff from pull request on repo=%s with pr_id=%d: %s", pr.repo.Fullname(), prId, err)
				os.Exit(1)
			}

			var labelsToAdd []string
			for _, f := range diff.Files {
				// Determine the teams based on the changed files
				if useCodeownersErr == nil {
					var owners []string
					if len(f.OrigName) > 0 {
						owners = append(owners, co.Owners(f.OrigName)...)
					}
					if len(f.NewName) > 0 {
						owners = append(owners, co.Owners(f.NewName)...)
					}

					for _, o := range owners {
						codeTeam := team.FindTeamByName(o, opts.teams)
						if codeTeam == nil {
							continue
						}

						// Add the team to the repository
						if _, ok := pr.teams[codeTeam.Fullname()]; !ok {
							log.G(ctx).
								WithField("team", codeTeam.Fullname()).
								Debugf("Adding extra team from CODEOWNERS...")

							pr.teams[codeTeam.Fullname()] = codeTeam
						}
					}
				}

				// Determine the labels to add based on the changed files
				for _, label := range opts.labels {
					if containsStr(labelsToAdd, label.Name) {
						continue
					}

					if len(f.OrigName) > 0 && label.AppliesTo(repoName, f.OrigName) {
						labelsToAdd = append(labelsToAdd, label.Name)
					}

					if !containsStr(labelsToAdd, label.Name) && len(f.NewName) > 0 && label.AppliesTo(repoName, f.NewName) {
						labelsToAdd = append(labelsToAdd, label.Name)
					}
				}
			}

			if len(labelsToAdd) > 0 {
				log.G(ctx).
					WithField("repo", repoName).
					WithField("pr_id", prId).
					WithField("labels", labelsToAdd).
					Infof("Setting labels on pull request...")

				if !kitcfg.G[config.Config](ctx).DryRun {
					err := opts.ghApi.AddLabelsToPr(ctx, repoName, prId, labelsToAdd)
					if err != nil {
						log.G(ctx).Fatalf("could not add labels repo=%s pr_id=%d: %s", repoName, prId, err)
					}
				}
			}

			// Go through all calculated teams and add memebers as potential
			// candidates for reviewers and maintainers
			for _, t := range pr.teams {
				for _, m := range t.Maintainers {
					// Don't add duplicates
					if containsStr(maintainers, m.Github) {
						continue
					}

					// Do not add the PR author
					if m.Github == *pr.pr.User.Login {
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
					if m.Github == *pr.pr.User.Login {
						continue
					}

					reviewers = append(reviewers, m.Github)
				}
			}

			err = opts.updatePrWithPossibleMaintainersAndReviewers(
				ctx,
				repoName,
				prId,
				maintainers,
				reviewers,
			)
			if err != nil {
				log.G(ctx).Fatalf("could not update repo=%s pr_id=%d: %s", repoName, prId, err)
				os.Exit(1)
			}
		}
	}

	return nil
}

func (opts *SyncPR) popLeastStressedMaintainer(subset []string) string {
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

func (opts *SyncPR) popLeastStressedReviewer(subset []string) string {
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

func (opts *SyncPR) updatePrWithPossibleMaintainersAndReviewers(ctx context.Context, repo string, prId int, possibleMaintainers []string, possibleReviewers []string) error {
	log.G(ctx).
		WithField("repo", repo).
		WithField("pr_id", prId).
		// WithField("maintainers", possibleMaintainers).
		// WithField("reviewers", possibleReviewers).
		Infof("Assigning reviewer(s) and maintainer(s) to pull request...")

	if len(possibleMaintainers) == 0 {
		return fmt.Errorf("could not assign reviewers as none provided")
	}
	if len(possibleReviewers) == 0 {
		return fmt.Errorf("could not assign reviewers as none provided")
	}

	maintainers, err := opts.ghApi.GetMaintainersOnPr(ctx, repo, prId)
	if err != nil {
		return err
	}

	if len(maintainers) == 0 {
		for i := 0; i < opts.numMaintainers; i++ {
			m := opts.popLeastStressedMaintainer(possibleMaintainers)
			maintainers = append(maintainers, m)

			log.G(ctx).
				WithField("maintainer", m).
				Info("Assigning maintainer...")
		}

		if !kitcfg.G[config.Config](ctx).DryRun {
			err := opts.ghApi.AddMaintainersToPr(ctx, repo, prId, maintainers)
			if err != nil {
				log.G(ctx).Fatalf("could not add maintainers to repo=%s pr_id=%d: %s", repo, prId, err)
				os.Exit(1)
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
		Debugf("Assigned maintainers")

	var reviewers []string

	// Run a check to see if the PR has already received reviews
	r, _ := opts.ghApi.GetReviewUsersOnPr(ctx, repo, prId)
	if len(r) > 0 {
		reviewers = append(reviewers, r...)
	}

	r, err = opts.ghApi.GetReviewersOnPr(ctx, repo, prId)
	if err != nil {
		return err
	}
	if len(r) > 0 {
		reviewers = append(reviewers, r...)
	}

	if len(reviewers) == 0 {
		for i := len(reviewers); i < opts.numReviewers; i++ {
			r := opts.popLeastStressedReviewer(possibleReviewers)
			reviewers = append(reviewers, r)

			log.G(ctx).
				WithField("reviewer", r).
				Info("Assigning reviewer...")
		}

		if !kitcfg.G[config.Config](ctx).DryRun {
			err := opts.ghApi.AddReviewersToPr(ctx, repo, prId, reviewers)
			if err != nil {
				log.G(ctx).Fatalf("could not add reviewer to repo=%s pr_id=%d: %s", repo, prId, err)
				os.Exit(1)
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
