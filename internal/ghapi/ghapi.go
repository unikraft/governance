// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package ghapi

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
	"kraftkit.sh/log"

	"github.com/unikraft/governance/utils"
)

// GithubClient containing the necessary information to authenticate and perform
// actions against the REST API.
type GithubClient struct {
	client *github.Client
}

var (
	userCache     map[string]*github.User
	userTeamCache map[string][]string
)

// NewGitHubClient for creating a new instance of the client.
func NewGithubClient(ctx context.Context, accessToken string, skipSSL bool, githubEndpoint string) (*GithubClient, error) {
	if skipSSL {
		insecureClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}

		ctx = context.WithValue(ctx, oauth2.HTTPClient, insecureClient)
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

	return &GithubClient{client}, nil
}

// FindTeam takes an organization name and team name and returns a detailed
// struct with information about the team.
func (c *GithubClient) FindTeam(ctx context.Context, org string, team string) (*github.Team, error) {
	opts := &github.ListOptions{}

	for {
		teams, resp, err := c.client.Teams.ListTeams(ctx, org, opts)
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
func (c *GithubClient) FindUser(ctx context.Context, username string) (*github.User, error) {
	if user, ok := userCache[username]; ok {
		return user, nil
	}

	user, _, err := c.client.Users.Get(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("could not find user: %s: %s", username, err)
	}

	userCache[username] = user

	return user, nil
}

func (c *GithubClient) CreateOrUpdateTeam(ctx context.Context, org, name, description string, parentTeamID int64, privacy *string, maintainers, repos []string) (*github.Team, error) {
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
	_, err = c.FindTeam(ctx, org, name)
	if err != nil {
		team, _, err = c.client.Teams.CreateTeam(ctx, org, newTeam)
	} else {
		removeParent := false
		if parentTeamID < 0 {
			removeParent = true
		}
		team, _, err = c.client.Teams.EditTeamBySlug(
			ctx,
			org,
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

func (c *GithubClient) ListOrgMembers(ctx context.Context, org, role string) ([]string, error) {
	var members []string

	users, _, err := c.client.Organizations.ListMembers(
		ctx,
		org,
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

func (c *GithubClient) SyncTeamMembers(ctx context.Context, org, team, role string, members []string) error {
	var allCurrentUsernames []string
	opts := github.ListOptions{}

	for {
		more, resp, err := c.client.Teams.ListTeamMembersBySlug(
			ctx,
			org,
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
			log.G(ctx).Infof("removing: %s...", user)
			resp, err := c.client.Teams.RemoveTeamMembershipBySlug(
				ctx,
				org,
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
			log.G(ctx).
				WithField("user", user).
				WithField("team", fmt.Sprintf("@%s/%s", org, team)).
				Info("adding")
			_, _, err := c.client.Teams.AddTeamMembershipBySlug(
				ctx,
				org,
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
func (c *GithubClient) ListOpenPullRequests(ctx context.Context, org, repo string) ([]*github.PullRequest, error) {
	var allPrs []*github.PullRequest
	opts := github.ListOptions{}

	for {
		prs, resp, err := c.client.PullRequests.List(
			ctx,
			org,
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
func (c *GithubClient) GetPullRequest(ctx context.Context, org, repo string, prId int) (*github.PullRequest, error) {
	pull, _, err := c.client.PullRequests.Get(
		ctx,
		org,
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
func (c *GithubClient) GetMaintainersOnPr(ctx context.Context, org, repo string, prId int) ([]string, error) {
	pull, err := c.GetPullRequest(ctx, org, repo, prId)
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
func (c *GithubClient) AddMaintainersToPr(ctx context.Context, org, repo string, prId int, maintainers []string) error {
	_, _, err := c.client.Issues.AddAssignees(
		ctx,
		org,
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
func (c *GithubClient) GetReviewersOnPr(ctx context.Context, org, repo string, prId int) ([]string, error) {
	ghReviewers, _, err := c.client.PullRequests.ListReviewers(
		ctx,
		org,
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
func (c *GithubClient) GetReviewUsersOnPr(ctx context.Context, org, repo string, prId int) ([]string, error) {
	reviews, _, err := c.client.PullRequests.ListReviews(
		ctx,
		org,
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
func (c *GithubClient) AddReviewersToPr(ctx context.Context, org, repo string, prId int, reviewers []string) error {
	_, _, err := c.client.PullRequests.RequestReviewers(
		ctx,
		org,
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
func (c *GithubClient) AddLabelsToPr(ctx context.Context, org, repo string, prId int, labels []string) error {
	_, _, err := c.client.Issues.AddLabelsToIssue(
		ctx,
		org,
		repo,
		prId,
		labels,
	)

	if err != nil {
		return fmt.Errorf("could not add labels to PR: %s", err)
	}

	return nil
}

// ListPullRequests returns the list of pull requests for the configured repo
func (c *GithubClient) ListPullRequests(ctx context.Context, org, repo string) ([]*github.PullRequest, error) {
	var pulls []*github.PullRequest
	opts := github.ListOptions{}

	for {
		more, resp, err := c.client.PullRequests.List(
			ctx,
			org,
			repo,
			&github.PullRequestListOptions{
				// We want all states so we can sort through them later
				State:       "all",
				ListOptions: opts,
			},
		)
		if err != nil {
			return nil, err
		}

		for _, pull := range more {
			pulls = append(pulls, pull)
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	return pulls, nil
}

// ListPullRequestComments returns the list of comments for the specific pull
// request given its ID relative to the configured repo
func (c *GithubClient) ListPullRequestComments(ctx context.Context, org, repo string, prID int) ([]*github.IssueComment, error) {
	opts := github.ListOptions{}
	var comments []*github.IssueComment

	for {
		more, resp, err := c.client.Issues.ListComments(
			ctx,
			org,
			repo,
			prID,
			&github.IssueListCommentsOptions{
				ListOptions: opts,
			},
		)
		if err != nil {
			return nil, err
		}

		comments = append(comments, more...)

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	return comments, nil
}

// ListPullRequestReviews returns the list of reviews for the specific pull
// request given its ID relative to the configured repo
func (c *GithubClient) ListPullRequestReviews(ctx context.Context, org, repo string, prID int) ([]*github.PullRequestReview, error) {
	opts := &github.ListOptions{}
	var reviews []*github.PullRequestReview

	for {
		more, resp, err := c.client.PullRequests.ListReviews(
			ctx,
			org,
			repo,
			prID,
			opts,
		)
		if err != nil {
			return nil, err
		}

		reviews = append(reviews, more...)

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	return reviews, nil
}

// GetPulLRequestComment returns the specific comment given its unique Github ID
func (c *GithubClient) GetPullRequestComment(ctx context.Context, org, repo string, commentID int64) (*github.IssueComment, error) {
	comment, _, err := c.client.Issues.GetComment(
		ctx,
		org,
		repo,
		commentID,
	)
	if err != nil {
		return nil, err
	}

	return comment, nil
}

// GetPulLRequestReview returns the specific review given its unique Github ID
func (c *GithubClient) GetPullRequestReview(ctx context.Context, org, repo string, prID int, reviewID int64) (*github.PullRequestReview, error) {
	review, _, err := c.client.PullRequests.GetReview(
		ctx,
		org,
		repo,
		prID,
		reviewID,
	)
	if err != nil {
		return nil, err
	}

	return review, nil
}

func (c *GithubClient) SetPullRequestState(ctx context.Context, org, repo string, prID int, state string) error {
	validState := false
	validStates := []string{"open", "closed"}
	for _, s := range validStates {
		if state == s {
			validState = true
		}
	}

	if !validState {
		return fmt.Errorf("invalid pull request state: %s", state)
	}

	_, _, err := c.client.Issues.Edit(
		ctx,
		org,
		repo,
		prID, &github.IssueRequest{
			State: &state,
		},
	)

	return err
}

func (c *GithubClient) DeleteLastPullRequestComment(ctx context.Context, org, repo string, prID int) error {
	comments, err := c.ListPullRequestComments(ctx, org, repo, prID)
	if err != nil {
		return err
	}

	// Retrieve the authenticated user provided by the access token
	user, _, err := c.client.Users.Get(
		ctx,
		"",
	)
	if err != nil {
		return err
	}

	// Only delete the last comment from the same author as the provided token
	var commentID int64
	for _, comment := range comments {
		if *comment.User.ID == *user.ID {
			commentID = *comment.ID
		}
	}

	if commentID > 0 {
		_, err = c.client.Issues.DeleteComment(
			ctx,
			org,
			repo,
			commentID,
		)

		return err
	}

	return nil
}

// AddPullRequestLabels adds the list of labels to the existing set of labels
// given the relative pull request ID to the configure repo
func (c *GithubClient) AddPullRequestLabels(ctx context.Context, org, repo string, prID int, labels []string) error {
	_, _, err := c.client.Issues.AddLabelsToIssue(
		ctx,
		org,
		repo,
		prID,
		labels,
	)

	return err
}

// RemovePullRequestLabels remove the list of labels from the set of existing
// labels given the relative pull request ID to the configured repo
func (c *GithubClient) RemovePullRequestLabels(ctx context.Context, org, repo string, prID int, labels []string) error {
	for _, l := range labels {
		_, err := c.client.Issues.RemoveLabelForIssue(
			ctx,
			org,
			repo,
			prID,
			l,
		)

		if err != nil {
			return err
		}
	}

	return nil
}

// ReplacePullRequestLabels overrides all existing labels with the given set of
// labels for the pull request ID relative to the configured repo
func (c *GithubClient) ReplacePullRequestLabels(ctx context.Context, org, repo string, prID int, labels []string) error {
	_, _, err := c.client.Issues.ReplaceLabelsForIssue(
		ctx,
		org,
		repo,
		prID,
		labels,
	)

	return err
}

// CreatePullRequestComment adds a new comment to the pull request given its
// ID relative to the configured repo
func (c *GithubClient) CreatePullRequestComment(ctx context.Context, org, repo string, prID int, comment string) error {
	_, _, err := c.client.Issues.CreateComment(
		ctx,
		org,
		repo,
		prID,
		&github.IssueComment{
			Body: &comment,
		},
	)
	return err
}

func (c *GithubClient) ListTeamMembers(ctx context.Context, orgTeam string) ([]string, error) {
	org, team, err := parseTeam(orgTeam)
	if err != nil {
		return nil, fmt.Errorf("could not find team: %s", err)
	}

	opts := github.ListOptions{}
	var members []*github.User

	for {
		more, resp, err := c.client.Teams.ListTeamMembersBySlug(
			ctx,
			org,
			team,
			&github.TeamListTeamMembersOptions{
				ListOptions: opts,
			},
		)
		if err != nil {
			return nil, err
		}

		members = append(members, more...)

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	var usernames []string
	for _, member := range members {
		usernames = append(usernames, *member.Login)
	}

	return usernames, nil
}

func (c *GithubClient) UserMemberOfTeam(ctx context.Context, username, team string) (bool, error) {
	if teams, ok := userTeamCache[username]; ok {
		for _, t := range teams {
			if team == t {
				return true, nil
			}
		}

		return false, nil
	}

	members, err := c.ListTeamMembers(ctx, team)
	if err != nil {
		return false, nil
	}

	// Cache request
	for _, member := range members {
		userTeamCache[member] = append(userTeamCache[member], team)
	}

	if teams, ok := userTeamCache[username]; ok {
		for _, t := range teams {
			if team == t {
				return true, nil
			}
		}
	}

	return false, nil
}

// func parseRepository(s string) (string, string, error) {
// 	parts := strings.Split(s, "/")
// 	if len(parts) != 2 {
// 		return "", "", fmt.Errorf("malformed repository")
// 	}
// 	return parts[0], parts[1], nil
// }

func parseTeam(s string) (string, string, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid team format: expected @org/team")
	}
	parts[0] = strings.TrimPrefix(parts[0], "@")
	return parts[0], parts[1], nil
}

// ParseCommentHTMLURL takes in a standard issue URL and returns the issue
// number, e.g.:
// https://github.com/octocat/Hello-World/issues/1347#issuecomment-1
func ParseCommentHTMLURL(prUrl string) (int, error) {
	u, err := url.Parse(prUrl)
	if err != nil {
		return -1, err
	}

	parts := strings.Split(u.Path, "/")
	i, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return -1, err
	}

	return i, nil
}
