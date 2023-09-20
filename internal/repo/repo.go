package repo

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
