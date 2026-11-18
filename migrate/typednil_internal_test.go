// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// A hand-built plan can carry a typed-nil pointer payload. The helpers that
// read index, unique and foreign-key payloads must treat it as absent, not
// dereference it.
func TestTypedNilPayloadsDoNotPanic(t *testing.T) {
	var (
		fk *schema.ForeignKey
		ix *schema.Index
		un *schema.Unique
	)
	tbl := schema.Table{Name: "t"}
	snap := func() *schema.Schema { return &schema.Schema{Tables: []schema.Table{tbl}} }

	t.Run("requires", func(t *testing.T) {
		requires(Change{Kind: AddFK, Table: "t", Name: "fk", After: fk})
		requires(Change{Kind: CreateIndex, Table: "t", Name: "ix", After: ix})
	})
	t.Run("refTableKey", func(t *testing.T) {
		if got := refTableKey(Change{Kind: AddFK, Table: "t", After: fk}); got != "" {
			t.Errorf("refTableKey = %q, want empty", got)
		}
	})
	t.Run("undoInTail", func(t *testing.T) {
		undoInTail(snap(), Change{Kind: DropIndex, Table: "t", Name: "ix", Before: ix}, true)
		undoInTail(snap(), Change{Kind: DropUnique, Table: "t", Name: "u", Before: un}, true)
		undoInTail(snap(), Change{Kind: DropFK, Table: "t", Name: "fk", Before: fk}, true)
	})
	t.Run("autoSkipReason", func(t *testing.T) {
		autoSkipReason(Change{Kind: CreateIndex, Table: "t", Name: "ix", After: ix}, &schema.Schema{}, map[string]bool{})
	})
}
