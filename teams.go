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
  "strings"
  "io/ioutil"

  "gopkg.in/yaml.v2"

  ghApi "github.com/google/go-github/v32/github"

  "github.com/unikraft/governance/apis/github"
)

type UserRole string

const (
  Admin      UserRole = "admin"
  Maintainer UserRole = "maintainer"
  Reviewer   UserRole = "reviewer"
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

type TeamPrivacy string

const (
  TeamClosed TeamPrivacy = "closed"
  TeamSecret TeamPrivacy = "secret"
)

type Team struct {
  Name           string      `yaml:"name,omitempty"`
  Type           TeamType    `yaml:"type,omitempty"`
  Privacy        TeamPrivacy `yaml:"privacy,omitempty"`
  Parent         string      `yaml:"parent,omitempty"`
  parentTeam    *Team        // private reference to parent team
  Description    string      `yaml:"description,omitempty"`
  CodeReview     CodeReview  `yaml:"code_review,omitempty"`
  Maintainers  []User        `yaml:"maintainers,omitempty"`
  Reviewers    []User        `yaml:"reviewers,omitempty"`
  Members      []User        `yaml:"members,omitempty"`
  Repositories []Repository  `yaml:"repositories,omitempty"`
  hasSynced      bool
  shortName      string
}

var (
  gh      *github.GithubClient
  teams []*Team
)

func findTeamByName(a string) *Team {
  for _, b := range teams {
    if b.Name == a {
      return b
    }
  }

  return nil
}

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

  // Now let's check if all maintainers, reviewers and members have at least
  // their Github username provided.
  users := append(t.Maintainers, t.Reviewers...)
  users = append(users, t.Members...)
  for _, user := range users {
    if user.Github == "" {
      return fmt.Errorf("user does not have github username: %s", user.Name)
    }
  }

  return nil
}

