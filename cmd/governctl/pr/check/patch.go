// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package check

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/MakeNowJust/heredoc"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"kraftkit.sh/cmdfactory"
	kitcfg "kraftkit.sh/config"
	"kraftkit.sh/iostreams"
	"kraftkit.sh/log"

	"github.com/unikraft/governance/internal/checkpatch"
	"github.com/unikraft/governance/internal/cmdutils"
	"github.com/unikraft/governance/internal/config"
	"github.com/unikraft/governance/internal/ghapi"
	"github.com/unikraft/governance/internal/ghpr"
	"github.com/unikraft/governance/internal/tableprinter"
)

type Patch struct {
	Output           string   `long:"output" short:"o" env:"GOVERN_OUTPUT" usage:"Set the output format of choice [table, html, json, yaml]" default:"table"`
	CheckpatchScript string   `long:"checkpatch-script" env:"GOVERN_CHECKPATCH_SCRIPT" usage:"Use an existing checkpatch.pl script"`
	BaseBranch       string   `long:"base" env:"GOVERN_BASE_BRANCH" usage:"Set the base branch name that the PR will be rebased onto"`
	Ignores          []string `long:"ignore" env:"GOVERN_IGNORES" usage:"Ignore one or many checkpatch checks"`
}

func NewPatch() *cobra.Command {
	cmd, err := cmdfactory.New(&Patch{}, cobra.Command{
		Use:   "patch [OPTIONS] ORG/REPO/PRID",
		Short: "Run checkpatch against a pull request",
		Args:  cmdutils.OrgRepoAndPullRequestNumber(),
		Annotations: map[string]string{
			cmdfactory.AnnotationHelpGroup: "pr",
		},
		Example: heredoc.Doc(`
		# Run checkpatch against PR #1000
		governctl pr check patch unikraft/unikraft/1000
		`),
	})
	if err != nil {
		panic(err)
	}

	return cmd
}

func (opts *Patch) Run(cmd *cobra.Command, args []string) error {
	if len(opts.Ignores) == 0 {
		opts.Ignores = []string{
			"FILE_PATH_CHANGES",
			"OBSOLETE",
			"ASSIGN_IN_IF",
			"NEW_TYPEDEFS",
			"EMAIL_SUBJECT",
		}
	}

	ctx := cmd.Context()

	ghOrg, ghRepo, ghPrId, err := cmdutils.ParseOrgRepoAndPullRequestArgs(args)
	if err != nil {
		return err
	}

	ghClient, err := ghapi.NewGithubClient(
		ctx,
		kitcfg.G[config.Config](ctx).GithubToken,
		kitcfg.G[config.Config](ctx).GithubSkipSSL,
		kitcfg.G[config.Config](ctx).GithubEndpoint,
	)
	if err != nil {
		return err
	}

	pull, err := ghpr.NewPullRequestFromID(ctx,
		ghClient,
		ghOrg,
		ghRepo,
		ghPrId,
		ghpr.WithBaseBranch(opts.BaseBranch),
		ghpr.WithWorkdir(kitcfg.G[config.Config](ctx).TempDir),
	)
	if err != nil {
		return fmt.Errorf("could not prepare pull request: %w", err)
	}

	// Use a well-known path of the checkpatch.pl script contained within the
	// repository or the user-provided alternative.
	if opts.CheckpatchScript == "" {
		opts.CheckpatchScript = filepath.Join(
			pull.LocalRepo(),
			"support", "scripts", "checkpatch.pl",
		)
	}
	if _, err := os.Stat(opts.CheckpatchScript); err != nil {
		return fmt.Errorf("could not access checkpatch script at '%s': %w", opts.CheckpatchScript, err)
	}

	cs := iostreams.G(ctx).ColorScheme()

	table, err := tableprinter.NewTablePrinter(ctx,
		tableprinter.WithOutputFormatFromString(opts.Output),
		tableprinter.WithMaxWidth(iostreams.G(ctx).TerminalWidth()),
	)
	if err != nil {
		return err
	}

	table.AddField("COMMIT", cs.Bold)
	table.AddField("LEVEL", cs.Bold)
	table.AddField("TYPE", cs.Bold)
	table.AddField("MESSAGE", cs.Bold)
	table.AddField("FILE", cs.Bold)
	table.AddField("LINE", cs.Bold)
	table.EndRow()

	warnings := 0
	errors := 0

	for _, patch := range pull.Patches() {
		if _, err := os.Stat(patch.Filename); err != nil {
			log.G(ctx).
				WithField("patch", patch.Filename).
				Info("saving")

			if err := os.WriteFile(patch.Filename, patch.Bytes(), 0644); err != nil {
				return fmt.Errorf("could not write patch file: %w", err)
			}
		}

		check, err := checkpatch.NewCheckpatch(ctx,
			patch.Filename,
			checkpatch.WithIgnore(opts.Ignores...),
			checkpatch.WithCheckpatchScriptPath(opts.CheckpatchScript),
			checkpatch.WithStderr(log.G(ctx).WriterLevel(logrus.TraceLevel)),
		)
		if err != nil {
			return fmt.Errorf("could not parse patch file: %w", err)
		}

		for _, note := range check.Notes() {
			level := cs.Red
			if note.Level == checkpatch.NoteLevelWarning {
				level = cs.Yellow
				warnings++
			} else {
				errors++
			}

			table.AddField(patch.Hash[0:7], nil)
			table.AddField(string(note.Level), level)
			table.AddField(note.Type, nil)
			table.AddField(note.Message, nil)
			table.AddField(note.File, nil)
			table.AddField(fmt.Sprintf("%d", note.Line), nil)
			table.EndRow()

			// Set an annotations on the PR if run in a GitHub Actions context.
			// See: https://docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions#setting-an-error-message
			if os.Getenv("GITHUB_ACTIONS") == "true" && len(note.File) > 0 && note.Line > 0 {
				fmt.Printf("echo ::%s file=%s,line=%d::%s", note.Level, note.File, note.Line, note.Message)
			}
		}
	}

	if errors == 0 && warnings == 0 {
		fmt.Fprintf(iostreams.G(ctx).Out, cs.Green("✔")+" checkpatch passed\n")

		return nil
	}

	// If the user has not specified a temporary directory which will have been
	// passed as the working directory, a temporary one will have been generated.
	// This isn't a "neat" way of cleaning up.
	if kitcfg.G[config.Config](ctx).TempDir == "" {
		log.G(ctx).WithField("path", pull.Workdir()).Info("removing")
		os.RemoveAll(pull.Workdir())
	}

	err = iostreams.G(ctx).StartPager()
	if err != nil {
		log.G(ctx).Errorf("error starting pager: %v", err)
	}

	defer iostreams.G(ctx).StopPager()

	err = table.Render(iostreams.G(ctx).Out)
	if err != nil {
		return err
	}

	if errors > 0 || warnings > 0 {
		return fmt.Errorf("checkpatch failed with %d errors and %d warnings", errors, warnings)
	}

	return nil
}
