package team
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
  "fmt"
  "strings"

  log "github.com/sirupsen/logrus"
  gh "github.com/google/go-github/v32/github"

  "github.com/unikraft/governance/internal/user"
)

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

  var githubTeam *gh.Team
  var parentGithubTeam *gh.Team

  // Check if the parent exists.  Note, we may have a dependency problem here.
  if t.Parent != "" {
    if t.ParentTeam != nil {
      // Synchronise the parent now so that information for the child is correct
      // and up-to-date.
      err = t.ParentTeam.Sync()
      if err != nil {
        return fmt.Errorf("could not synchronize parent: %s", err)
      }
    }

    parentGithubTeam, err = t.ghApi.FindTeam(t.Org, t.Parent)
    if err != nil {
      return err
    }
  }

  log.Infof("Synchronising @%s/%s...", t.Org, t.Name)

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
  log.Infof(" >>> Updating team details...")
  githubTeam, err = t.ghApi.CreateOrUpdateTeam(
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

  log.Infof(" >>> Synchronising team members...")
  err = t.ghApi.SyncTeamMembers(
    t.Name,
    string(user.Member),
    members,
  )
  if err != nil {
    return fmt.Errorf("could not synchronise team members: %s", err)
  }

  if len(maintainers) > 0 {
    maintainersTeamName := fmt.Sprintf("%ss-%s", string(user.Maintainer), t.shortName)
    log.Infof("Synchronising @%s/%s...", t.Org, maintainersTeamName)

    // Create or update a sub-team with list of maintainers
    log.Infof(" >>> Updating team details...")
    _, err := t.ghApi.CreateOrUpdateTeam(
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
    log.Infof(" >>> Synchronising team members...")
    err = t.ghApi.SyncTeamMembers(
      maintainersTeamName,
      string(user.Maintainer),
      maintainers,
    )
    if err != nil {
      return fmt.Errorf("could not synchronize team members: %s", err)
    }
  }

  if len(reviewers) > 0 {
    reviewersTeamName := fmt.Sprintf("%ss-%s", string(user.Reviewer), t.shortName)
    log.Infof("Synchronising @%s/%s...", t.Org, reviewersTeamName)

    // Create or update a sub-team with list of reviewers
    log.Infof(" >>> Updating team details...")
    _, err := t.ghApi.CreateOrUpdateTeam(
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
    log.Infof(" >>> Synchronising team members...")
    err = t.ghApi.SyncTeamMembers(
      reviewersTeamName,
      string(user.Member),
      reviewers,
    )
    if err != nil {
      return fmt.Errorf("could not synchronize team members: %s", err)
    }
  }

  t.hasSynced = true
  return nil
}
