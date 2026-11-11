// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// dbdPtrRow has NOT NULL columns whose defaults are true and 3, as pointer
// fields: nil means "use the database default", any other value is written,
// false and 0 included (issue 90).
type dbdPtrRow struct {
	Name   string
	Active *bool
	Level  *int64
}

var dbdPtrMeta = momo.Meta[dbdPtrRow]{
	Table: "dbd_ptr",
	Columns: []momo.ColumnMeta{
		{Name: "name"},
		{Name: "active", Flags: momo.FlagHasDefault | momo.FlagDBDefault},
		{Name: "level", Flags: momo.FlagHasDefault | momo.FlagDBDefault},
	},
	Ptr:   func(m *dbdPtrRow, i int) any { return [...]any{&m.Name, &m.Active, &m.Level}[i] },
	Value: func(m *dbdPtrRow, i int) any { return [...]any{m.Name, m.Active, m.Level}[i] },
	TableSchema: &schema.Table{
		Name: "dbd_ptr",
		Columns: []schema.Column{
			{Name: "name", Type: "text"},
			{Name: "active", Type: "boolean", Default: "true"},
			{Name: "level", Type: "bigint", Default: "3"},
		},
	},
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*dbdPtrRow) MomoMeta() *momo.Meta[dbdPtrRow] { return &dbdPtrMeta }

func ptrTo[T any](v T) *T { return &v }

// deref returns the value a bound argument holds, whether the driver got the
// value or a pointer to it.
func deref(v any) any {
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer && !rv.IsNil() {
		return rv.Elem().Interface()
	}
	return v
}

// Catches: a nil pointer on a dbdefault column written as NULL (a NOT NULL
// violation) instead of left to the database default, or the stored value not
// read back into the pointer.
func TestDBDefaultPointerNilTakesTheDefault(t *testing.T) {
	db, drv := dbdSetup(dialect.Postgres, [][]any{{true, int64(3)}})
	m := dbdPtrRow{Name: "a"}
	if _, err := momo.Insert[dbdPtrRow](db).Model(&m).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := lastSQL(t, drv)
	if want := `INSERT INTO "dbd_ptr" ("name") VALUES ($1) RETURNING "active", "level"`; c.SQL != want {
		t.Fatalf("SQL = %s, want %s", c.SQL, want)
	}
	if m.Active == nil || !*m.Active || m.Level == nil || *m.Level != 3 {
		t.Fatalf("pointers not set to the read-back values: %+v", m)
	}
}

// Catches: a pointer to false or 0 treated as unset, so the database default
// replaces the value the caller set.
func TestDBDefaultPointerExplicitFalseAndZeroAreWritten(t *testing.T) {
	db, drv := dbdSetup(dialect.Postgres, nil)
	m := dbdPtrRow{Name: "a", Active: ptrTo(false), Level: ptrTo(int64(0))}
	if _, err := momo.Insert[dbdPtrRow](db).Model(&m).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := lastSQL(t, drv)
	if want := `INSERT INTO "dbd_ptr" ("name", "active", "level") VALUES ($1, $2, $3)`; c.SQL != want {
		t.Fatalf("SQL = %s, want %s", c.SQL, want)
	}
	if len(c.Args) != 3 || deref(c.Args[1]) != false || deref(c.Args[2]) != int64(0) {
		t.Fatalf("args = %v", c.Args)
	}
}

// Catches: a mixed batch (nil, false, true) writing one value for all rows
// instead of DEFAULT for the nil row and the given value for the others, or
// the read-back overwriting the pointers the caller set.
func TestDBDefaultPointerModelsMixNilAndExplicit(t *testing.T) {
	db, drv := dbdSetup(dialect.Postgres, [][]any{
		{true, int64(3)}, {false, int64(0)}, {true, int64(9)},
	})
	ms := []dbdPtrRow{
		{Name: "a"},
		{Name: "b", Active: ptrTo(false), Level: ptrTo(int64(0))},
		{Name: "c", Active: ptrTo(true), Level: ptrTo(int64(9))},
	}
	if _, err := momo.Insert[dbdPtrRow](db).Models(ms).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := lastSQL(t, drv)
	if want := `INSERT INTO "dbd_ptr" ("name", "active", "level") VALUES ($1, DEFAULT, DEFAULT), ($2, $3, $4), ($5, $6, $7) RETURNING "active", "level"`; c.SQL != want {
		t.Fatalf("SQL = %s, want %s", c.SQL, want)
	}
	if ms[0].Active == nil || !*ms[0].Active || ms[0].Level == nil || *ms[0].Level != 3 {
		t.Fatalf("the nil row is not read back: %+v", ms[0])
	}
	if ms[1].Active == nil || *ms[1].Active || ms[1].Level == nil || *ms[1].Level != 0 {
		t.Fatalf("an explicit false/0 row changed: %+v", ms[1])
	}
}
