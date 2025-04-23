// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.
package label

import (
	"fmt"
	"io/ioutil"
	"path"
	"time"

	doublestar "github.com/bmatcuk/doublestar/v4"
	"github.com/unikraft/governance/internal/ghapi"
	"gopkg.in/yaml.v2"
)

type Label struct {
	ghApi                    *ghapi.GithubClient
	Name                     string        `yaml:"name"`
	Description              string        `yaml:"description"`
	Color                    string        `yaml:"color"`
	ApplyOnPrMatchRepos      []string      `yaml:"apply_on_pr_match_repos"`
	ApplyOnPrMatchPaths      []string      `yaml:"apply_on_pr_match_paths"`
	ApplyAfter               time.Duration `yaml:"apply_after"`
	RemoveAfter              time.Duration `yaml:"remove_after"`
	DoNotRemoveIfLabelsExist []string      `yaml:"do_not_remove_if_labels_exist"`
}

type Labels struct {
	Labels []Label `yaml:"labels"`
}

func NewListOfLabelsFromYAML(ghApi *ghapi.GithubClient, githubOrg, labelsFile string) ([]Label, error) {
	yamlFile, err := ioutil.ReadFile(labelsFile)
	if err != nil {
		return nil, fmt.Errorf("could not open yaml file: %s", err)
	}

	allLabels := &Labels{}
	labels := make([]Label, 0)

	err = yaml.Unmarshal(yamlFile, allLabels)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal yaml file: %s", err)
	}

	for _, label := range allLabels.Labels {
		// Let's perform a sanity check and check if we have at least the name of the
		// label.
		if label.Name == "" {
			return nil, fmt.Errorf("label name not provided for %s", labelsFile)
		}

		label.ghApi = ghApi
		labels = append(labels, label)
	}

	return labels, nil
}

func NewListOfLabelsFromPath(ghApi *ghapi.GithubClient, githubOrg, labelsDir string) ([]Label, error) {
	labels := make([]Label, 0)

	files, err := ioutil.ReadDir(labelsDir)
	if err != nil {
		return nil, fmt.Errorf("could not read directory: %s", err)
	}

	// Iterate through all files and populate a list of known labels.
	for _, file := range files {
		l, err := NewListOfLabelsFromYAML(
			ghApi,
			githubOrg,
			path.Join(labelsDir, file.Name()),
		)
		if err != nil {
			return nil, fmt.Errorf("could not parse labels file: %s", err)
		}

		labels = append(labels, l...)
	}

	return labels, nil
}

func (l *Label) AppliesTo(repo, file string) bool {
	if l.ApplyOnPrMatchRepos != nil && len(l.ApplyOnPrMatchRepos) > 0 {
		for _, c := range l.ApplyOnPrMatchRepos {
			if c == repo {
				goto checkMatchPaths
			}
		}

		return false
	}

checkMatchPaths:
	if l.ApplyOnPrMatchPaths != nil && len(l.ApplyOnPrMatchPaths) > 0 {
		for _, p := range l.ApplyOnPrMatchPaths {
			if ok, _ := doublestar.Match(p, file); ok {
				return true
			}
		}
	}

	return false
}
