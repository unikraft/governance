package main

// SPDX-License-Identifier: BSD-3-Clause
//
// Authors: Alexander Jung <a.jung@lancs.ac.uk>
//
// Copyright (c) 2021, Lancaster University.  All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions
// are met:
//
// 1. Redistributions of source code must retain the above copyright
//    notice, this list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright
//    notice, this list of conditions and the following disclaimer in the
//    documentation and/or other materials provided with the distribution.
// 3. Neither the name of the copyright holder nor the names of its
//    contributors may be used to endorse or promote products derived from
//    this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
// ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
// LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
// CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
// SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
// CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
// ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
// POSSIBILITY OF SUCH DAMAGE.

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/unikraft/governance/internal/ghapi"
	"github.com/unikraft/governance/internal/label"
	"github.com/unikraft/governance/internal/repo"
	"github.com/unikraft/governance/internal/team"
)

type GlobalConfig struct {
	dryRun         bool
	labelsDir      string
	teamsDir       string
	reposDir       string
	githubOrg      string
	githubToken    string
	githubSkipSSL  bool
	githubEndpoint string
	tempDir        string
	ghApi          *ghapi.GithubClient
}

const (
	// The environment variable prefix of all environment variables bound to our
	// command line flags.  For example, --number is bound to STING_NUMBER.
	envPrefix = "GOVERN"
)

var (
	version      = "No version provided"
	commit       = "No commit provided"
	buildTime    = "No build timestamp provided"
	globalConfig = &GlobalConfig{}
	rootCmd      *cobra.Command

	// Global lists of populated definitions
	Labels []label.Label
	Teams  []*team.Team
	Repos  []*repo.Repository
)

// Build the cobra command that handles our command line tool.
func NewRootCommand() *cobra.Command {
	rootCmd = &cobra.Command{
		Use:   "governctl",
		Short: `Govern a GitHub organisation`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			showVer, err := cmd.Flags().GetBool("version")
			if err != nil {
				fmt.Printf("%s\n", err)
				os.Exit(1)
			}

			if showVer {
				fmt.Printf(
					"governctl %s (%s) built %s\n",
					version,
					commit,
					buildTime,
				)
				os.Exit(0)
			}

			verbose, err := cmd.Flags().GetBool("verbose")
			if err != nil {
				fmt.Printf("%s\n", err)
				os.Exit(1)
			}

			if err := initConfig(cmd); err != nil {
				return err
			}
			if err := initLogging(verbose); err != nil {
				return err
			}
			if err := loadDefinitions(); err != nil {
				return err
			}
			if err := initGithubClient(); err != nil {
				return err
			}

			// Check to create temporary directory
			tempDir, err := cmd.Flags().GetString("temp-dir")
			if len(tempDir) == 0 || err != nil {
				tempDir, err := ioutil.TempDir("", "governance")
				if err != nil {
					log.Fatalf("could not create temporary directory: %s", err)
					os.Exit(1)
				}

				globalConfig.tempDir = tempDir
			}

			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				cmd.Help() // nolint:errcheck
				os.Exit(0)
			}
		},
	}

	// Persistent global flags
	rootCmd.PersistentFlags().BoolVarP(
		&globalConfig.dryRun,
		"dry-run",
		"D",
		false,
		"Do not perform any actual change",
	)
	rootCmd.PersistentFlags().StringVarP(
		&globalConfig.labelsDir,
		"labels-dir",
		"l",
		"./labels",
		"Path to the labels definition directory",
	)
	rootCmd.PersistentFlags().StringVarP(
		&globalConfig.teamsDir,
		"teams-dir",
		"t",
		"./teams",
		"Path to the teams definition directory",
	)
	rootCmd.PersistentFlags().StringVarP(
		&globalConfig.reposDir,
		"repos-dir",
		"r",
		"./repos",
		"Path to the repos definition directory",
	)
	rootCmd.PersistentFlags().StringVarP(
		&globalConfig.githubOrg,
		"github-org",
		"O",
		"",
		"GitHub organisation to manipulate",
	)
	rootCmd.PersistentFlags().StringVarP(
		&globalConfig.githubToken,
		"github-token",
		"T",
		"",
		"GitHub API token",
	)
	rootCmd.PersistentFlags().StringVarP(
		&globalConfig.githubEndpoint,
		"github-endpoint",
		"E",
		"",
		"Alternative GitHub API endpoint (usually GitHub enterprise)",
	)
	rootCmd.PersistentFlags().BoolVarP(
		&globalConfig.githubSkipSSL,
		"github-skip-ssl",
		"S",
		false,
		"Skip SSL check with GitHub API endpoint",
	)
	rootCmd.PersistentFlags().StringVarP(
		&globalConfig.tempDir,
		"temp-dir",
		"j",
		"",
		"Temporary directory to store intermediate git clones",
	)
	rootCmd.PersistentFlags().BoolP(
		"verbose",
		"v",
		false,
		"Enable verbose logging",
	)
	rootCmd.PersistentFlags().BoolP(
		"version",
		"V",
		false,
		"Show version and quit",
	)

	// Subcommands
	rootCmd.AddCommand(syncTeamsCmd)
	rootCmd.AddCommand(syncPrCmd)

	return rootCmd
}

