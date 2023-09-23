// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package check

import (
	"github.com/spf13/cobra"
	"kraftkit.sh/cmdfactory"
)

type Check struct{}

func New() *cobra.Command {
	cmd, err := cmdfactory.New(&Check{}, cobra.Command{
		Use:   "check SUBCOMMAND",
		Short: "Check information about a pull request",
		Annotations: map[string]string{
			cmdfactory.AnnotationHelpGroup: "pr",
		},
		Hidden: true,
	})
	if err != nil {
		panic(err)
	}

	cmd.AddCommand(NewMergable())

	return cmd
}

func (*Check) Run(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}
