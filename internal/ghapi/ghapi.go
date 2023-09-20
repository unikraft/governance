// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package ghapi

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"

	"github.com/unikraft/governance/utils"
)

// GithubClient containing the necessary information to authenticate and perform
// actions against the REST API.
type GithubClient struct {
	Org    string
	Client *github.Client
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
		Name:        name,
		Description: &description,
		Maintainers: maintainers,
		RepoNames:   repos,
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
	_, err = c.FindTeam(c.Org, name)
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

// ListPullRequests returns the list of pull requests for the configured repo
func (c *GithubClient) ListOpenPullRequests(repo string) ([]*github.PullRequest, error) {
	var allPrs []*github.PullRequest
	opts := github.ListOptions{}

	for {
		prs, resp, err := c.Client.PullRequests.List(
			context.TODO(),
			c.Org,
			repo,
			&github.PullRequestListOptions{
				State:       "open",
				ListOptions: opts,
			},
		)
		if err != nil {
			return allPrs, err
		}

		allPrs = append(allPrs, prs...)

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	return allPrs, nil
}

// GetPullRequest returns the specific pull request given its ID relative to the
// configured repo
func (c *GithubClient) GetPullRequest(repo string, prId int) (*github.PullRequest, error) {
	pull, _, err := c.Client.PullRequests.Get(
		context.TODO(),
		c.Org,
		repo,
		prId,
	)

	if err != nil {
		return nil, err
	}

	return pull, nil
}

// GetMaintainersOnPr retrieves a list of GitHub usernames attached as the
// "assignee" (or maintainer) of a particular PR
func (c *GithubClient) GetMaintainersOnPr(repo string, prId int) ([]string, error) {
	pull, err := c.GetPullRequest(repo, prId)
	if err != nil {
		return nil, err
	}

	var maintainers []string

	for _, user := range pull.Assignees {
		maintainers = append(maintainers, *user.Login)
	}

	return maintainers, nil
}

// AddMaintainersToPr adds a list of GitHub usernames as "assignee" to a PR
func (c *GithubClient) AddMaintainersToPr(repo string, prId int, maintainers []string) error {
	_, _, err := c.Client.Issues.AddAssignees(
		context.TODO(),
		c.Org,
		repo,
		prId,
		maintainers,
	)
	if err != nil {
		return err
	}

	return nil
}

// GetReviewersOnPr retrieves a lsit of GitHub usernames attached as the
// reviewer for a particular PR
func (c *GithubClient) GetReviewersOnPr(repo string, prId int) ([]string, error) {
	ghReviewers, _, err := c.Client.PullRequests.ListReviewers(
		context.TODO(),
		c.Org,
		repo,
		prId,
		&github.ListOptions{},
	)
	if err != nil {
		return nil, err
	}

	var reviewers []string

	for _, user := range ghReviewers.Users {
		reviewers = append(reviewers, *user.Login)
	}

	return reviewers, err
}

// GetReviewUsersOnPr retrieves a list of usernames of provided reviews for a
// particular PR
func (c *GithubClient) GetReviewUsersOnPr(repo string, prId int) ([]string, error) {
	reviews, _, err := c.Client.PullRequests.ListReviews(
		context.TODO(),
		c.Org,
		repo,
		prId,
		&github.ListOptions{},
	)
	if err != nil {
		return nil, err
	}

	var reviewers []string

	for _, review := range reviews {
		reviewers = append(reviewers, *review.User.Login)
	}

	return reviewers, err
}

// AddReviewersToPr adds a list of GitHub usernames as reviewers to a PR
func (c *GithubClient) AddReviewersToPr(repo string, prId int, reviewers []string) error {
	_, _, err := c.Client.PullRequests.RequestReviewers(
		context.TODO(),
		c.Org,
		repo,
		prId,
		github.ReviewersRequest{
			// NodeID: ,
			Reviewers: reviewers,
		},
	)

	if err != nil {
		return fmt.Errorf("could not add assignees to PR: %s", err)
	}

	return nil
}

// AddLabelsToPr adds a list of GitHub labels to a PR
func (c *GithubClient) AddLabelsToPr(repo string, prId int, labels []string) error {
	_, _, err := c.Client.Issues.AddLabelsToIssue(
		context.TODO(),
		c.Org,
		repo,
		prId,
		labels,
	)

	if err != nil {
		return fmt.Errorf("could not add labels to PR: %s", err)
	}

	return nil
}
