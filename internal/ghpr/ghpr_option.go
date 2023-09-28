// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package ghpr

type PullRequestOption func(*PullRequest) error

// WithWorkdir sets the base directory that can be used temporarily whilst
// manipulating the pull request.
func WithWorkdir(workdir string) PullRequestOption {
	return func(pr *PullRequest) error {
		pr.workdir = workdir
		return nil
	}
}

// WithBaseBranch sets the branch that the pull request is intended to merge
// into.
func WithBaseBranch(name string) PullRequestOption {
	return func(pr *PullRequest) error {
		pr.baseBranch = name
		return nil
	}
}
