// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package team

import (
	"context"
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

type Sync struct {
	Org string `long:"org" env:"GOVERN_GITHUB_ORG" usage:"Set the GitHub organisation that should have teams managed" default:"unikraft"`

	teams []*team.Team
}

func NewSync() *cobra.Command {
	cmd, err := cmdfactory.New(&Sync{}, cobra.Command{
		Use:   "sync",
		Short: "Synchronise teams",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			cmdfactory.AnnotationHelpGroup: "team",
		},
	})
	if err != nil {
		panic(err)
	}

	return cmd
}

func (opts *Sync) Pre(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	ghApi, err := ghapi.NewGithubClient(
		ctx,
		kitcfg.G[config.Config](ctx).GithubToken,
		kitcfg.G[config.Config](ctx).GithubSkipSSL,
		kitcfg.G[config.Config](ctx).GithubEndpoint,
	)
	if err != nil {
		return err
	}

	opts.teams, err = team.NewListOfTeamsFromPath(
		ghApi,
		opts.Org,
		kitcfg.G[config.Config](ctx).TeamsDir,
	)
	if err != nil {
		return fmt.Errorf("could not populate teams: %s", err)
	}
	return nil
}

func (opts *Sync) Run(ctx context.Context, args []string) error {
	for _, t := range opts.teams {
		err := t.Sync(ctx)
		if err != nil {
			log.Fatalf("could not syncronise team: %s: %s", t.Name, err)
			os.Exit(1)
		}
	}

	return nil
}
