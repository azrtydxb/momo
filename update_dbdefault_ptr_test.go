// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// updPtr has NOT NULL columns whose defaults are true and 3, as dbdefault
// pointer fields.
type updPtr struct {
	ID     int64
	Name   string
	Active *bool
	Level  *int64
}

var updPtrMeta = momo.Meta[updPtr]{
	Table: "upd_ptr",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "name"},
		{Name: "active", Flags: momo.FlagHasDefault | momo.FlagDBDefault},
		{Name: "level", Flags: momo.FlagHasDefault | momo.FlagDBDefault},
	},
	Ptr:      func(m *updPtr, i int) any { return [...]any{&m.ID, &m.Name, &m.Active, &m.Level}[i] },
	Value:    func(m *updPtr, i int) any { return [...]any{m.ID, m.Name, m.Active, m.Level}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*updPtr) MomoMeta() *momo.Meta[updPtr] { return &updPtrMeta }

// Catches: Update.Model writing SET col = NULL for a nil dbdefault pointer,
// which fails on the NOT NULL column. nil means "not set": the column is left
// out and keeps its stored value; a pointer to false or 0 is still written.
func TestUpdateModelSkipsNilDBDefaultPointer(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	m := updPtr{ID: 1, Name: "a", Level: ptrTo(int64(0))}
	c, err := momo.Update[updPtr](db).Model(&m).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(c.SQL, `"active"`) {
		t.Fatalf("a nil dbdefault pointer must stay out of SET: %s", c.SQL)
	}
	if !strings.Contains(c.SQL, `"level"`) || !strings.Contains(c.SQL, `"name"`) {
		t.Fatalf("name and the explicit 0 must be set: %s", c.SQL)
	}
	for _, a := range c.Args {
		if a == nil {
			t.Fatalf("a NULL was bound: %v", c.Args)
		}
	}
	// An explicit false is written.
	m.Active = ptrTo(false)
	c, err = momo.Update[updPtr](db).Model(&m).SQL()
	if err != nil || !strings.Contains(c.SQL, `"active"`) {
		t.Fatalf("an explicit false must be set: %v %s", err, c.SQL)
	}
}

// Catches: a Model update whose only candidate columns are nil dbdefault
// pointers rendering an empty SET instead of the builder's own error.
func TestUpdateModelAllNilDBDefaultPointersIsEmptySet(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	m := updOnly{ID: 1}
	if _, err := momo.Update[updOnly](db).Model(&m).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation (no columns to update)", err)
	}
}

type updOnly struct {
	ID     int64
	Active *bool
}

var updOnlyMeta = momo.Meta[updOnly]{
	Table: "upd_only",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "active", Flags: momo.FlagHasDefault | momo.FlagDBDefault},
	},
	Ptr:      func(m *updOnly, i int) any { return [...]any{&m.ID, &m.Active}[i] },
	Value:    func(m *updOnly, i int) any { return [...]any{m.ID, m.Active}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*updOnly) MomoMeta() *momo.Meta[updOnly] { return &updOnlyMeta }
