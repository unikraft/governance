// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/MakeNowJust/heredoc"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"kraftkit.sh/cmdfactory"
	kitcfg "kraftkit.sh/config"
	"kraftkit.sh/iostreams"
	"kraftkit.sh/log"

	"github.com/unikraft/governance/cmd/governctl/pr"
	"github.com/unikraft/governance/cmd/governctl/team"
	"github.com/unikraft/governance/internal/config"
	"github.com/unikraft/governance/internal/version"
)

type GovernCtl struct{}

func New() *cobra.Command {
	cmd, err := cmdfactory.New(&GovernCtl{}, cobra.Command{
		Use:   "governctl COMMAND",
		Short: `Govern the Unikraft Open-Source Project GitHub Organization`,
		Long: heredoc.Docf(`
		Govern the Unikraft Open-Source Project GitHub Organization

		The utility program governctl is intended to be used by maintainers,
		reviewers, team members, staff and contributors to ease repetitive
		maintenance tasks within the Unikraft Open-Source Project.

		VERSION
		  %s`, version.String()),
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
	})
	if err != nil {
		panic(err)
	}

	// Subcommands
	cmd.AddGroup(&cobra.Group{ID: "pr", Title: "PULL REQUEST COMMANDS"})
	cmd.AddCommand(pr.New())

	cmd.AddGroup(&cobra.Group{ID: "team", Title: "TEAM COMMANDS"})
	cmd.AddCommand(team.New())

	return cmd
}

func (*GovernCtl) Run(_ context.Context, _ []string) error {
	return pflag.ErrHelp
}

func main() {
	cfg := config.Config{}
	cfgm, err := kitcfg.NewConfigManager(&cfg)
	if err != nil {
		panic(err)
	}
	cmd := New()

	// Set up the global context
	ctx := signals.SetupSignalContext()
	ctx = kitcfg.WithConfigManager(ctx, cfgm)

	// Attribute all configuration flags and command-line argument values
	if err := cmdfactory.AttributeFlags(cmd, &cfg, os.Args[1:]...); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if err := cmd.ParseFlags(os.Args[1:]); err == nil {
		cmd.DisableFlagParsing = true
	}

	// Configure the log level
	logger := logrus.New()
	formatter := new(log.TextFormatter)
	formatter.ForceColors = true
	formatter.ForceFormatting = true
	formatter.FullTimestamp = true
	formatter.DisableTimestamp = true
	logger.Formatter = formatter

	if lvl, err := logrus.ParseLevel(cfgm.Config.LogLevel); err == nil {
		logger.SetLevel(lvl)
	}

	ctx = log.WithLogger(ctx, logger)
	ctx = iostreams.WithIOStreams(ctx, iostreams.System())

	// Execute the main command
	os.Exit(cmdfactory.Main(ctx, cmd))
}
