// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package check

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/google/go-github/v32/github"
	"github.com/spf13/cobra"
	"kraftkit.sh/cmdfactory"
	kitcfg "kraftkit.sh/config"

	"github.com/unikraft/governance/internal/cmdutils"
	"github.com/unikraft/governance/internal/config"
	"github.com/unikraft/governance/internal/ghapi"
)

type Mergable struct {
	ApproverComments []string `flag:"approver-comments" usage:"Regular expression that an approver writes"`
	ApproverTeams    []string `flag:"approver-teams" usage:"The GitHub team that the approver must be a part of to be considered an approver"`
	ApproveStates    []string `flag:"approve-states" usage:"The state of the GitHub approval from the assignee" default:"approve"`
	IgnoreLabels     []string `flag:"ignore-labels" usage:"Ignore the PR if it has any of these labels"`
	IgnoreStates     []string `flag:"ignore-states" usage:"Ignore the PR if it has any of these states"`
	Labels           []string `flag:"labels" usage:"The PR must have these labels to be considered mergable"`
	MinApprovals     int      `flag:"min-approvals" usage:"Minimum number of approvals required to be considered mergable" default:"1"`
	MinReviews       int      `flag:"min-reviews" usage:"Minimum number of reviews a PR requires to be considered mergable" default:"1"`
	NoConflicts      bool     `flag:"no-conflicts" usage:"Pull request must not have any conflicts"`
	NoDraft          bool     `flag:"no-draft" usage:"Pull request must not be in a draft state"`
	RespectAssignees bool     `flag:"respect-assignees" usage:"Whether the PR's assignees should be considered approvers even if they are not part of a team/codeowner"`
	RespectReviewers bool     `flag:"respect-reviewers" usage:"Whether the PR's requested reviewers review should be considered even if they are not part of a team/codeowner"`
	ReviewerComments []string `flag:"reviewer-comments" usage:"Regular expression that a reviewer writes"`
	ReviewerTeams    []string `flag:"reviewer-teams" usage:"The GitHub team that the reviewer must be a part to be considered a reviewer"`
	ReviewStates     []string `flag:"review-states" usage:"The state of the GitHub approval from the reivewer"`
	States           []string `flag:"states" usage:"Consider the PR mergable if it has one of these supplied states"`

	ghClient *ghapi.GithubClient
}

func NewMergable() *cobra.Command {
	cmd, err := cmdfactory.New(&Mergable{}, cobra.Command{
		Use:   "mergable [OPTIONS] ORG/REPO/PRID",
		Short: "Check whether a PR satisfies the provided merge requirements",
		Args:  cmdutils.OrgRepoAndPullRequestNumber(),
		Annotations: map[string]string{
			cmdfactory.AnnotationHelpGroup: "pr",
		},
		Example: heredoc.Doc(`
		governctl pr check mergable \
			--min-approvals=1 \
			--approver-comments="Approved-by: (?P<approved_by>.*>)" \
			--respect-assignees \
			--min-reviews=1 \
			--reviewer-comments="Reviewed-by: (?P<reviewed_by>.*>)" \
			--review-states=approved \
			--respect-reviewers \
			--ignore-labels="ci/wait" \
			unikraft/unikraft/1078
		`),
	})
	if err != nil {
		panic(err)
	}

	return cmd
}

