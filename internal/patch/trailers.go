// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package patch

// Trailers are a list of known Git trailers, including the well-known
// 'Signed-off-by' that are recognised in a Git message.
func Trailers() []string {
	return []string{
		"Signed-off-by",
		"Co-authored-by",
		"GitHub-Closes",
		"GitHub-Fixes",
	}
}
