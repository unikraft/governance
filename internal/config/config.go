// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package config

type Config struct {
	DryRun         bool   `long:"dry-run" short:"D" env:"GOVERN_DRY_RUN" usage:"Do not perform any actual change."`
	GithubUser     string `long:"github-user" env:"GOVERN_GITHUB_USER" usage:"GitHub User account name"`
	GithubToken    string `long:"github-token" env:"GOVERN_GITHUB_TOKEN" usage:"GitHub API token"`
	GithubEndpoint string `long:"github-endpoint" env:"GOVERN_GITHUB_ENDPOINT" short:"E" usage:"Alternative GitHub API endpoint (usually GitHub enterprise)"`
	GithubSkipSSL  bool   `long:"github-skip-ssl" short:"S" env:"GOVERN_GITHUB_SKIP_SSL" usage:"Skip SSL check with GitHub API endpoint"`
	LogLevel       string `long:"log-level" short:"l" env:"GOVERN_LOG_LEVEL" usage:"Log level verbosity" default:"info"`
	NoRender       bool   `long:"no-render" env:"GOVERN_NO_RENDER" usage:"Do not render the output"`
	ReposDir       string `long:"repos-dir" short:"r" env:"GOVERN_REPOS_DIR" usage:"Path to the repos definition directory" default:"repos"`
	TeamsDir       string `long:"teams-dir" short:"T" env:"GOVERN_TEAMS_DIR" usage:"Path to the teams definition directory" default:"teams"`
	TempDir        string `long:"temp-dir" short:"j" env:"GOVERN_TEMP_DIR" usage:"Temporary directory to store intermediate git clones"`
}
