// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package utils

// Difference finds the diff between two lists of strings
func Difference(a, b []string) []string {
	var diff []string
	mb := make(map[string]struct{}, len(b))

	for _, x := range b {
		mb[x] = struct{}{}
	}

	for _, x := range a {
		if _, found := mb[x]; !found {
			diff = append(diff, x)
		}
	}

	return diff
}

// Intersect finds the intersection between two lists of strings
func Intersect(a, b []string) []string {
	var diff []string
	mb := make(map[string]struct{}, len(b))

	for _, x := range b {
		mb[x] = struct{}{}
	}

	for _, x := range a {
		if _, found := mb[x]; found {
			diff = append(diff, x)
		}
	}

	return diff
}
