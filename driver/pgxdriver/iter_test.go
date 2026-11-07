// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"fmt"
	"os"

	"github.com/azrtydxb/momo"
)

// iterTable is unique per test process so concurrent test runs against the
// same database cannot collide.
var iterTable = fmt.Sprintf("iter_rows_%d", os.Getpid())

type iterRow struct{ ID int64 }

func (*iterRow) MomoMeta() *momo.Meta[iterRow] {
	return &momo.Meta[iterRow]{
		Table:   iterTable,
		Columns: []momo.ColumnMeta{{Name: "id"}},
		Ptr:     func(m *iterRow, i int) any { return &m.ID },
		Value:   func(m *iterRow, i int) any { return m.ID },
		// Index 0 would otherwise read as the soft-delete column and add
		// "id IS NULL" to every query.
		Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	}
}
