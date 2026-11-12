// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a VIRTUAL generated column rendered as STORED on PostgreSQL, which
// would silently create the wrong kind (PostgreSQL has VIRTUAL only from 18;
// momo supports 16 and 17).
func TestRenderVirtualGeneratedIsUnsupported(t *testing.T) {
	virtual := schema.Column{Name: "a", Type: "bigint", Nullable: true, Generated: "id * 2", GeneratedVirtual: true}
	tbl := schema.Table{Name: "vg", Columns: []schema.Column{{Name: "id", Type: "bigint"}, virtual}, PrimaryKey: []string{"id"}}
	for name, c := range map[string]migrate.Change{
		"create table": {Kind: migrate.CreateTable, Table: "vg", After: tbl},
		"add column":   {Kind: migrate.AddColumn, Table: "vg", Name: "a", After: virtual},
	} {
		if _, err := (mpg.Dialect{}).Render([]migrate.Change{c}); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: err = %v, want momo.ErrUnsupported", name, err)
		}
	}
	// A kind change with the same expression is refused too.
	stored := virtual
	stored.GeneratedVirtual = false
	if _, err := (mpg.Dialect{}).Render([]migrate.Change{{Kind: migrate.AlterColumn, Table: "vg", Name: "a", Before: stored, After: virtual}}); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("alter to VIRTUAL: err = %v, want momo.ErrUnsupported", err)
	}
	if _, err := (mpg.Dialect{}).Render([]migrate.Change{{Kind: migrate.AlterColumn, Table: "vg", Name: "a", Before: virtual, After: stored}}); err == nil {
		t.Error("alter VIRTUAL to STORED: want an error, got none")
	}
}
