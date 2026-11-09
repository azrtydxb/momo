// SPDX-License-Identifier: Apache-2.0

package arrayop_test

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// tally is a projection fixture: it has no table of its own.
type tally struct{ N int64 }

var tallyMeta = momo.Meta[tally]{
	Columns:    []momo.ColumnMeta{{Name: "n"}},
	Ptr:        func(m *tally, _ int) any { return &m.N },
	Value:      func(m *tally, _ int) any { return m.N },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
}

func (*tally) MomoMeta() *momo.Meta[tally] { return &tallyMeta }

// dbFor opens a DB over a driver that records statements and runs none.
func dbFor(name dialect.Name) *momo.DB { return momo.New(fakedriver.New(name)) }