func (opts *Mergable) Run(cmd *cobra.Command, args []string) error {
	var err error

	ctx := cmd.Context()

	ghOrg, ghRepo, ghPrId, err := cmdutils.ParseOrgRepoAndPullRequestArgs(args)
	if err != nil {
		return err
	}

	opts.ghClient, err = ghapi.NewGithubClient(
		ctx,
		kitcfg.G[config.Config](ctx).GithubToken,
		kitcfg.G[config.Config](ctx).GithubSkipSSL,
		kitcfg.G[config.Config](ctx).GithubEndpoint,
	)
	if err != nil {
		return err
	}

	pull, err := opts.ghClient.GetPullRequest(ctx, ghOrg, ghRepo, ghPrId)
	if err != nil {
		return fmt.Errorf("could not get pull request: %w", err)
	}

	// Ignore if state not requested
	if !opts.requestsState(*pull.State) {
		return fmt.Errorf("pull request does not match requested state")
	}

	// Ignore if labels not requested
	if !opts.requestsLabels(pull.Labels) {
		return fmt.Errorf("pull request does not have requested labels")
	}

	// Ignore if only mergeables requested
	if opts.NoConflicts && !*pull.Mergeable {
		return fmt.Errorf("pull request has merge conflicts")
	}

	// Ignore drafts
	if *pull.Draft {
		return fmt.Errorf("pull request is in draft state")
	}

	// Iterate through all the comments for this PR
	comments, err := opts.ghClient.ListPullRequestComments(
		ctx,
		ghOrg,
		ghRepo,
		ghPrId,
	)
	if err != nil {
		return fmt.Errorf("could not get pull request comments: %w", err)
	}

	res := make(map[string][]string)
	prApprovals := 0
	prReviews := 0

	for _, c := range comments {
		if ok, matches := opts.requestsApproverRegex(*c.Body); ok {
			if opts.requestsApproverTeam(ctx, *pull, *c.User.Login) {
				if !opts.requestsApproveState("comment") {
					continue
				}

				for k, v := range matches {
					if _, ok := res[k]; !ok {
						res[k] = make([]string, 0)
					}
					res[k] = append(res[k], v)
					prApprovals++
				}
			}
		}

		if ok, matches := opts.requestsReviewerRegex(*c.Body); ok {
			if opts.requestsReviewerTeam(ctx, *pull, *c.User.Login) {
				for k, v := range matches {
					if _, ok := res[k]; !ok {
						res[k] = make([]string, 0)
					}
					res[k] = append(res[k], v)
					prReviews++
				}
			}
		}
	}

	// Iterate through all the reviews for this PR
	reviews, err := opts.ghClient.ListPullRequestReviews(ctx, ghOrg, ghRepo, ghPrId)
	if err != nil {
		return fmt.Errorf("could not list pull request reviews: %w", err)
	}

	for _, r := range reviews {
		if ok, matches := opts.requestsApproverRegex(*r.Body); ok {
			if opts.requestsApproverTeam(ctx, *pull, *r.User.Login) {
				if !opts.requestsApproveState(*r.State) {
					continue
				}

				for k, v := range matches {
					if _, ok := res[k]; !ok {
						res[k] = make([]string, 0)
					}
					res[k] = append(res[k], v)
					prApprovals++
				}
			}
		}

		if ok, matches := opts.requestsReviewerRegex(*r.Body); ok {
			if opts.requestsReviewerTeam(ctx, *pull, *r.User.Login) {
				if !opts.requestsReviewState(*r.State) {
					continue
				}

				for k, v := range matches {
					if _, ok := res[k]; !ok {
						res[k] = make([]string, 0)
					}
					res[k] = append(res[k], v)
					prReviews++
				}
			}
		}
	}

	if !opts.hasMinApprovers(prApprovals) || !opts.hasMinReviewers(prReviews) {
		return fmt.Errorf("pull request does not meet the minimum number approvers and reviewers")
	}

	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(&res); err != nil {
		return fmt.Errorf("could not marshal JSON response: %w", err)
	}

	fmt.Print(buffer.String())

	return nil
}

// requestsState checks whether the source requests this particular state
func (opts *Mergable) requestsState(state string) bool {
	ret := false

	// if there are no set states, assume only "open" states
	if len(opts.States) == 0 {
		ret = state == "open"
	} else {
		for _, s := range opts.States {
			if s == state {
				ret = true
				break
			}
		}
	}

	// Ensure ignored states
	for _, s := range opts.IgnoreStates {
		if s == state {
			ret = false
			break
		}
	}

	return ret
}

// requestsApproveState checks whether the PR approver matches the desired state
func (opts *Mergable) requestsApproveState(state string) bool {
	if len(opts.ApproveStates) == 0 {
		return true
	}

	state = strings.ToLower(state)
	for _, s := range opts.ApproveStates {
		if state == strings.ToLower(s) {
			return true
		}
	}

	return false
}

