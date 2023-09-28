// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

// Package patch provides support for generating a patch and patch file between
// two commits.
package patch

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	gitobject "github.com/go-git/go-git/v5/plumbing/object"
	"github.com/sirupsen/logrus"
	"kraftkit.sh/log"
)

// Patch represents a specific commit and all the metadata associated with the
// specific commit.
type Patch struct {
	Title       string
	Hash        string
	Message     string
	Trailers    []string
	AuthorName  string
	AuthorEmail string
	AuthorDate  string
	Filename    string
	Stat        string
	Diff        string

	// patch *gitobject.Patch
}

// NewPatchFromCommits accepts two commits which are used to generate a patch.
func NewPatchFromCommits(ctx context.Context, repoPath string, commit, diff *gitobject.Commit) (*Patch, error) {
	if commit == nil {
		return nil, fmt.Errorf("cannot create patchfile without commit")
	} else if diff == nil {
		return nil, fmt.Errorf("cannot create patchfile without diff")
	}

	split := strings.Split(commit.Message, "\n")

	patch := Patch{
		Title:       split[0],
		AuthorName:  commit.Author.Name,
		AuthorEmail: commit.Author.Email,
		AuthorDate:  commit.Author.When.Format(time.RFC1123Z),
		Hash:        commit.Hash.String(),
	}

	var message []string

	for _, line := range split[1:] {
		isTrailer := false
		for _, trailer := range Trailers() {
			if strings.HasPrefix(strings.ToLower(line), strings.ToLower(trailer)+":") {
				isTrailer = true
				patch.Trailers = append(patch.Trailers, line)
				break
			}
		}

		if !isTrailer {
			message = append(message, line)
		}
	}

	// Remove trailing new lines
	if message[len(message)-1] == "" {
		message = message[:len(message)-1]
	}

	patch.Message = strings.Join(message, "\n")

	var buf bytes.Buffer

	gitShow := exec.CommandContext(ctx,
		"git",
		"-C", repoPath,
		"show",
		"--stat",
		"--oneline",
		// "--format=\"\"", // TODO(nderjung): Should work, but it doesn't.
		"--no-color",
		fmt.Sprintf("%s...%s", diff.Hash, commit.Hash),
	)
	gitShow.Stderr = log.G(ctx).WriterLevel(logrus.ErrorLevel)
	gitShow.Stdout = &buf
	if err := gitShow.Run(); err != nil {
		return nil, fmt.Errorf("could not generate stats: %w", err)
	}

	split = strings.Split(buf.String(), "\n")
	patch.Stat = strings.Join(split[1:], "\n")

	buf = *bytes.NewBuffer(nil)

	gitDiff := exec.CommandContext(ctx,
		"git",
		"-C",
		repoPath,
		"diff",
		fmt.Sprintf("%s..%s", diff.Hash, commit.Hash),
	)
	gitDiff.Stderr = log.G(ctx).WriterLevel(logrus.ErrorLevel)
	gitDiff.Stdout = &buf
	if err := gitDiff.Run(); err != nil {
		return nil, fmt.Errorf("could not generate patch: %w", err)
	}

	patch.Diff = buf.String()

	// XXX(nderjung): This "native" method does not work:
	// var err error
	// patch.patch, err = commit.PatchContext(ctx, diff)
	// if err != nil {
	// 	return nil, fmt.Errorf("could not generate patch: %w", err)
	// }

	return &patch, nil
}

func (p *Patch) message() *bytes.Buffer {
	var b bytes.Buffer

	b.WriteString("From ")
	b.WriteString(p.Hash)
	b.WriteString("\n")
	b.WriteString("From: ")
	b.WriteString(p.AuthorName)
	b.WriteString(" <")
	b.WriteString(p.AuthorEmail)
	b.WriteString(">\n")
	b.WriteString("Date: ")
	b.WriteString(p.AuthorDate)
	b.WriteString("\n")
	b.WriteString("Subject: [PATCH] ")
	b.WriteString(p.Title)
	b.WriteString("\n")
	b.WriteString(p.Message)
	b.WriteString("\n")
	b.WriteString(strings.Join(p.Trailers, "\n"))
	b.WriteString("\n---\n")
	b.WriteString(p.Stat)
	b.WriteString("\n")
	b.WriteString(p.Diff)
	// TODO(nderjung): Set this version dynamically. How much does it matter?
	b.WriteString("-- \n2.39.2\n\n")

	return &b
}

func (p *Patch) String() string {
	return p.message().String()
}

func (p *Patch) Bytes() []byte {
	return p.message().Bytes()
}
