// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package ghpr

import "github.com/unikraft/governance/internal/ghapi"

type mergableOptions struct {
	approverComments   []string
	approverTeams      []string
	approveStates      []string
	ignoreLabels       []string
	ignoreStates       []string
	labels             []string
	minApprovals       int
	minReviews         int
	noConflicts        bool
	noDraft            bool
	noRespectAssignees bool
	noRespectReviewers bool
	reviewerComments   []string
	reviewerTeams      []string
	reviewStates       []string
	states             []string

	ghClient *ghapi.GithubClient
}

type PullRequestMergableOption func(*mergableOptions)

// WithApproverComments sets the regular expression that an approver writes.
func WithApproverComments(approverComments ...string) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		if opts.approverComments == nil {
			opts.approverComments = []string{}
		}

		opts.approverComments = append(opts.approverComments, approverComments...)
	}
}

// WithApproverTeams sets the the GitHub team that the approver must be a part
// of to be considered an approver.
func WithApproverTeams(approverTeams ...string) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		if opts.approverTeams == nil {
			opts.approverTeams = []string{}
		}

		opts.approverTeams = append(opts.approverTeams, approverTeams...)
	}
}

// WithApproveStates sets the the state of the GitHub approval from the
// assignee.
func WithApproveStates(approveStates ...string) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		if opts.approveStates == nil {
			opts.approveStates = []string{}
		}

		opts.approveStates = append(opts.approveStates, approveStates...)
	}
}

// WithIgnoreLabels sets the ignore the PR if it has any of these labels.
func WithIgnoreLabels(ignoreLabels ...string) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		if opts.ignoreLabels == nil {
			opts.ignoreLabels = []string{}
		}

		opts.ignoreLabels = append(opts.ignoreLabels, ignoreLabels...)
	}
}

// WithIgnoreStates sets the ignore the PR if it has any of these states.
func WithIgnoreStates(ignoreStates ...string) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		if opts.ignoreStates == nil {
			opts.ignoreStates = []string{}
		}

		opts.ignoreStates = append(opts.ignoreStates, ignoreStates...)
	}
}

// WithLabels sets the the PR must have these labels to be considered
// mergable.
func WithLabels(labels ...string) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		if opts.labels == nil {
			opts.labels = []string{}
		}

		opts.labels = append(opts.labels, labels...)
	}
}

// WithMinApprovals sets the minimum number of approvals required to be
// considered mergable
func WithMinApprovals(minApprovals int) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		opts.minApprovals = minApprovals
	}
}

// WithMinReviews sets the minimum number of reviews a PR requires to be
// considered mergable.
func WithMinReviews(minReviews int) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		opts.minReviews = minReviews
	}
}

// WithNoConflicts sets the pull request must not have any conflicts.
func WithNoConflicts(noConflicts bool) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		opts.noConflicts = noConflicts
	}
}

// WithNoDraft sets the pull request must not be in a draft state.
func WithNoDraft(noDraft bool) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		opts.noDraft = noDraft
	}
}

// WithNoRespectAssignees sets the whether the PR's assignees should be not
// considered approvers even if they are not part of a team/codeowner.
func WithNoRespectAssignees(noRespectAssignees bool) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		opts.noRespectAssignees = noRespectAssignees
	}
}

// WithNoRespectReviewers sets the whether the PR's requested reviewers review
// should not be considered even if they are not part of a team/codeowner.
func WithNoRespectReviewers(noRespectReviewers bool) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		opts.noRespectReviewers = noRespectReviewers
	}
}

// WithReviewerComments sets the regular expression that a reviewer writes.
func WithReviewerComments(reviewerComments ...string) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		if opts.reviewerComments == nil {
			opts.reviewerComments = []string{}
		}

		opts.reviewerComments = append(opts.reviewerComments, reviewerComments...)
	}
}

// WithReviewerTeams sets the the GitHub team that the reviewer must be a part
// to be considered a reviewer.
func WithReviewerTeams(reviewerTeams ...string) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		if opts.reviewerTeams == nil {
			opts.reviewerTeams = []string{}
		}

		opts.reviewerTeams = append(opts.reviewerTeams, reviewerTeams...)
	}
}

// WithReviewStates sets the the state of the GitHub approval from the reivewer.
func WithReviewStates(reviewStates ...string) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		if opts.reviewStates == nil {
			opts.reviewStates = []string{}
		}

		opts.reviewStates = append(opts.reviewStates, reviewStates...)
	}
}

// WithStates sets the consider the PR mergable if it has one of these supplied
// states.
func WithStates(states ...string) PullRequestMergableOption {
	return func(opts *mergableOptions) {
		if opts.states == nil {
			opts.states = []string{}
		}

		opts.states = append(opts.states, states...)
	}
}
