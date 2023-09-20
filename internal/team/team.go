// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package team

import (
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	"github.com/unikraft/governance/internal/ghapi"
	"github.com/unikraft/governance/internal/repo"
	"github.com/unikraft/governance/internal/user"
)

type CodeReviewAlgorithm string

const (
	RoundRobin  CodeReviewAlgorithm = "rr"
	LoadBalance CodeReviewAlgorithm = "lb"
)

type CodeReview struct {
	NumReviewers         int                 `yaml:"num_reviewers,omitempty"`
	Algorithm            CodeReviewAlgorithm `yaml:"algorithm,omitempty"`
	NeverAssign          []user.User         `yaml:"never_assign,omitempty"`
	DontNotifyTeam       bool                `yaml:"dont_notify_team,omitempty"`
	IncludeChildTeams    bool                `yaml:"include_child_teams,omitempty"`
	RemoveReviewRequest  bool                `yaml:"remove_review_request,omitempty"`
	CountExistingMembers bool                `yaml:"count_existing_members,omitempty"`
}

type TeamType string

const (
	SIGTeam         TeamType = "sig"
	MaintainersTeam TeamType = "maintainers"
	ReviewersTeam   TeamType = "reviewers"
	MiscTeam        TeamType = "misc"
)

var (
	TeamTypes = []TeamType{
		SIGTeam,
		MaintainersTeam,
		ReviewersTeam,
	}
)

type TeamPrivacy string

const (
	TeamClosed TeamPrivacy = "closed"
	TeamSecret TeamPrivacy = "secret"
)

type Team struct {
	ghApi        *ghapi.GithubClient
	Org          string
	fullname     string
	Name         string      `yaml:"name,omitempty"`
	Type         TeamType    `yaml:"type,omitempty"`
	Privacy      TeamPrivacy `yaml:"privacy,omitempty"`
	Parent       string      `yaml:"parent,omitempty"`
	ParentTeam   *Team
	Description  string            `yaml:"description,omitempty"`
	CodeReview   CodeReview        `yaml:"code_review,omitempty"`
	Maintainers  []user.User       `yaml:"maintainers,omitempty"`
	Reviewers    []user.User       `yaml:"reviewers,omitempty"`
	Members      []user.User       `yaml:"members,omitempty"`
	Repositories []repo.Repository `yaml:"repos,omitempty"`
	hasSynced    bool
	shortName    string
}

func FindTeamByName(a string, teams []*Team) *Team {
	if a[0] == '@' {
		split := strings.Split(a, "/")
		a = split[1]
	}

	for _, b := range teams {
		// Check if the name is equal verbatim
		if b.Name == a {
			return b
		}

		// Check if the team type has been prefixed
		for _, t := range TeamTypes {
			if fmt.Sprintf("%s-%s", t, b.Name) == a {
				return b
			}
		}
	}

	return nil
}

func (r *Team) Fullname() string {
	if r.fullname != "" {
		return r.fullname
	}

	if strings.Contains(r.Name, "-") {
		split := strings.Split(r.Name, "-")
		n := strings.Join(split[1:], "-")

		for _, t := range TeamTypes {
			if split[0] == string(t) {
				r.Name = n
				r.Type = t
				break
			}
		}
	}

	if r.Type == MiscTeam {
		r.fullname = r.Name
	} else {
		r.fullname = fmt.Sprintf("%s-%s", r.Type, r.Name)
	}

	return r.fullname
}

func NewTeamFromYAML(ghApi *ghapi.GithubClient, githubOrg, teamsFile string) (*Team, error) {
	yamlFile, err := ioutil.ReadFile(teamsFile)
	if err != nil {
		return nil, fmt.Errorf("could not open yaml file: %s", err)
	}

	team := &Team{
		ghApi: ghApi,
	}

	err = yaml.Unmarshal(yamlFile, team)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal yaml file: %s", err)
	}

	// Let's perform a sanity check and check if we have at least the name of the
	// team.
	if team.Name == "" {
		return nil, fmt.Errorf("team name not provided for %s", teamsFile)
	}

	if strings.Contains(team.Name, "-") {
		split := strings.Split(team.Name, "-")
		n := strings.Join(split[1:], "-")

		for _, t := range TeamTypes {
			if split[0] == string(t) {
				team.Name = n
				team.Type = t
				break
			}
		}
	}

	// Now let's check if all maintainers, reviewers and members have at least
	// their Github username provided.
	users := append(team.Maintainers, team.Reviewers...)
	users = append(users, team.Members...)
	for _, user := range users {
		if user.Github == "" {
			return nil, fmt.Errorf("user does not have github username: %s", user.Name)
		}
	}

	return team, nil
}

func NewListOfTeamsFromPath(ghApi *ghapi.GithubClient, githubOrg, teamsDir string) ([]*Team, error) {
	teams := make([]*Team, 0)

	files, err := ioutil.ReadDir(teamsDir)
	if err != nil {
		return nil, fmt.Errorf("could not read directory: %s", err)
	}

	// To solve a potential dependency problem where teams are dependent on teams
	// which do not exist, we are going to populate a list "processed" teams first
	// and then check if any of the teams has a parent which does not exist in the
	// list which we have just populated.

	// Iterate through all files and populate a list of known teams.
	for _, file := range files {
		t, err := NewTeamFromYAML(
			ghApi,
			githubOrg,
			path.Join(teamsDir, file.Name()),
		)
		if err != nil {
			return nil, fmt.Errorf("could not parse teams file: %s", err)
		}

		teams = append(teams, t)
	}

	// Now iterate through known teams and match parents
	for _, t := range teams {
		if t.Parent != "" {
			parent := FindTeamByName(t.Parent, teams)
			if parent != nil {
				t.ParentTeam = parent
				break
			} else {
				// We might be lucky... it may exist upstream when we later call the
				// Github API.  If it doesn't then we're in trouble...
				log.Warnf("cannot find parent from provided teams: %s", t.Parent)
			}
		}
	}

	return teams, nil
}
