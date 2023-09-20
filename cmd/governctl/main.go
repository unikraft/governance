// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package main

import (
	"fmt"
	"os"

	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"kraftkit.sh/cmdfactory"
	kitcfg "kraftkit.sh/config"
	"kraftkit.sh/log"

	"github.com/unikraft/governance/internal/config"
)

type GovernCtl struct{}

func New() *cobra.Command {
	cmd, err := cmdfactory.New(&GovernCtl{}, cobra.Command{
		Use:   "governctl COMMAND",
		Short: `Govern a GitHub organisation`,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
	})
	if err != nil {
		panic(err)
	}

	// Subcommands
	cmd.AddGroup(&cobra.Group{ID: "main", Title: "COMMANDS"})
	cmd.AddCommand(NewSyncTeams())
	cmd.AddCommand(NewSyncPR())

	return cmd
}

func (*GovernCtl) Run(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}

func main() {
	cfg := config.Config{}
	cfgm, err := kitcfg.NewConfigManager(&cfg)
	if err != nil {
		panic(err)
	}

	cmd := New()

	// Attribute all configuration flags and command-line argument values
	cmd, args, err := cmd.Find(os.Args[1:])
	if err != nil {
		panic(err)
	}
	if err := cmdfactory.AttributeFlags(cmd, &cfg, args...); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Set up the global context
	ctx := signals.SetupSignalContext()
	ctx = kitcfg.WithConfigManager(ctx, cfgm)

	// Configure the log level
	logger := logrus.New()
	if lvl, err := logrus.ParseLevel(cfgm.Config.LogLevel); err == nil {
		logger.SetLevel(lvl)
	}

	ctx = log.WithLogger(ctx, logger)

	// Execute the main command
	cmdfactory.Main(ctx, cmd)
}
