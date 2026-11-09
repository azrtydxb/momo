// SPDX-License-Identifier: Apache-2.0

package exprtest

import "github.com/azrtydxb/momo"

// Pair is a two-column projection fixture with no table of its own, for
// rendering a select list of an expression and a count, for example a grouped
// extraction. Select it with Columns(expr, momo.CountAll()).
type Pair struct {
	K int64
	N int64
}

var pairMeta = momo.Meta[Pair]{
	Columns:    []momo.ColumnMeta{{Name: "k"}, {Name: "n"}},
	Ptr:        func(m *Pair, i int) any { return [...]any{&m.K, &m.N}[i] },
	Value:      func(m *Pair, i int) any { return [...]any{m.K, m.N}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
}

// MomoMeta makes Pair a model for momo.Select.
func (*Pair) MomoMeta() *momo.Meta[Pair] { return &pairMeta }
