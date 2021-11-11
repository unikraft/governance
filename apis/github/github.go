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
package github

import (
  "fmt"
  "log"
  "context"
  "net/url"
  "net/http"
  "crypto/tls"

  "golang.org/x/oauth2"
  "github.com/google/go-github/v32/github"

  "github.com/unikraft/governance/utils"
)

// GithubClient containing the necessary information to authenticate and perform
// actions against the REST API.
type GithubClient struct {
  Org      string
  Client  *github.Client
}

// Github interface representing the desired functions for this resource.
type Github interface {
  FindTeam(org string, team string) (*github.Team, error)
  CreateOrUpdateTeam(name, description string, parentTeamID int64, privacy *string, maintainers, repos []string) (*github.Team, error)
}

var (
  userCache map[string]*github.User
)

// NewGitHubClient for creating a new instance of the client.
func NewGithubClient(org string, accessToken string, skipSSL bool, githubEndpoint string) (*GithubClient, error) {
  var ctx context.Context

  if skipSSL {
    insecureClient := &http.Client{
      Transport: &http.Transport{
        TLSClientConfig: &tls.Config{
          InsecureSkipVerify: true,
        },
      },
    }

    ctx = context.WithValue(context.TODO(), oauth2.HTTPClient, insecureClient)
  } else {
    ctx = context.TODO()
  }

  var client *github.Client
  oauth2Client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
    &oauth2.Token{
      AccessToken: accessToken,
    },
  ))

  if githubEndpoint != "" {
    endpoint, err := url.Parse(githubEndpoint)
    if err != nil {
      return nil, fmt.Errorf("failed to parse v3 endpoint: %s", err)
    }

    client, err = github.NewEnterpriseClient(
      endpoint.String(),
      endpoint.String(),
      oauth2Client,
    )
    if err != nil {
      return nil, err
    }
  } else {
    client = github.NewClient(oauth2Client)
  }

  userCache = make(map[string]*github.User)

  return &GithubClient{
    Org:    org,
    Client: client,
  }, nil
}

// FindTeam takes an organization name and team name and returns a detailed
// struct with information about the team.
func (c *GithubClient) FindTeam(org string, team string) (*github.Team, error) {
	opts := &github.ListOptions{}

	for {
		teams, resp, err := c.Client.Teams.ListTeams(context.TODO(), org, opts)
		if err != nil {
			return nil, err
		}

		for _, t := range teams {
			if t.GetName() == team {
				return t, nil
			}
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

  return nil, fmt.Errorf("could not find team: @%s/%s", org, team)
}

// FindUser takes a Github username and returns a detaled object with
// information about the user.
func (c *GithubClient) FindUser(username string) (*github.User, error) {
  if user, ok := userCache[username]; ok {
    return user, nil
  }

  user, _, err := c.Client.Users.Get(
    context.TODO(),
    username,
  )
  if err != nil {
    return nil, fmt.Errorf("could not find user: %s: %s", username, err)
  }

  userCache[username] = user

  return user, nil
}

func (c *GithubClient) CreateOrUpdateTeam(name, description string, parentTeamID int64, privacy *string, maintainers, repos []string) (*github.Team, error) {
  newTeam := github.NewTeam{
    Name:          name,
    Description:  &description,
    Maintainers:   maintainers,
    RepoNames:     repos,
  }

  if parentTeamID > 0 {
    newTeam.ParentTeamID = &parentTeamID
  }

  if privacy != nil {
    newTeam.Privacy = privacy
  }

  var err error
  var team *github.Team

  // Check if the team already exists
  team, err = c.FindTeam(c.Org, name)
  if err != nil {
    team, _, err = c.Client.Teams.CreateTeam(
      context.TODO(),
      c.Org,
      newTeam,
    )
  } else {
    removeParent := false
    if parentTeamID < 0 {
      removeParent = true
    }
    team, _, err = c.Client.Teams.EditTeamBySlug(
      context.TODO(),
      c.Org,
      name,
      newTeam,
      removeParent,
    )
  }

  if err != nil {
    return nil, err
  }

  return team, nil
}

func (c *GithubClient) ListOrgMembers(role string) ([]string, error) {
  var members []string

  users, _, err := c.Client.Organizations.ListMembers(
    context.TODO(),
    c.Org,
    &github.ListMembersOptions{
      Role: role,
    },
  )
  if err != nil {
    return nil, fmt.Errorf("could not list org members: %s", err)
  }

  for _, user := range users {
    userCache[*user.Login] = user
    members = append(members, *user.Login)
  }

  return members, nil
}

func (c *GithubClient) SyncTeamMembers(team, role string, members []string) error {
  var allCurrentUsernames []string
	opts := github.ListOptions{}

  for {
    more, resp, err := c.Client.Teams.ListTeamMembersBySlug(
      context.TODO(),
      c.Org,
      team,
      &github.TeamListTeamMembersOptions{
        // Role: role,
        ListOptions: opts,
      },
    )
    if err != nil {
      return err
    }

    for _, user := range more {
      allCurrentUsernames = append(allCurrentUsernames, *user.Login)
    }

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
  }

  usernamesToRemove := utils.Difference(allCurrentUsernames, members)

  if len(usernamesToRemove) > 0 {
    for _, user := range usernamesToRemove {
      log.Printf(" >>>>>> Removing: %s...", user)
      resp, err := c.Client.Teams.RemoveTeamMembershipBySlug(
        context.TODO(),
        c.Org,
        team,
        user,
      )
      if err != nil {
        fmt.Printf("%#v\n\n", resp.Request)

        return fmt.Errorf("could not remove user: %s: %s", user, err)
      }
    }
  }

  usernamesToAdd := utils.Difference(members, allCurrentUsernames)

  if len(usernamesToAdd) > 0 {
    for _, user := range usernamesToAdd {
      log.Printf(" >>>>>> Adding: %s...", user)
      _, _, err := c.Client.Teams.AddTeamMembershipBySlug(
        context.TODO(),
        c.Org,
        team,
        user,
        &github.TeamAddTeamMembershipOptions{
          Role: role,
        },
      )
      if err != nil {
        return fmt.Errorf("could not add user: %s: %s", user, err)
      }
    }
  }

  return nil
}
