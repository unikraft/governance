package main
// SPDX-License-Identifier: BSD-3-Clause
//
// Authors: Alexander Jung <a.jung@lancs.ac.uk>
//
// Copyright (c) 2021, Lancaster University.  All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions
// are met:
//
// 1. Redistributions of source code must retain the above copyright
//    notice, this list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright
//    notice, this list of conditions and the following disclaimer in the
//    documentation and/or other materials provided with the distribution.
// 3. Neither the name of the copyright holder nor the names of its
//    contributors may be used to endorse or promote products derived from
//    this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
// ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
// LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
// CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
// SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
// CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
// ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
// POSSIBILITY OF SUCH DAMAGE.

import (
  "os"
  "fmt"
  "path"
  "strconv"
  "net/url"
  "io/ioutil"
  "path/filepath"

  "github.com/spf13/cobra"
  git "gopkg.in/src-d/go-git.v4"
  "github.com/waigani/diffparser"
  log "github.com/sirupsen/logrus"
  "github.com/google/go-github/v32/github"
  "github.com/hairyhenderson/go-codeowners"

  "github.com/unikraft/governance/utils"
  "github.com/unikraft/governance/internal/team"
  "github.com/unikraft/governance/internal/repo"
  "github.com/unikraft/governance/internal/pair"
)

type SyncPrConfig struct {
  numMaintainers int
  numReviewers   int
  noLabels       bool
  repo           string
  prId           int
}

type PullRequest struct {
  pr   *github.PullRequest
  repo  repo.Repository
  teams map[string]*team.Team
}

type RepoTeams struct {
  repo  repo.Repository
  teams map[string]*team.Team
}

var (
  syncPrConfig = &SyncPrConfig{}
  syncPrCmd *cobra.Command
  maintainerWorkload = make(map[string]int)
  reviewerWorkload = make(map[string]int)
  repoDirs = make(map[string]string)
)

func init() {
  syncPrCmd = &cobra.Command{
    Use:                   "sync-pr [OPTIONS] [REPO [PRID]]",
    Short:                 "Synchronise one or many Pull Requests",
    Run:                   doSyncPrCmd,
    Args:                  cobra.MaximumNArgs(2),
    DisableFlagsInUseLine: true,
  }

  syncPrCmd.PersistentFlags().IntVarP(
    &syncPrConfig.numMaintainers,
    "num-maintainers",
    "A",
    1,
    "Number of maintainers for the PR",
  )

  syncPrCmd.PersistentFlags().IntVarP(
    &syncPrConfig.numReviewers,
    "num-reviewers",
    "R",
    1,
    "Number of reviewers for the PR",
  )

  syncPrCmd.PersistentFlags().BoolVar(
    &syncPrConfig.noLabels,
    "no-labels",
    false,
    "Do not set labels on this PR",
  )
}

