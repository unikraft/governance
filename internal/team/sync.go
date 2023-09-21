// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package team

import (
	"context"
	"fmt"
	"strings"

	gh "github.com/google/go-github/v32/github"
	kitcfg "kraftkit.sh/config"
	"kraftkit.sh/log"

	"github.com/unikraft/governance/internal/config"
	"github.com/unikraft/governance/internal/user"
)

func (t *Team) Sync(ctx context.Context) error {
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
			err = t.ParentTeam.Sync(ctx)
			if err != nil {
				return fmt.Errorf("could not synchronize parent: %s", err)
			}
		}

		parentGithubTeam, err = t.ghApi.FindTeam(ctx, t.Org, t.Parent)
		if err != nil {
			return err
		}
	}

	log.G(ctx).Infof("synchronising @%s/%s...", t.Org, t.Name)

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
	log.G(ctx).Infof("updating team details...")
	githubTeam, err = t.ghApi.CreateOrUpdateTeam(
		ctx,
		kitcfg.G[config.Config](ctx).GithubOrg,
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

	log.G(ctx).Infof("synchronising team members...")
	err = t.ghApi.SyncTeamMembers(
		ctx,
		kitcfg.G[config.Config](ctx).GithubOrg,
		t.Name,
		string(user.Member),
		members,
	)
	if err != nil {
		return fmt.Errorf("could not synchronise team members: %s", err)
	}

	if len(maintainers) > 0 {
		maintainersTeamName := fmt.Sprintf("%ss-%s", string(user.Maintainer), t.shortName)
		log.G(ctx).Infof("Synchronising @%s/%s...", t.Org, maintainersTeamName)

		// Create or update a sub-team with list of maintainers
		log.G(ctx).Infof("updating team details...")
		_, err := t.ghApi.CreateOrUpdateTeam(
			ctx,
			kitcfg.G[config.Config](ctx).GithubOrg,
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
		log.G(ctx).Infof("synchronising team members...")
		err = t.ghApi.SyncTeamMembers(
			ctx,
			kitcfg.G[config.Config](ctx).GithubOrg,
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
		log.G(ctx).Infof("Synchronising @%s/%s...", t.Org, reviewersTeamName)

		// Create or update a sub-team with list of reviewers
		log.G(ctx).Infof("updating team details...")
		_, err := t.ghApi.CreateOrUpdateTeam(
			ctx,
			kitcfg.G[config.Config](ctx).GithubOrg,
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
		log.G(ctx).Infof("synchronising team members...")
		err = t.ghApi.SyncTeamMembers(
			ctx,
			kitcfg.G[config.Config](ctx).GithubOrg,
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
