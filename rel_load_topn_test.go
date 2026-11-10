// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

func ids(cs []models.Comment) []int64 {
	out := []int64{}
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return out
}