// requestsReviewState checks whether the PR review matches the desired state
func (opts *Mergable) requestsReviewState(state string) bool {
	if len(opts.ReviewStates) == 0 {
		return true
	}

	state = strings.ToLower(state)
	for _, s := range opts.ReviewStates {
		if state == strings.ToLower(s) {
			return true
		}
	}

	return false
}

// requestsLabels checks whether the source requests these set of labels
func (opts *Mergable) requestsLabels(labels []*github.Label) bool {
	ret := false

	// If no set labels, assume all
	if len(opts.Labels) == 0 {
		ret = true
	} else {
	includeLoop:
		for _, rl := range opts.Labels {
			for _, rr := range labels {
				if rl == rr.GetName() {
					ret = true
					break includeLoop
				}
			}
		}
	}

excludeLoop:
	for _, rl := range opts.IgnoreLabels {
		for _, rr := range labels {
			if rl == rr.GetName() {
				ret = false
				break excludeLoop
			}
		}
	}

	return ret
}

// requestsReviewerRegex determines if the source requests this reviewer regex
func (opts *Mergable) requestsReviewerRegex(comment string) (bool, map[string]string) {
	if len(opts.ReviewerComments) == 0 {
		return true, nil
	}

	matches := make(map[string]string)
	for _, c := range opts.ReviewerComments {
		for k, v := range getParams(c, comment) {
			matches[k] = v
		}
	}

	return len(matches) > 0, matches
}

// requestsReviewerTeam determines if the source requests this reviewer team
func (opts *Mergable) requestsReviewerTeam(ctx context.Context, pr github.PullRequest, username string) bool {
	if opts.RespectReviewers {
		return true
	}

	// Check the named approver teams part of the input to this resource
	for _, t := range opts.ReviewerTeams {
		if ok, _ := opts.ghClient.UserMemberOfTeam(ctx, username, t); ok {
			return true
		}
	}

	return false
}

// hasMinReviewers determines whether the supplied list meets the requested
// minimum
func (opts *Mergable) hasMinReviewers(numReviewers int) bool {
	min := 1
	if opts.MinReviews > min {
		min = opts.MinReviews
	}

	return numReviewers >= min
}

// requestsApproverRegex determines if the source requests this approver regex
func (opts *Mergable) requestsApproverRegex(comment string) (bool, map[string]string) {
	if len(opts.ApproverComments) == 0 {
		return true, nil
	}

	matches := make(map[string]string)
	for _, c := range opts.ApproverComments {
		for k, v := range getParams(c, comment) {
			matches[k] = v
		}
	}

	return len(matches) > 0, matches
}

// requestsApproverTeam determines if the source requests this approver team
func (opts *Mergable) requestsApproverTeam(ctx context.Context, pr github.PullRequest, username string) bool {
	if opts.RespectAssignees {
		for _, assignee := range pr.Assignees {
			if username == *assignee.Login {
				return true
			}
		}
	}

	// Check the named approver teams part of the input to this resource
	for _, t := range opts.ApproverTeams {
		if ok, _ := opts.ghClient.UserMemberOfTeam(ctx, username, t); ok {
			return true
		}
	}

	return false
}

// hasMinApprovers determines whether the supplied list meets the requested
// minimum
func (opts *Mergable) hasMinApprovers(numApprovers int) bool {
	min := 1
	if opts.MinApprovals > min {
		min = opts.MinApprovals
	}

	return numApprovers >= min
}

// getParams parses the provided regular expression which has identifiers and
// matches it against the provided body, matches are detected and populated in a
// map with the key as the identifier.
func getParams(regEx, body string) (paramsMap map[string]string) {
	var compRegEx = regexp.MustCompile(regEx)
	match := compRegEx.FindStringSubmatch(body)

	paramsMap = make(map[string]string)
	for i, name := range compRegEx.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	return paramsMap
}
