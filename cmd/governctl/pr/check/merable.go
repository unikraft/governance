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
	"os"

	heredoc "github.com/MakeNowJust/heredoc/v2"
	"github.com/spf13/cobra"
	"kraftkit.sh/cmdfactory"
	kitcfg "kraftkit.sh/config"
	"kraftkit.sh/log"

	"github.com/unikraft/governance/internal/cmdutils"
	"github.com/unikraft/governance/internal/config"
	"github.com/unikraft/governance/internal/ghapi"
	"github.com/unikraft/governance/internal/ghpr"
)

type Mergable struct {
	ApproverComments   []string `long:"approver-comments" env:"GOVERN_APPROVER_COMMENTS" usage:"Regular expression that an approver writes"`
	ApproverTeams      []string `long:"approver-teams" env:"GOVERN_APPROVER_TEAMS" usage:"The GitHub team that the approver must be a part of to be considered an approver"`
	ApproveStates      []string `long:"approve-states" env:"GOVERN_APPROVE_STATES" usage:"The state of the GitHub approval from the assignee" default:"approve"`
	CommitterEmail     string   `long:"committer-email" short:"e" env:"GOVERN_COMMITTER_EMAIL" usage:"Set the Git committer author's email"`
	CommitterGlobal    bool     `long:"committer-global" env:"GOVERN_COMMITTER_GLOBAL" usage:"Set the Git committer author's email/name globally"`
	CommitterName      string   `long:"committer-name" short:"n" env:"GOVERN_COMMITTER_NAME" usage:"Set the Git committer author's name"`
	IgnoreLabels       []string `long:"ignore-labels" env:"GOVERN_IGNORE_LABELS" usage:"Ignore the PR if it has any of these labels"`
	IgnoreStates       []string `long:"ignore-states" env:"GOVERN_IGNORE_STATES" usage:"Ignore the PR if it has any of these states"`
	Labels             []string `long:"labels" env:"GOVERN_LABELS" usage:"The PR must have these labels to be considered mergable"`
	MinApprovals       int      `long:"min-approvals" env:"GOVERN_MIN_APPROVALS" usage:"Minimum number of approvals required to be considered mergable" default:"1"`
	MinReviews         int      `long:"min-reviews" env:"GOVERN_MIN_REVIEWS" usage:"Minimum number of reviews a PR requires to be considered mergable" default:"1"`
	NoConflicts        bool     `long:"no-conflicts" env:"GOVERN_NO_CONFLICTS" usage:"Pull request must not have any conflicts"`
	NoDraft            bool     `long:"no-draft" env:"GOVERN_NO_DRAFT" usage:"Pull request must not be in a draft state"`
	NoRespectAssignees bool     `long:"no-respect-assignees" env:"GOVERN_NO_RESPECT_ASSIGNEES" usage:"Whether the PR's assignees should be not considered approvers even if they are not part of a team/codeowner"`
	NoRespectReviewers bool     `long:"no-respect-reviewers" env:"GOVERN_NO_RESPECT_REVIEWERS" usage:"Whether the PR's requested reviewers review should not be considered even if they are not part of a team/codeowner"`
	ReviewerComments   []string `long:"reviewer-comments" env:"GOVERN_REVIEWER_COMMENTS" usage:"Regular expression that a reviewer writes"`
	ReviewerTeams      []string `long:"reviewer-teams" env:"GOVERN_REVIEWER_TEAMS" usage:"The GitHub team that the reviewer must be a part to be considered a reviewer"`
	ReviewStates       []string `long:"review-states" env:"GOVERN_REVIEW_STATES" usage:"The state of the GitHub approval from the reivewer"`
	States             []string `long:"states" env:"GOVERN_STATES" usage:"Consider the PR mergable if it has one of these supplied states"`
}

func NewMergable() *cobra.Command {
	cmd, err := cmdfactory.New(&Mergable{}, cobra.Command{
		Use:   "mergable [OPTIONS] ORG/REPO/PRID",
		Short: "Check whether a PR satisfies the provided merge requirements",
		Args:  cobra.MaximumNArgs(2),
		Annotations: map[string]string{
			cmdfactory.AnnotationHelpGroup: "pr",
		},
		Example: heredoc.Doc(`
		governctl pr check mergable \
			--min-approvals=1 \
			--approver-comments="Approved-by: (?P<approved_by>.*>)" \
			--min-reviews=1 \
			--reviewer-comments="Reviewed-by: (?P<reviewed_by>.*>)" \
			--review-states=approved \
			--ignore-labels="ci/wait" \
			unikraft/unikraft/1078
		`),
	})
	if err != nil {
		panic(err)
	}

	return cmd
}

func (opts *Mergable) Run(ctx context.Context, args []string) error {
	ghOrg, ghRepo, ghPrId, err := cmdutils.ParseOrgRepoAndPullRequestArgs(args)
	if err != nil {
		return err
	}

	ghClient, err := ghapi.NewGithubClient(
		ctx,
		kitcfg.G[config.Config](ctx).GithubToken,
		kitcfg.G[config.Config](ctx).GithubSkipSSL,
		kitcfg.G[config.Config](ctx).GithubEndpoint,
	)
	if err != nil {
		return err
	}

	pull, err := ghpr.NewPullRequestFromID(ctx,
		ghClient,
		ghOrg,
		ghRepo,
		opts.CommitterName,
		opts.CommitterEmail,
		ghPrId,
		opts.CommitterGlobal,
		// ghpr.WithBaseBranch(opts.BaseBranch),
		ghpr.WithWorkdir(kitcfg.G[config.Config](ctx).TempDir),
	)
	if err != nil {
		return fmt.Errorf("could not prepare pull request: %w", err)
	}

	_, result, err := pull.SatisfiesMergeRequirements(ctx,
		ghpr.WithApproverComments(opts.ApproverComments...),
		ghpr.WithApproverTeams(opts.ApproverTeams...),
		ghpr.WithApproveStates(opts.ApproveStates...),
		ghpr.WithIgnoreLabels(opts.IgnoreLabels...),
		ghpr.WithIgnoreStates(opts.IgnoreStates...),
		ghpr.WithLabels(opts.Labels...),
		ghpr.WithMinApprovals(opts.MinApprovals),
		ghpr.WithMinReviews(opts.MinReviews),
		ghpr.WithNoConflicts(opts.NoConflicts),
		ghpr.WithNoDraft(opts.NoDraft),
		ghpr.WithNoRespectAssignees(opts.NoRespectAssignees),
		ghpr.WithNoRespectReviewers(opts.NoRespectReviewers),
		ghpr.WithReviewerComments(opts.ReviewerComments...),
		ghpr.WithReviewerTeams(opts.ReviewerTeams...),
		ghpr.WithReviewStates(opts.ReviewStates...),
		ghpr.WithStates(opts.States...),
	)
	if err != nil {
		return fmt.Errorf("pull request is not mergable: %w", err)
	}

	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(&result); err != nil {
		return fmt.Errorf("could not marshal JSON response: %w", err)
	}

	fmt.Print(buffer.String())

	// If the user has not specified a temporary directory which will have been
	// passed as the working directory, a temporary one will have been generated.
	// This isn't a "neat" way of cleaning up.
	if kitcfg.G[config.Config](ctx).TempDir == "" {
		log.G(ctx).WithField("path", pull.Workdir()).Info("removing")
		os.RemoveAll(pull.Workdir())
	}

	return nil
}
