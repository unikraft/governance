// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

// Package checkpatch is a utility package which wraps the checkpatch.pl
// program.
package checkpatch

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"kraftkit.sh/log"
)

type Checkpatch struct {
	NoTree    bool   `flag:"no-tree"`
	Patch     bool   `flag:"patch"`
	Ignore    string `flag:"ignore"`
	ShowTypes bool   `flag:"flag-types"`
	Color     string `flag:"color"`
}

type Patch struct {
	File    string
	ignores []string
	notes   []*Note
	stderr  io.Writer
	script  string
	conf    string
}

type NoteLevel string

const (
	NoteLevelWarning = NoteLevel("warning")
	NoteLevelError   = NoteLevel("error")
)

// Note is a result from executing checkpatch.
type Note struct {
	Level   NoteLevel `json:"level"`
	Type    string    `json:"type"`
	Message string    `json:"message"`
	File    string    `json:"file"`
	Line    int       `json:"line"`
	Excerpt []string  `json:"excerpt"`
}

// NewCheckpatch executes a checkpatch against a provided file which represents
// a formatted, mailbox patch.
func NewCheckpatch(ctx context.Context, file string, opts ...PatchOption) (*Patch, error) {
	patch := Patch{
		notes: make([]*Note, 0),
	}

	for _, opt := range opts {
		if err := opt(&patch); err != nil {
			return nil, err
		}
	}

	if patch.script == "" {
		patch.script = "checkpatch.pl"
	}

	if patch.conf == "" {
		patch.conf = ".checkpatch.conf"
	}

	args := []string{
		"--patch",
		"--color=never",
		"--root=" + filepath.Dir(filepath.Dir(filepath.Dir(patch.script))),
	}

	// Add options from the conf file in the PR
	content, err := os.ReadFile(patch.conf)
	if err != nil {
		return nil, fmt.Errorf("could not read checkpatch configuration file: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		args = append(args, strings.Split(line, " ")...)
	}

	// Extra ignores from the commits.
	if len(patch.ignores) > 0 {
		args = append(args,
			"--ignore",
			strings.Join(patch.ignores, ","),
		)
	}

	args = append(args, file)

	c := exec.Command(patch.script, args...)
	c.Stderr = patch.stderr
	c.Env = os.Environ()

	log.G(ctx).Info(
		strings.Join(append([]string{patch.script}, args...), " "),
	)

	var out []byte
	if out, err = c.Output(); err != nil && out == nil {
		return nil, fmt.Errorf("running checkpatch.pl failed: %w", err)
	}

	var note *Note
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		if warning := strings.TrimPrefix(line, "WARNING:"); len(warning) < len(line) {
			split := strings.SplitN(warning, ":", 2)
			if len(split) != 2 {
				return nil, fmt.Errorf("malformed checkpatch line '%s': expected ':'", line)
			}

			note = &Note{
				Level:   NoteLevelWarning,
				Type:    split[0],
				Message: strings.TrimSpace(split[1]),
				Excerpt: make([]string, 0),
			}
			patch.notes = append(patch.notes, note)

		} else if erro := strings.TrimPrefix(line, "ERROR:"); len(erro) < len(line) {
			split := strings.SplitN(erro, ":", 2)
			if len(split) != 2 {
				return nil, fmt.Errorf("malformed checkpatch line '%s': expected ':'", line)
			}

			note = &Note{
				Level:   NoteLevelError,
				Type:    split[0],
				Message: strings.TrimSpace(split[1]),
				Excerpt: make([]string, 0),
			}
			patch.notes = append(patch.notes, note)

		} else if strings.HasPrefix(line, "total:") {
			break
		} else if note != nil && note.File == "" && strings.Contains(line, "FILE") {
			split := strings.Split(line, ": ")
			if len(split) != 3 {
				return nil, fmt.Errorf("malformed line information: expected format '#<DIGITS>: FILE: <FILE>:<LINE>:' but got '%s'", line)
			}

			fileLine := strings.Split(split[2], ":")
			if len(fileLine) != 3 {
				return nil, fmt.Errorf("malformed line formation: expected '<FILE>:<LINE>:' but got '%s'", line)
			}

			note.File = fileLine[0]

			var err error
			note.Line, err = strconv.Atoi(fileLine[1])
			if err != nil {
				return nil, fmt.Errorf("could not convert line number '%s' on line '%s': %w", fileLine[1], line, err)
			}

		} else if note != nil && len(line) > 0 {
			note.Excerpt = append(note.Excerpt, line)
		}
	}

	return &patch, nil
}

// Notes returns the results from the checkpatch.
func (patch *Patch) Notes() []*Note {
	return patch.notes
}
