// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package ghpr

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/go-github/v32/github"
)

// SatisfiesMergeRequirements
func (pr *PullRequest) SatisfiesMergeRequirements(ctx context.Context, opts ...PullRequestMergableOption) (bool, map[string][]string, error) {
	mopts := mergableOptions{
		ghClient:     pr.client,
		minApprovals: 1,
		minReviews:   1,
	}

	for _, opt := range opts {
		opt(&mopts)
	}

	if len(mopts.approverComments) == 0 {
		mopts.approverComments = []string{
			"Approved-by: (?P<approved_by>.*>)",
		}
	}
	if len(mopts.reviewerComments) == 0 {
		mopts.reviewerComments = []string{
			"Reviewed-by: (?P<reviewed_by>.*>)",
		}
	}

	pull, err := mopts.ghClient.GetPullRequest(ctx, pr.ghOrg, pr.ghRepo, pr.ghPrId)
	if err != nil {
		return false, nil, fmt.Errorf("could not get pull request: %w", err)
	}

	// Ignore if state not requested
	if !mopts.requestsState(*pull.State) {
		return false, nil, fmt.Errorf("pull request does not match requested state: got '%s' want '%s'", *pull.State, mopts.states)
	}

	// Ignore if labels not requested
	if !mopts.requestsLabels(pull.Labels) {
		return false, nil, fmt.Errorf("pull request does not have requested labels: got '%s' want '%s'", pull.Labels, mopts.labels)
	}

	// Ignore if only mergeables requested
	if mopts.noConflicts && !*pull.Mergeable {
		return false, nil, fmt.Errorf("pull request has merge conflicts")
	}

	// Ignore drafts
	if *pull.Draft {
		return false, nil, fmt.Errorf("pull request is in draft state")
	}

	// Iterate through all the comments for this PR
	comments, err := mopts.ghClient.ListPullRequestComments(
		ctx,
		pr.ghOrg,
		pr.ghRepo,
		pr.ghPrId,
	)
	if err != nil {
		return false, nil, fmt.Errorf("could not get pull request comments: %w", err)
	}

	res := make(map[string][]string)
	prApprovals := 0
	prReviews := 0

	for _, c := range comments {
		if ok, matches := mopts.requestsApproverRegex(*c.Body); ok {
			if mopts.requestsApproverTeam(ctx, *pull, *c.User.Login) {
				if !mopts.requestsApproveState("comment") {
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

		if ok, matches := mopts.requestsReviewerRegex(*c.Body); ok {
			if mopts.requestsReviewerTeam(ctx, *pull, *c.User.Login) {
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
	reviews, err := mopts.ghClient.ListPullRequestReviews(ctx, pr.ghOrg, pr.ghRepo, pr.ghPrId)
	if err != nil {
		return false, nil, fmt.Errorf("could not list pull request reviews: %w", err)
	}

	for _, r := range reviews {
		if ok, matches := mopts.requestsApproverRegex(*r.Body); ok {
			if mopts.requestsApproverTeam(ctx, *pull, *r.User.Login) {
				if !mopts.requestsApproveState(*r.State) {
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

		if ok, matches := mopts.requestsReviewerRegex(*r.Body); ok {
			if mopts.requestsReviewerTeam(ctx, *pull, *r.User.Login) {
				if !mopts.requestsReviewState(*r.State) {
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

	fmt.Printf("approvers (%d/%d) and reviewers (%d/%d)\n",
		prApprovals,
		mopts.minApprovals,
		prReviews,
		mopts.minReviews)

	if prApprovals < mopts.minApprovals || prReviews < mopts.minReviews {
		return false, nil, fmt.Errorf(
			"pull request does not meet the minimum number approvers (%d/%d) and reviewers (%d/%d)",
			prApprovals,
			mopts.minApprovals,
			prReviews,
			mopts.minReviews,
		)
	}

	return true, res, nil
}

// requestsState checks whether the source requests this particular state
func (opts *mergableOptions) requestsState(state string) bool {
	ret := false

	// if there are no set states, assume only "open" states
	if len(opts.states) == 0 {
		ret = state == "open"
	} else {
		for _, s := range opts.states {
			if s == state {
				ret = true
				break
			}
		}
	}

	// Ensure ignored states
	for _, s := range opts.ignoreStates {
		if s == state {
			ret = false
			break
		}
	}

	return ret
}

// requestsApproveState checks whether the PR approver matches the desired state
func (opts *mergableOptions) requestsApproveState(state string) bool {
	if len(opts.approveStates) == 0 {
		return true
	}

	state = strings.ToLower(state)
	for _, s := range opts.approveStates {
		if state == strings.ToLower(s) {
			return true
		}
	}

	return false
}

// requestsReviewState checks whether the PR review matches the desired state
func (opts *mergableOptions) requestsReviewState(state string) bool {
	if len(opts.reviewStates) == 0 {
		return true
	}

	state = strings.ToLower(state)
	for _, s := range opts.reviewStates {
		if state == strings.ToLower(s) {
			return true
		}
	}

	return false
}

// requestsLabels checks whether the source requests these set of labels
func (opts *mergableOptions) requestsLabels(labels []*github.Label) bool {
	ret := false

	// If no set labels, assume all
	if len(opts.labels) == 0 {
		ret = true
	} else {
	includeLoop:
		for _, rl := range opts.labels {
			for _, rr := range labels {
				if rl == rr.GetName() {
					ret = true
					break includeLoop
				}
			}
		}
	}

excludeLoop:
	for _, rl := range opts.ignoreLabels {
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
func (opts *mergableOptions) requestsReviewerRegex(comment string) (bool, map[string]string) {
	if len(opts.reviewerComments) == 0 {
		return true, nil
	}

	matches := make(map[string]string)
	for _, c := range opts.reviewerComments {
		for k, v := range getParams(c, comment) {
			matches[k] = v
		}
	}

	return len(matches) > 0, matches
}

// requestsReviewerTeam determines if the source requests this reviewer team
func (opts *mergableOptions) requestsReviewerTeam(ctx context.Context, pr github.PullRequest, username string) bool {
	if !opts.noRespectReviewers {
		return true
	}

	// Check the named approver teams part of the input to this resource
	for _, t := range opts.reviewerTeams {
		if ok, _ := opts.ghClient.UserMemberOfTeam(ctx, username, t); ok {
			return true
		}
	}

	return false
}

// requestsApproverRegex determines if the source requests this approver regex
func (opts *mergableOptions) requestsApproverRegex(comment string) (bool, map[string]string) {
	if len(opts.approverComments) == 0 {
		return true, nil
	}

	matches := make(map[string]string)
	for _, c := range opts.approverComments {
		for k, v := range getParams(c, comment) {
			matches[k] = v
		}
	}

	return len(matches) > 0, matches
}

// requestsApproverTeam determines if the source requests this approver team
func (opts *mergableOptions) requestsApproverTeam(ctx context.Context, pr github.PullRequest, username string) bool {
	if !opts.noRespectAssignees {
		for _, assignee := range pr.Assignees {
			if username == *assignee.Login {
				return true
			}
		}
	}

	// Check the named approver teams part of the input to this resource
	for _, t := range opts.approverTeams {
		if ok, _ := opts.ghClient.UserMemberOfTeam(ctx, username, t); ok {
			return true
		}
	}

	return false
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
