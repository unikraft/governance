// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The Unikraft Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package user

type UserRole string

const (
	Admin      UserRole = "admin"
	Maintainer UserRole = "maintainer"
	Reviewer   UserRole = "reviewer"
	Member     UserRole = "member"
)

type User struct {
	Name    string   `yaml:"name,omitempty"`
	Email   string   `yaml:"email,omitempty"`
	Github  string   `yaml:"github,omitempty"`
	Discord string   `yaml:"discord,omitempty"`
	Role    UserRole `yaml:"role,omitempty"`
}
