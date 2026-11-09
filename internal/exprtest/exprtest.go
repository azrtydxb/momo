// SPDX-License-Identifier: Apache-2.0

// Package exprtest holds the render-only fixtures that the tests of a feature
// package (arrayop, fts, ...) need to see the SQL and arguments of a
// predicate or expression: a one-column projection row and a DB over the
// recording fake driver. Nothing here runs a statement.
package exprtest

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// Tally is a projection fixture with a single count column and no table of its
// own. Select it with Columns(momo.CountAll()).
type Tally struct{ N int64 }

var tallyMeta = momo.Meta[Tally]{
	Columns:    []momo.ColumnMeta{{Name: "n"}},
	Ptr:        func(m *Tally, _ int) any { return &m.N },
	Value:      func(m *Tally, _ int) any { return m.N },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
}

// MomoMeta makes Tally a model for momo.Select.
func (*Tally) MomoMeta() *momo.Meta[Tally] { return &tallyMeta }

// DB opens a DB for the dialect over a driver that records statements and
// runs none.
func DB(name dialect.Name) *momo.DB { return momo.New(fakedriver.New(name)) }

// Compile renders the predicate in "SELECT COUNT(*) FROM docs WHERE p" for the
// dialect and returns the statement.
func Compile(name dialect.Name, p momo.Predicate) (momo.Compiled, error) {
	return momo.Select[Tally](DB(name)).
		From(momo.TableRef{Name: "docs"}).
		Columns(momo.CountAll()).
		Where(p).
		SQL()
}