func initLogging(verbose bool) error {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)
	if verbose {
		log.SetLevel(log.TraceLevel)
	}

	return nil
}

func loadDefinitions() error {
	var err error

	Teams, err = team.NewListOfTeamsFromPath(
		globalConfig.ghApi,
		globalConfig.githubOrg,
		globalConfig.teamsDir,
	)
	if err != nil {
		return fmt.Errorf("could not populate teams: %s", err)
	}

	Repos, err = repo.NewListOfReposFromPath(
		globalConfig.ghApi,
		globalConfig.githubOrg,
		globalConfig.reposDir,
	)
	if err != nil {
		return fmt.Errorf("could not populate repos: %s", err)
	}

	Labels, err = label.NewListOfLabelsFromPath(
		globalConfig.ghApi,
		globalConfig.githubOrg,
		globalConfig.labelsDir,
	)
	if err != nil {
		return fmt.Errorf("could not populate repos: %s", err)
	}

	return nil
}

func initConfig(cmd *cobra.Command) error {
	v := viper.New()

	// When we bind flags to environment variables expect that the environment
	// variables are prefixed, e.g. a flag like --number binds to an environment
	// variable STING_NUMBER. This helps avoid conflicts.
	v.SetEnvPrefix(envPrefix)

	// Bind to environment variables.  Works great for simple config names, but
	// needs help for names like --favorite-color which we fix in the bindFlags
	// function
	v.AutomaticEnv()

	// Bind the current command's flags to viper
	bindFlags(cmd, v)

	return nil
}

// Bind each cobra flag to its associated viper configuration
func bindFlags(cmd *cobra.Command, v *viper.Viper) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// Environment variables can't have dashes in them, so bind them to their
		// equivalent keys with underscores, e.g. --favorite-color to
		// STING_FAVORITE_COLOR
		if strings.Contains(f.Name, "-") {
			envVarSuffix := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
			v.BindEnv(f.Name, fmt.Sprintf("%s_%s", envPrefix, envVarSuffix)) // nolint:errcheck
		}

		// Apply the viper config value to the flag when the flag is not set and
		// viper has a value
		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val)) // nolint:errcheck
		}
	})
}

func initGithubClient() error {
	ghApi, err := ghapi.NewGithubClient(
		globalConfig.githubOrg,
		globalConfig.githubToken,
		globalConfig.githubSkipSSL,
		globalConfig.githubEndpoint,
	)
	if err != nil {
		return err
	}

	globalConfig.ghApi = ghApi

	return nil
}

func main() {
	cmd := NewRootCommand()
	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
