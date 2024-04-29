// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package checkpatch

import "io"

type PatchOption func(*Patch) error

// WithIgnore sets the types which should be ignored by checkpatch.
func WithIgnore(ignore ...string) PatchOption {
	return func(patch *Patch) error {
		if patch.ignores == nil {
			patch.ignores = make([]string, 0)
		}

		patch.ignores = append(patch.ignores, ignore...)

		return nil
	}
}

// WithStderr sets the stderr for when executing the checkpatch program.
func WithStderr(stderr io.Writer) PatchOption {
	return func(patch *Patch) error {
		patch.stderr = stderr
		return nil
	}
}

// WithCheckpatchScriptPath sets an alternative path to the checkpatch program.
func WithCheckpatchScriptPath(path string) PatchOption {
	return func(patch *Patch) error {
		patch.script = path
		return nil
	}
}

// WithCheckpatchConf sets the checkpatch configuration to use.
func WithCheckpatchConfPath(conf string) PatchOption {
	return func(patch *Patch) error {
		patch.conf = conf
		return nil
	}
}
