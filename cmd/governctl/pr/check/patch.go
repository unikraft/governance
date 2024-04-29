// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package check

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	CommitterEmail   string `long:"committer-email" short:"e" env:"GOVERN_COMMITTER_EMAIL" usage:"Set the Git committer author's email"`
	CommiterGlobal   bool   `long:"committer-global" env:"GOVERN_COMMITTER_GLOBAL" usage:"Set the Git committer author's email/name globally"`
	CommitterName    string `long:"committer-name" short:"n" env:"GOVERN_COMMITTER_NAME" usage:"Set the Git committer author's name"`
	Output           string `long:"output" short:"o" env:"GOVERN_OUTPUT" usage:"Set the output format of choice [table, html, json, yaml]" default:"table"`
	CheckpatchScript string `long:"checkpatch-script" env:"GOVERN_CHECKPATCH_SCRIPT" usage:"Use an existing checkpatch.pl script"`
	CheckpatchConf   string `long:"checkpatch-conf" env:"GOVERN_CHECKPATCH_CONF" usage:"Use an existing checkpatch.conf file"`
	Ignore           string `long:"ignore" env:"GOVERN_IGNORE" usage:"DEPRECATED: Set the types which should be ignored by checkpatch (ignored)"`
	BaseBranch       string `long:"base" env:"GOVERN_BASE_BRANCH" usage:"Set the base branch name that the PR will be rebased onto"`
}

const (
	// checkpatchIgnore is the string that is used to ignore a checkpatch check
	// in a commit message.
	checkpatchIgnore = "Checkpatch-Ignore: "
)

func NewPatch() *cobra.Command {
	cmd, err := cmdfactory.New(&Patch{}, cobra.Command{
		Use:   "patch [OPTIONS] ORG/REPO/PRID",
		Short: "Run checkpatch against a pull request",
		Args:  cobra.MaximumNArgs(2),
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

func (opts *Patch) Run(ctx context.Context, args []string) error {
	var extraIgnores []string

	fmt.Printf("opts: %v\n", opts)
	fmt.Printf("%d\n", len(opts.CommitterName))

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
		opts.CommitterName,
		opts.CommitterEmail,
		ghPrId,
		opts.CommiterGlobal,
		ghpr.WithBaseBranch(opts.BaseBranch),
		ghpr.WithWorkdir(kitcfg.G[config.Config](ctx).TempDir),
	)
	if err != nil {
		return fmt.Errorf("could not prepare pull request: %w", err)
	}

	for _, patch := range pull.Patches() {
		for _, line := range strings.Split(patch.Message, "\n") {
			if !strings.HasPrefix(line, checkpatchIgnore) {
				continue
			}

			ignoreList := strings.SplitN(line, checkpatchIgnore, 2)[1]
			for _, i := range strings.Split(ignoreList, ",") {
				extraIgnores = append(extraIgnores,
					strings.ToUpper(strings.TrimSpace(i)),
				)
			}
		}
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

	if opts.CheckpatchConf == "" {
		opts.CheckpatchConf = filepath.Join(
			pull.LocalRepo(),
			".checkpatch.conf",
		)
	}
	if _, err := os.Stat(opts.CheckpatchConf); err != nil {
		return fmt.Errorf("could not access checkpatch configuration at '%s': %w", opts.CheckpatchConf, err)
	}

	cs := iostreams.G(ctx).ColorScheme()

	topts := []tableprinter.TablePrinterOption{
		tableprinter.WithOutputFormatFromString(opts.Output),
	}

	if kitcfg.G[config.Config](ctx).NoRender {
		topts = append(topts, tableprinter.WithMaxWidth(10000))
	} else {
		topts = append(topts, tableprinter.WithMaxWidth(iostreams.G(ctx).TerminalWidth()))
	}

	table, err := tableprinter.NewTablePrinter(ctx, topts...)
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
			checkpatch.WithIgnore(extraIgnores...),
			checkpatch.WithCheckpatchScriptPath(opts.CheckpatchScript),
			checkpatch.WithCheckpatchConfPath(opts.CheckpatchConf),
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
			table.AddField("\""+note.Message+"\"", nil)
			table.AddField(note.File, nil)
			table.AddField(fmt.Sprintf("%d", note.Line), nil)
			table.EndRow()

			// Set an annotations on the PR if run in a GitHub Actions context.
			// See: https://docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions#setting-an-error-message
			if os.Getenv("GITHUB_ACTIONS") == "true" && len(note.File) > 0 && note.Line > 0 {
				fmt.Printf("::%s file=%s,line=%d,title=%s::%s\n",
					note.Level,
					note.File,
					note.Line,
					note.Type,
					note.Message,
				)
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

	if !kitcfg.G[config.Config](ctx).NoRender {
		err = iostreams.G(ctx).StartPager()
		if err != nil {
			log.G(ctx).Errorf("error starting pager: %v", err)
		}

		defer iostreams.G(ctx).StopPager()
	}

	if os.Getenv("GITHUB_ACTIONS") == "" {
		err = table.Render(iostreams.G(ctx).Out)
		if err != nil {
			return err
		}
	}

	if errors > 0 || warnings > 0 {
		return fmt.Errorf("summary: checkpatch failed with %d errors and %d warnings", errors, warnings)
	}

	return nil
}
