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
  "path"
  "io/ioutil"

  "github.com/spf13/cobra"
  log "github.com/sirupsen/logrus"

  "github.com/unikraft/governance/apis/github"
  "github.com/unikraft/governance/internal/team"
)

var (
  gh      *github.GithubClient
  teams []*team.Team
  syncTeamsCmd = &cobra.Command{
    Use: "sync-teams",
    Short: "Synchronise teams",
    Run: doSyncTeamsCmd,
  }
)

// doSyncTeamsCmd starts the main system
func doSyncTeamsCmd(cmd *cobra.Command, args []string) {
  var err error

  files, err := ioutil.ReadDir(globalConfig.teamsDir)
  if err != nil {
    log.Fatalf("could not read directory: %s", err)
    os.Exit(1)
  }

  // To solve a potential dependency problem where teams are dependent on teams
  // which do not exist, we are going to populate a list "processed" teams first
  // and then check if any of the teams has a parent which does not exist in the
  // list which we have just populated.

  // Iterate through all files and populate a list of known teams.
  for _, file := range files {
    t, err := team.NewTeamFromYAML(
      globalConfig.ghApi,
      globalConfig.githubOrg,
      path.Join(globalConfig.teamsDir, file.Name()),
    )
    if err != nil {
      log.Fatalf("could not parse teams file: %s", err)
    }

    teams = append(teams, t)
  }

  // Now iterate through known teams and match parents
  for _, t := range teams {
    if t.Parent != "" {
      parent := team.FindTeamByName(t.Parent, teams)
      if parent != nil {
        t.ParentTeam = parent
        break
      } else {
        // We might be lucky... it may exist upstream when we later call the
        // Github API.  If it doesn't then we're in trouble...
        log.Warnf("cannot find parent from provided teams: %s", t.Parent)
      }
    }
  }

  // Finally, synchronise all teams now that we have linked relevant teams
  for _, t := range teams {
    err = t.Sync()
    if err != nil {
      log.Fatalf("could not syncronise team: %s: %s", t.Name, err)
      os.Exit(1)
    }
  }
}