func (t *Team) Sync() error {
  if t.hasSynced {
    return nil
  }

  t.shortName = t.Name

  var err error
  t.hasSynced = false

  // Determine the team type if unset
  if t.Type == "" {
    for _, prefix := range []TeamType{SIGTeam, MaintainersTeam, ReviewersTeam} {
      if strings.HasPrefix(t.Name, string(prefix)) {
        t.shortName = strings.TrimPrefix(t.Name, fmt.Sprintf("%s-", prefix))
        t.Type = prefix
        break
      }
    }

    // If the type is still unset...
    if t.Type == "" {
      t.Type = MiscTeam
    }
  }

  var githubTeam *ghApi.Team
  var parentGithubTeam *ghApi.Team

  // Check if the parent exists.  Note, we may have a dependency problem here.
  if t.Parent != "" {
    if t.parentTeam != nil {
      // Synchronise the parent now so that information for the child is correct
      // and up-to-date.
      err = t.parentTeam.Sync()
      if err != nil {
        return fmt.Errorf("could not synchronize parent: %s", err)
      }
    }

    parentGithubTeam, err = gh.FindTeam(gh.Org, t.Parent)
    if err != nil {
      return err
    }
  }

  log.Printf("Synchronising @%s/%s...", gh.Org, t.Name)

  var maintainers []string
  var reviewers []string
  var members []string
  var repos []string

  for _, maintainer := range t.Maintainers {
    maintainers = append(maintainers, maintainer.Github)
    members = append(members, maintainer.Github)
  }

  for _, reviewer := range t.Reviewers {
    reviewers = append(reviewers, reviewer.Github)
    members = append(members, reviewer.Github)
  }

  for _, member := range t.Members {
    members = append(members, member.Github)
  }

  for _, repo := range t.Repositories {
    repos = append(repos, repo.Name)
  }

  // Github's Go API is a bit stupid... There is a type mis-match in their
  // Golang SDK when it comes to the "privacy" attribute (either 'closed' or
  // 'private') and so we must pass a pointer to a string, rather than the
  // actual string.
  p := string(t.Privacy)
  var parentTeamID int64

  if parentGithubTeam != nil {
    parentTeamID = *parentGithubTeam.ID
  } else {
    parentTeamID = -1
  }

  // Check if the team already exists, if it does not, we must create it.
  log.Printf(" >>> Updating team details...")
  githubTeam, err = gh.CreateOrUpdateTeam(
    t.Name,
    t.Description,
    parentTeamID,
    &p,
    maintainers,
    repos,
  )
  if err != nil {
    return fmt.Errorf("could not create or update team: %s", err)
  }

  // Create list of all maintainers
  var allMaintainerGithubUsernames []string
  for _, user := range t.Maintainers {
    allMaintainerGithubUsernames = append(
      allMaintainerGithubUsernames,
      user.Github,
    )
  }

  log.Printf(" >>> Synchronising team members...")
  err = gh.SyncTeamMembers(
    t.Name,
    string(Member),
    members,
  )

  if len(maintainers) > 0 {
    maintainersTeamName := fmt.Sprintf("%ss-%s", string(Maintainer), t.shortName)
    log.Printf("Synchronising @%s/%s...", gh.Org, maintainersTeamName)

    // Create or update a sub-team with list of maintainers
    log.Printf(" >>> Updating team details...")
    _, err := gh.CreateOrUpdateTeam(
      maintainersTeamName,
      fmt.Sprintf("%s maintainers", t.Name),
      *githubTeam.ID,
      &p,
      maintainers,
      repos,
    )
    if err != nil {
      return fmt.Errorf("could not create or update team: %s", err)
    }

    // Add and remove these usernames from the second-level `maintainers-` group
    log.Printf(" >>> Synchronising team members...")
    err = gh.SyncTeamMembers(
      maintainersTeamName,
      string(Maintainer),
      maintainers,
    )
    if err != nil {
      return fmt.Errorf("could not synchronize team members: %s", err)
    }
  }

  if len(reviewers) > 0 {
    reviewersTeamName := fmt.Sprintf("%ss-%s", string(Reviewer), t.shortName)
    log.Printf("Synchronising @%s/%s...", gh.Org, reviewersTeamName)

    // Create or update a sub-team with list of reviewers
    log.Printf(" >>> Updating team details...")
    _, err := gh.CreateOrUpdateTeam(
      reviewersTeamName,
      fmt.Sprintf("%s reviewers", t.Name),
      *githubTeam.ID,
      &p,
      nil,
      repos,
    )
    if err != nil {
      return fmt.Errorf("could not create or update team: %s", err)
    }

    // Add and remove these usernames from the second-level `reviewers-` group
    log.Printf(" >>> Synchronising team members...")
    err = gh.SyncTeamMembers(
      reviewersTeamName,
      string(Member),
      reviewers,
    )
    if err != nil {
      return fmt.Errorf("could not synchronize team members: %s", err)
    }
  }

  t.hasSynced = true
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

  // To solve a potential dependency problem where teams are dependent on teams
  // which do not exist, we are going to populate a list "processed" teams first
  // and then check if any of the teams has a parent which does not exist in the
  // list which we have just populated.

  // Iterate through all files and populate a list of known teams.
  for _, file := range files {
    team := &Team{}
    err = team.Parse(path.Join(teamsDir, file.Name()))
    if err != nil {
      log.Fatalf("could not parse teams file: %s", err)
    }

    teams = append(teams, team)
  }

  // Now iterate through known teams and match parents
  for _, team := range teams {
    if team.Parent != "" {
      parent := findTeamByName(team.Parent)
      if parent != nil {
        team.parentTeam = parent
        break
      } else {
        // We might be lucky... it may exist upstream when we later call the
        // Github API.  If it doesn't then we're in trouble...
        log.Printf("warning: cannot find parent from provided teams: %s", team.Parent)
      }
    }
  }

  // Finally, synchronise all teams now that we have linked relevant teams
  for _, team := range teams {
    err = team.Sync()
    if err != nil {
      log.Fatalf("could not syncronise team: %s: %s", team.Name, err)
      os.Exit(1)
    }
  }
}
