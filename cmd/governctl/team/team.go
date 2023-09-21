// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package team

import (
	"github.com/spf13/cobra"
	"kraftkit.sh/cmdfactory"
)

type Team struct{}

func New() *cobra.Command {
	cmd, err := cmdfactory.New(&Team{}, cobra.Command{
		Use:    "team SUBCOMMAND",
		Short:  "Manage GitHub teams",
		Hidden: true,
		Annotations: map[string]string{
			cmdfactory.AnnotationHelpGroup: "team",
		},
	})
	if err != nil {
		panic(err)
	}

	cmd.AddCommand(NewSync())

	return cmd
}

func (opts *Team) Run(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}