// doSyncPrCmd
func doSyncPrCmd(cmd *cobra.Command, args []string) {
  if len(args) > 0 {
    // Check to determine if provided argument is a local git folder
    if _, err := os.Stat(args[0]); !os.IsNotExist(err) {
      basename := filepath.Base(args[0])
      r := repo.FindRepoByName(basename, Repos)
      if r == nil {
        // If we can't figure out the repo based on its folder name, let's try
        // and see if its a Git repo and determine its name by its remotes
        d, err := git.PlainOpen(args[0])
        if err != nil {
          log.Fatalf("unknown repo: %s", args[0])
          os.Exit(1)
        }

        // No remotes
        remotes, err := d.Remotes()
        if err != nil {
          log.Fatalf("unknown repo: %s", args[0])
          os.Exit(1)
        }

        // No or too many remotes
        if len(remotes) == 0 || len(remotes) > 1 {
          log.Fatalf("unknown repo: %s", args[0])
          os.Exit(1)
        }

        // No or too many URLs on remote
        config := remotes[0].Config()
        if len(config.URLs) > 1 {
          log.Fatalf("unknown repo: %s", args[0])
          os.Exit(1)
        }

        // Invalid remote URL
        uri, err := url.ParseRequestURI(config.URLs[0])
        if err != nil {
          log.Fatalf("unknown repo: %s", args[0])
          os.Exit(1)
        }

        r = repo.FindRepoByName(filepath.Base(uri.Path), Repos)
        if r == nil {
          log.Fatalf("unknown repo: %s", args[0])
          os.Exit(1)
        }
      }

      syncPrConfig.repo = r.Fullname()
      repoDirs[r.Fullname()] = args[0]

    // Check to determine if provided argument is a remote git repo
    } else if uri, err := url.ParseRequestURI(args[0]); err == nil {
      basename := filepath.Base(uri.Path)
      r := repo.FindRepoByName(basename, Repos)
      if r == nil {
        log.Fatalf("unknown repo: %s", args[0])
        os.Exit(1)
      }

      localRepo := path.Join(globalConfig.tempDir, basename)

      if _, err := os.Stat(localRepo); os.IsNotExist(err) {
        log.Debugf("Cloning remote git repository: %s to %s", args[0], localRepo)
        _, err := git.PlainClone(localRepo, false, &git.CloneOptions{
          URL: args[0],
        })
        if err != nil {
          log.Fatalf("could not clone repository: %s", err)
          os.Exit(1)
        }
      }

      syncPrConfig.repo = r.Fullname()
      repoDirs[r.Fullname()] = localRepo

    } else {
      repo := repo.FindRepoByName(args[0], Repos)
      if repo == nil {
        log.Fatalf("unknown repo: %s", args[0])
        os.Exit(1)
      }

      syncPrConfig.repo = args[0]
    }
  }

  if len(args) > 1 {
    i, err := strconv.Atoi(args[1])
    if err != nil {
      log.Fatalf("Could not convert PRID to integer: %s", err)
      os.Exit(1)
    }
    syncPrConfig.prId = i
  }

  teams, err := team.NewListOfTeamsFromPath(
    globalConfig.ghApi,
    globalConfig.githubOrg,
    globalConfig.teamsDir,
  )
  if err != nil {
    log.Fatalf("could not parse teams: %s", err)
    os.Exit(1)
  }

  log.Debug("Reversing the relationship between teams and repos...")
  repoTeams := make(map[string]RepoTeams)
  for _, t := range teams {
    for _, r := range t.Repositories {
      // Do not parse repositories if we requested a specific and it does not
      // match
      if len(syncPrConfig.repo) > 0 && !r.NameEquals(syncPrConfig.repo) {
        continue
      }

      if repoTeam, ok := repoTeams[r.Fullname()]; ok {
        repoTeam.teams[t.Fullname()] = t
      } else {
        // Use thhe the initialised repository
        r2 := repo.FindRepoByName(r.Fullname(), Repos)
        if r2 != nil {
          r = *r2
        }

        repoTeams[r.Fullname()] = RepoTeams{
          repo:  r,
          teams: map[string]*team.Team{
            t.Fullname(): t,
          },
        }
      }
    }

    // Populate global lists of workloads for both maintainers and reviewers
    for _, m := range t.Maintainers {
      if _, ok := maintainerWorkload[m.Github]; !ok {
        maintainerWorkload[m.Github] = 0
      }
    }

    for _, m := range t.Reviewers {
      if _, ok := reviewerWorkload[m.Github]; !ok {
        reviewerWorkload[m.Github] = 0
      }
    }
  }

  log.Debugf("Determining the workload of all maintainers and reviewers...")
  for _, r := range repoTeams {
    // Get a list of all open PRs
    prs, err := globalConfig.ghApi.ListOpenPullRequests(r.repo.Fullname())
    if err != nil {
      log.Fatalf("could not retrieve pull requests: %s", err)
      os.Exit(1)
    }

    for _, pr := range prs {
      maintainers, err := globalConfig.ghApi.GetMaintainersOnPr(r.repo.Fullname(), *pr.Number)
      if err != nil {
        log.Fatalf("could not get maintainers on pull requests: %s", err)
        os.Exit(1)
      }

      for _, maintainer := range maintainers {
        if _, ok := maintainerWorkload[maintainer]; !ok {
          maintainerWorkload[maintainer] = 0
        }

        maintainerWorkload[maintainer]++
      }

      reviewers, err := globalConfig.ghApi.GetReviewersOnPr(r.repo.Fullname(), *pr.Number)
      if err != nil {
        log.Fatalf("could not get reviewers on pull requests: %s", err)
        os.Exit(1)
      }

      for _, reviewer := range reviewers {
        if _, ok := reviewerWorkload[reviewer]; !ok {
          reviewerWorkload[reviewer] = 0
        }

        reviewerWorkload[reviewer]++
      }
    }
  }

  relevantPrs := make(map[string]map[int]*PullRequest)

  log.Debugf("Calculating lists of potential reviewers and maintainers...")
  for _, r := range repoTeams {
    // Get a list of all open PRs
    prs, err := globalConfig.ghApi.ListOpenPullRequests(r.repo.Fullname())
    if err != nil {
      log.Fatalf("could not retrieve pull requests: %s", err)
      os.Exit(1)
    }

    for _, pr := range prs {
      if syncPrConfig.prId > 0 && *pr.Number != syncPrConfig.prId {
        continue
      }

      // Ignore draft PRs
      if *pr.Draft {
        continue
      }

      if _, ok := relevantPrs[r.repo.Fullname()]; !ok {
        relevantPrs[r.repo.Fullname()] = make(map[int]*PullRequest)
      }

      relevantPrs[r.repo.Fullname()][*pr.Number] = &PullRequest{
        pr:    pr,
        repo:  r.repo,
        teams: r.teams,
      }
    }
  }

  for repoName, prs := range relevantPrs {
    if len(prs) == 0 && syncPrConfig.prId > 0 {
      log.Fatalf("could not find pr with id=%d for repo=%s", syncPrConfig.prId, repoName)
      os.Exit(1)

    } else if len(prs) == 0 {
      log.WithFields(log.Fields{
        "repo": repoName,
      }).Infof("no open pull requests")
      continue
    }

    localRepo, ok := repoDirs[repoName]
    if !ok {
      localRepo = path.Join(globalConfig.tempDir, repoName)
      repoDirs[repoName] = localRepo
    }

    // Check if we have a copy of the repo locally, we'll use it in the next
    // step when checking CODEOWNERS
    if _, err := os.Stat(localRepo); os.IsNotExist(err) {
      log.Debugf("Cloning remote git repositeory: %s to %s", args[0], localRepo)
      _, err := git.PlainClone(localRepo, false, &git.CloneOptions{
        URL: args[0],
      })
      if err != nil {
        log.Fatalf("could not clone repository: %s", err)
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

      if useCodeownersErr == nil {
        log.WithFields(log.Fields{
          "repo": pr.repo.Fullname(),
        }).Debugf("Repo uses CODEOWNERS")

        // Retrieve a list of modofied files in this PR
        localDiffFile := path.Join(globalConfig.tempDir, fmt.Sprintf("%s-%d.diff",
          pr.repo.Fullname(),
          prId,
        ))

        if _, err := os.Stat(localDiffFile); os.IsNotExist(err) {
          log.Debugf("Saving %s to %s...", *pr.pr.DiffURL, localDiffFile)
          err = utils.DownloadFile(localDiffFile, *pr.pr.DiffURL)
          if err != nil {
            log.Fatalf("could not download pull request on repo=%s with pr_id=%d diff: %s", pr.repo.Fullname(), prId, err)
            os.Exit(1)
          }
        }

        d, err := ioutil.ReadFile(localDiffFile)
        if err != nil {
          log.Fatalf("could not read diff file for request on repo=%s with pr_id=%d diff: %s", pr.repo.Fullname(), prId, err)
          os.Exit(1)
        }

        diff, err := diffparser.Parse(string(d))
        if err != nil {
          log.Fatalf("could not parse diff from pull request on repo=%s with pr_id=%d: %s", pr.repo.Fullname(), prId, err)
          os.Exit(1)
        }

        for _, f := range diff.Files {
          var owners []string
          if len(f.OrigName) > 0 {
            owners = append(owners, co.Owners(f.OrigName)...)
          }
          if len(f.NewName) > 0 {
            owners = append(owners, co.Owners(f.NewName)...)
          }

          for _, o := range owners {
            codeTeam := team.FindTeamByName(o, Teams)
            if codeTeam == nil {
              continue
            }

            // Add the team to the repository
            if _, ok := pr.teams[codeTeam.Fullname()]; !ok {
              log.WithFields(log.Fields{
                "team": codeTeam.Fullname(),
              }).Debugf("Adding extra team from CODEOWNERS...")

              pr.teams[codeTeam.Fullname()] = codeTeam
            }
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

      err := updatePrWithPossibleMaintainersAndReviewers(
        repoName,
        prId,
        maintainers,
        reviewers,
      )
      if err != nil {
        log.Fatalf("could not update repo=%s pr_id=%d: %s", repoName, prId, err)
        os.Exit(1)
      }
    }
  }
}

func popLeastStressedMaintainer(subset []string) string {
  maintainers := make(map[string]int)

  for _, username := range subset {
    if _, ok := maintainerWorkload[username]; !ok {
      maintainerWorkload[username] = 0
    }

    maintainers[username] = maintainerWorkload[username]
  }

  sorted := pair.RankByWorkload(maintainers)

  least := sorted[0].Key
  maintainerWorkload[least]++
  return least
}

func popLeastStressedReviewer(subset []string) string {
  reviewers := make(map[string]int)

  for _, username := range subset {
    if _, ok := reviewerWorkload[username]; !ok {
      reviewerWorkload[username] = 0
    }

    reviewers[username] = reviewerWorkload[username]
  }

  sorted := pair.RankByWorkload(reviewers)

  least := sorted[0].Key
  reviewerWorkload[least]++
  return least
}

func updatePrWithPossibleMaintainersAndReviewers(repo string, prId int, possibleMaintainers []string, possibleReviewers []string) error {
  log.WithFields(log.Fields{
    "repo": repo,
    "pr_id": prId,
    // "maintainers": possibleMaintainers,
    // "reviewers": possibleReviewers,
  }).Infof("Assigning reviewer(s) and maintainer(s) to pull request...")

  if len(possibleMaintainers) == 0 {
    return fmt.Errorf("could not assign reviewers as none provided")
  }
  if len(possibleReviewers) == 0 {
    return fmt.Errorf("could not assign reviewers as none provided")
  }

  maintainers, err := globalConfig.ghApi.GetMaintainersOnPr(repo, prId)
  if err != nil {
    return err
  }

  if len(maintainers) == 0 {
    for i := 0; i < syncPrConfig.numMaintainers; i++ {
      m := popLeastStressedMaintainer(possibleMaintainers)
      maintainers = append(maintainers, m)

      log.WithFields(log.Fields{
        "maintainer": m,
      }).Info("Assigning maintainer...")
    }

    if !globalConfig.dryRun {
      err := globalConfig.ghApi.AddMaintainersToPr(repo, prId, maintainers)
      if err != nil {
        log.Fatalf("could not add maintainers to repo=%s pr_id=%d: %s", repo, prId, err)
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

  log.WithFields(log.Fields{
    "repo": repo,
    "pr_id": prId,
    "maintainers": maintainers,
  }).Debugf("Assigned maintainers")

  var reviewers []string

  // Run a check to see if the PR has already received reviews
  r, _ := globalConfig.ghApi.GetReviewUsersOnPr(repo, prId)
  if len(r) > 0 {
    reviewers = append(reviewers, r...)
  }

  r, err = globalConfig.ghApi.GetReviewersOnPr(repo, prId)
  if err != nil {
    return err
  }
  if len(r) > 0 {
    reviewers = append(reviewers, r...)
  }

  if len(reviewers) == 0 {
    for i := len(reviewers); i < syncPrConfig.numReviewers; i++ {
      r := popLeastStressedReviewer(possibleReviewers)
      reviewers = append(reviewers, r)

      log.WithFields(log.Fields{
        "reviewer": r,
      }).Info("Assigning reviewer...")
    }

    if !globalConfig.dryRun {
      err := globalConfig.ghApi.AddReviewersToPr(repo, prId, reviewers)
      if err != nil {
        log.Fatalf("could not add reviewer to repo=%s pr_id=%d: %s", repo, prId, err)
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
