// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package repo

import (
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/unikraft/governance/internal/ghapi"
)

type RepoType string

const (
	RepoTypeApp  RepoType = "app"
	RepoTypeLib  RepoType = "lib"
	RepoTypePlat RepoType = "plat"
	RepoTypeCore RepoType = "core"
	RepoTypeMisc RepoType = "misc"
)

var (
	RepoTypes = [...]RepoType{
		RepoTypeApp,
		RepoTypeLib,
		RepoTypePlat,
		RepoTypeCore,
		RepoTypeMisc,
	}
)

type RepoPermissionLevel string

const (
	RepoPermissionRead     RepoPermissionLevel = "read"
	RepoPermissionTriage   RepoPermissionLevel = "triage"
	RepoPermissionWrite    RepoPermissionLevel = "write"
	RepoPermissionMaintain RepoPermissionLevel = "maintain"
	RepoPermissionAdmin    RepoPermissionLevel = "admin"
)

type Repository struct {
	ghApi           *ghapi.GithubClient
	Type            RepoType `yaml:"type,omitempty"`
	Origin          string   `yaml:"origin,omitempty"`
	fullname        string
	Name            string              `yaml:"name,omitempty"`
	PermissionLevel RepoPermissionLevel `yaml:"permission,omitempty"`
}

func (r *Repository) NameEquals(name string) bool {
	if name == r.Name {
		return true
	}

	for _, t := range RepoTypes {
		if fmt.Sprintf("%s-%s", t, r.Name) == name {
			return true
		}
	}

	return false
}

func (r *Repository) Fullname() string {
	if r.fullname != "" {
		return r.fullname
	}

	if strings.Contains(r.Name, "-") {
		split := strings.Split(r.Name, "-")
		n := strings.Join(split[1:], "-")

		for _, t := range RepoTypes {
			if split[0] == string(t) {
				r.Name = n
				r.Type = t
				break
			}
		}
	}

	if r.Type == RepoTypeMisc || r.Type == RepoTypeCore {
		r.fullname = r.Name
	} else {
		r.fullname = fmt.Sprintf("%s-%s", r.Type, r.Name)
	}

	return r.fullname
}

func FindRepoByName(a string, repos []*Repository) *Repository {
	for _, b := range repos {
		if b.Name == a {
			return b
		}
	}

	// If we made it this far, let's try again but try and remove potential
	// any type-prefixing, e.g. "lib-lwip", "app-duktape"
	if strings.Contains(a, "-") {
		split := strings.Split(a, "-")
		n := strings.Join(split[1:], "-")

		for _, b := range repos {
			if b.Name == n {
				return b
			}
		}
	}

	return nil
}

func NewTeamFromYAML(ghApi *ghapi.GithubClient, githubOrg, reposFile string) (*Repository, error) {
	yamlFile, err := ioutil.ReadFile(reposFile)
	if err != nil {
		return nil, fmt.Errorf("could not open yaml file: %s", err)
	}

	repo := &Repository{
		ghApi: ghApi,
	}

	err = yaml.Unmarshal(yamlFile, repo)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal yaml file: %s", err)
	}

	// Let's perform a sanity check and check if we have at least the name of the
	// repo.
	if repo.Name == "" {
		return nil, fmt.Errorf("repo name not provided for %s", reposFile)
	}

	// Let's set the remote path to this repository
	repo.Origin = fmt.Sprintf(
		"https://github.com/%s/%s.git",
		githubOrg,
		repo.Fullname(),
	)

	return repo, nil
}

func NewListOfReposFromPath(ghApi *ghapi.GithubClient, githubOrg, reposDir string) ([]*Repository, error) {
	repos := make([]*Repository, 0)

	files, err := ioutil.ReadDir(reposDir)
	if err != nil {
		return nil, fmt.Errorf("could not read directory: %s", err)
	}

	// Iterate through all files and populate a list of known repos.
	for _, file := range files {
		r, err := NewTeamFromYAML(
			ghApi,
			githubOrg,
			path.Join(reposDir, file.Name()),
		)
		if err != nil {
			return nil, fmt.Errorf("could not parse repos file: %s", err)
		}

		repos = append(repos, r)
	}

	return repos, nil
}
