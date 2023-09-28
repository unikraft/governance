// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package pr

import (
	"github.com/spf13/cobra"
	"kraftkit.sh/cmdfactory"

	"github.com/unikraft/governance/cmd/governctl/pr/check"
	"github.com/unikraft/governance/cmd/governctl/pr/sync"
)

type PR struct{}

func New() *cobra.Command {
	cmd, err := cmdfactory.New(&PR{}, cobra.Command{
		Use:    "pr SUBCOMMAND",
		Short:  "Manage pull requests",
		Hidden: true,
		Annotations: map[string]string{
			cmdfactory.AnnotationHelpGroup: "pr",
		},
	})
	if err != nil {
		panic(err)
	}
	cmd.AddCommand(sync.New())
	cmd.AddCommand(check.New())
	cmd.AddCommand(NewMerge())

	return cmd
}

func (opts *PR) Run(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}
