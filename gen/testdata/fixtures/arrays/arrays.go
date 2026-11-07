// SPDX-License-Identifier: Apache-2.0

package arrays

import "github.com/azrtydxb/momo"

// Contact carries slice-of-scalar fields that map to PostgreSQL arrays.
type Contact struct {
	momo.Model `momo:"table:contacts"`
	ID         int64     `momo:"pk,identity"`
	Products   []string  `momo:"notnull"`
	Scores     []int32   `momo:"notnull"`
	Counts     []int64   `momo:"notnull"`
	Flags      []bool    `momo:"notnull"`
	Weights    []float64 `momo:"notnull"`
	Aliases    []string
}
