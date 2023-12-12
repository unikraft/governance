// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package sync

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"kraftkit.sh/cmdfactory"
)

type Sync struct{}

func New() *cobra.Command {
	cmd, err := cmdfactory.New(&Sync{}, cobra.Command{
		Use:   "sync SUBCOMMAND",
		Short: "Synchronise a pull request",
		Annotations: map[string]string{
			cmdfactory.AnnotationHelpGroup: "pr",
		},
		Hidden: true,
	})
	if err != nil {
		panic(err)
	}

	cmd.AddCommand(NewLabels())
	cmd.AddCommand(NewReviewers())

	return cmd
}

func (*Sync) Run(_ context.Context, _ []string) error {
	return pflag.ErrHelp
}
