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
  "log"
  "path"
  "io/ioutil"

  "gopkg.in/yaml.v2"

  "github.com/unikraft/governance/apis/github"
)

type UserRole string

const (
  Maintainer UserRole = "maintainer"
  Member     UserRole = "member"
)

type User struct {
  Name    string   `yaml:"name,omitempty"`
  Email   string   `yaml:"email,omitempty"`
  Github  string   `yaml:"github,omitempty"`
  Discord string   `yaml:"discord,omitempty"`
  Role    UserRole `yaml:"role,omitempty"`
}

type CodeReviewAlgorithm string

const (
  RoundRobin  CodeReviewAlgorithm = "rr"
  LoadBalance CodeReviewAlgorithm = "lb"
)

type CodeReview struct {
  NumReviewers           int                 `yaml:"num_reviewers,omitempty"`
  Algorithm              CodeReviewAlgorithm `yaml:"algorithm,omitempty"`
  NeverAssign          []User                `yaml:"never_assign,omitempty"`
  DontNotifyTeam         bool                `yaml:"dont_notify_team,omitempty"`
  IncludeChildTeams      bool                `yaml:"include_child_teams,omitempty"`
  RemoveReviewRequest    bool                `yaml:"remove_review_request,omitempty"`
  CountExistingMembers   bool                `yaml:"count_existing_members,omitempty"`
}

type PermissionLevel string

const (
  PermissionRead     PermissionLevel = "read"
  PermissionTriage   PermissionLevel = "triage"
  PermissionWrite    PermissionLevel = "write"
  PermissionMaintain PermissionLevel = "maintain"
  PermissionAdmin    PermissionLevel = "admin"
)

type Repository struct {
  Name            string          `yaml:"name,omitempty"`
  PermissionLevel PermissionLevel `yaml:"permission,omitempty"`
}

type TeamType string

const (
  SIGTeam         TeamType = "sig"
  MaintainersTeam TeamType = "maintainers"
  ReviewersTeam   TeamType = "reviewers"
  MiscTeam        TeamType = "misc"
)

type Team struct {
  Name           string     `yaml:"name,omitempty"`
  Type           TeamType   `yaml:"type,omitempty"`
  Parent         string     `yaml:"parent,omitempty"`
  Description    string     `yaml:"description,omitempty"`
  CodeReview     CodeReview `yaml:"code_review,omitempty"`
  Members      []User       `yaml:"members,omitempty"`
  Repositories []Repository `yaml:"repositories,omitempty"`
}

var (
  gh      *github.GithubClient
  teams []*Team
)

func (t *Team) Parse(path string) error {
  yamlFile, err := ioutil.ReadFile(path)
  if err != nil {
    return fmt.Errorf("could not open yaml file: %s", err)
  }

  err = yaml.Unmarshal(yamlFile, t)
  if err != nil {
    return fmt.Errorf("could not unmarshal yaml file: %s", err)
  }

  // Let's perform a sanity check and check if we have at least the name of the
  // team.
  if t.Name == "" {
    return fmt.Errorf("team name not provided for %s", path)
  }

  return nil
}

func setupGithubClient() error {
  var err error

  githubOrg := os.Getenv("GITHUB_ORG")
  if githubOrg == "" {
    return fmt.Errorf("GITHUB_ORG not set")
  }

  githubToken := os.Getenv("GITHUB_TOKEN")
  if githubToken == "" {
    return fmt.Errorf("GITHUB_TOKEN token not set")
  }

  var githubSkipSSL bool
  if os.Getenv("GITHUB_SKIP_SSL") == "true" {
    githubSkipSSL = true
  } else {
    githubSkipSSL = false
  }

  githubEndpoint := os.Getenv("GITHUB_ENDPOINT")

  gh, err = github.NewGithubClient(
    githubOrg,
    githubToken,
    githubSkipSSL,
    githubEndpoint,
  )
  if err != nil {
    return fmt.Errorf("could not create github client: %s", err)
  }

  return nil
}

func main() {
  var err error
  teamsDir := "./teams"

  if len(os.Args) > 1 {
    teamsDir = os.Args[1]
  }

  if _, err := os.Stat(teamsDir); os.IsNotExist(err) {
    log.Fatalf("could not read find teams directory: %s", err)
    os.Exit(1)
  }

  files, err := ioutil.ReadDir(teamsDir)
  if err != nil {
    log.Fatalf("could not read directory: %s", err)
    os.Exit(1)
  }

  err = setupGithubClient()
  if err != nil {
    log.Fatalf("could not setup github client: %s", err)
    os.Exit(1)
  }

  // Iterate through all files and populate a list of known teams.
  for _, file := range files {
    team := &Team{}
    err = team.Parse(path.Join(teamsDir, file.Name()))
    if err != nil {
      log.Fatalf("could not parse teams file: %s", err)
    }
  }
}
