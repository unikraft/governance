// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package cmdutils

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"kraftkit.sh/cmdfactory"
)

// OrgRepoAndPullRequestNumber
func OrgRepoAndPullRequestNumber() cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if _, _, _, err := ParseOrgRepoAndPullRequestArgs(args); err != nil {
			return cmdfactory.FlagErrorf("%w", err)
		}

		return nil
	}
}

// ParseOrgRepoAndPullRequestArgs accepts input command-line arguments in the
// following forms:
//
//   - []string{"ORG/REPO/PRID"}, e.g. unikraft/unikraft/123
//   - []string{"https://github.com/org/repo/pull/123"}
//   - []string{"https://github.com/org/repo.git", "123"}
//   - Or with no args and when used in a GitHub Actions context, derived from
//     environmental variables.
//
// When none of the above formats are
func ParseOrgRepoAndPullRequestArgs(args []string) (string, string, int, error) {
	// If we are in a GitHub actions context and no arguments have been
	// specified, determine the values of org, repo and prId from the environment.
	if os.Getenv("GITHUB_ACTIONS") == "true" && len(args) == 0 {
		split := strings.SplitN(os.Getenv("GITHUB_REPOSITORY"), "/", 0)
		if len(split) != 2 {
			return "", "", 0, fmt.Errorf("could not parse environmental variable 'GITHUB_REPOSITORY': invalid format")
		}

		org, repo := split[0], split[1]

		split = strings.SplitN(os.Getenv("GITHUB_REF"), "/", 3)
		if len(split) != 3 {
			return "", "", 0, fmt.Errorf("could not parse environmental variable 'GITHUB_REF': invalid format")
		}

		prId, err := strconv.Atoi(split[2])
		if err != nil {
			return "", "", 0, fmt.Errorf("could not parse 'GITHUB_REF': expected reference to be pull request ID: %w", err)
		}

		return org, repo, prId, nil

	} else if len(args) == 1 {
		split := strings.SplitN(args[0], "/", 3)
		if len(split) != 3 {
			return "", "", 0, fmt.Errorf("expected format ORG/REPO/ID")
		}

		prId, err := strconv.Atoi(split[2])
		if err != nil {
			return "", "", 0, fmt.Errorf("PR ID is not numeric")
		}

		return split[0], split[1], prId, nil

	} else if len(args) == 2 {
		uri, err := url.ParseRequestURI(args[0])
		if err != nil {
			return "", "", 0, fmt.Errorf("expected URL: %w", err)
		}
		if uri.Host != "github.com" {
			return "", "", 0, fmt.Errorf("not a GitHub URL")
		}

		split := strings.SplitN(uri.Path, "/", 2)
		if len(split) != 2 {
			return "", "", 0, fmt.Errorf("expected GitHub URL to only have org/repo")
		}

		prId, err := strconv.Atoi(args[1])
		if err != nil {
			return "", "", 0, fmt.Errorf("expected second position argument to be numeric: %w", err)
		}

		return split[0], split[1], prId, nil

	} else if len(args) == 1 {
		uri, err := url.ParseRequestURI(args[0])
		if err != nil {
			return "", "", 0, fmt.Errorf("expected URL: %w", err)
		}
		if uri.Host != "github.com" {
			return "", "", 0, fmt.Errorf("not a GitHub URL")
		}

		if !strings.Contains(uri.Path, "/pull/") {
			return "", "", 0, fmt.Errorf("expected GitHub URL to contain pull request")
		}

		split := strings.SplitN(uri.Path, "/pull/", 2)
		if len(split) != 2 {
			return "", "", 0, fmt.Errorf("expected GitHub URL to contain pull request number")
		}

		orgRepo := split[0]

		prId, err := strconv.Atoi(strings.TrimSuffix(split[1], "/"))
		if err != nil {
			return "", "", 0, fmt.Errorf("expected GitHub URL to contain pull request number")
		}

		split = strings.SplitN(orgRepo, "/", 2)
		if len(split) != 2 {
			return "", "", 0, fmt.Errorf("expected GitHub URL to contain organization/user and repository")
		}

		return split[0], split[1], prId, nil
	}

	return "", "", 0, fmt.Errorf("could not parse arguments: invalid format: expected ORG/REPO/PRID")
}
