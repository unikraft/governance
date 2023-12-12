// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package check

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	cmd.AddCommand(NewPatch())

	return cmd
}

func (*Check) Run(_ context.Context, _ []string) error {
	return pflag.ErrHelp
}
