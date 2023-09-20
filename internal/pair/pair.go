// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

// Pair is a simple package used to calculate a rank of items in a list.
package pair

import (
	"sort"
)

type Pair struct {
	Key   string
	Value int
}

type PairList []Pair

func (p PairList) Len() int {
	return len(p)
}

func (p PairList) Less(i, j int) bool {
	return p[i].Value < p[j].Value
}

func (p PairList) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func RankByWorkload(users map[string]int) PairList {
	pl := make(PairList, len(users))
	i := 0

	for k, v := range users {
		pl[i] = Pair{k, v}
		i++
	}

	sort.Sort(pl)
	return pl
}
