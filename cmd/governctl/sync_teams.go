// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package main

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/unikraft/governance/internal/config"
	"github.com/unikraft/governance/internal/ghapi"
	"github.com/unikraft/governance/internal/team"
	"kraftkit.sh/cmdfactory"
	kitcfg "kraftkit.sh/config"
)

type SyncTeams struct {
	teams []*team.Team
}

func NewSyncTeams() *cobra.Command {
	cmd, err := cmdfactory.New(&SyncTeams{}, cobra.Command{
		Use:   "sync-teams",
		Short: "Synchronise teams",
		Annotations: map[string]string{
			cmdfactory.AnnotationHelpGroup: "main",
		},
	})
	if err != nil {
		panic(err)
	}

	return cmd
}

func (opts *SyncTeams) Pre(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	ghApi, err := ghapi.NewGithubClient(
		ctx,
		kitcfg.G[config.Config](ctx).GithubOrg,
		kitcfg.G[config.Config](ctx).GithubToken,
		kitcfg.G[config.Config](ctx).GithubSkipSSL,
		kitcfg.G[config.Config](ctx).GithubEndpoint,
	)
	if err != nil {
		return err
	}

	opts.teams, err = team.NewListOfTeamsFromPath(
		ghApi,
		kitcfg.G[config.Config](ctx).GithubOrg,
		kitcfg.G[config.Config](ctx).TeamsDir,
	)
	if err != nil {
		return fmt.Errorf("could not populate teams: %s", err)
	}
	return nil
}

func (opts *SyncTeams) Run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	for _, t := range opts.teams {
		err := t.Sync(ctx)
		if err != nil {
			log.Fatalf("could not syncronise team: %s: %s", t.Name, err)
			os.Exit(1)
		}
	}

	return nil
}
