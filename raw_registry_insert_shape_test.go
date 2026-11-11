// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

var shapeAt = momo.NewColumn[time.Time]("dbd_rows", "at", 0)

func compileShape(t *testing.T, build func(q momo.Querier) momo.Compilable) string {
	t.Helper()
	r := momo.NewQueryRegistry()
	r.Register("q", build)
	got, err := r.Compile(dialect.Postgres)
	if err != nil || len(got) != 1 {
		t.Fatalf("Compile = %+v, %v", got, err)
	}
	return got[0].SQL
}

// Catches: the registry docs promising a stable shape that Insert.Model does
// not have. The SQL of Model depends on whether the dbdefault fields are set,
// so a sample must be set as a real call sets it.
func TestRegistryInsertModelSQLDependsOnDBDefaultValues(t *testing.T) {
	zero := compileShape(t, func(q momo.Querier) momo.Compilable {
		return momo.Insert[dbdRow](q).Model(&dbdRow{Title: "x"})
	})
	set := compileShape(t, func(q momo.Querier) momo.Compilable {
		return momo.Insert[dbdRow](q).Model(&dbdRow{ID: "i", Title: "x", At: dbdStamp})
	})
	if zero != `INSERT INTO "dbd_rows" ("title") VALUES ($1)` {
		t.Errorf("zero dbdefault fields: %s", zero)
	}
	if set != `INSERT INTO "dbd_rows" ("id", "title", "at") VALUES ($1, $2, $3)` {
		t.Errorf("set dbdefault fields: %s", set)
	}
}

// Catches: the explicit column list of Values (the documented stable shape)
// starting to depend on the values bound, for example by skipping zero values.
func TestRegistryInsertValuesSQLDoesNotDependOnValues(t *testing.T) {
	build := func(id, title string, at time.Time) func(momo.Querier) momo.Compilable {
		return func(q momo.Querier) momo.Compilable {
			return momo.Insert[dbdRow](q).Values(dbdID.Set(id), dbdTitle.Set(title), shapeAt.Set(at))
		}
	}
	zero := compileShape(t, build("", "", time.Time{}))
	set := compileShape(t, build("i", "x", dbdStamp))
	const want = `INSERT INTO "dbd_rows" ("id", "title", "at") VALUES ($1, $2, $3)`
	if zero != want || set != want {
		t.Errorf("zero values: %s\nset values:  %s\nwant:        %s", zero, set, want)
	}
}

// Catches: Omit ceasing to give Model a value-independent column list.
func TestRegistryInsertModelOmitFixesTheShape(t *testing.T) {
	build := func(m dbdRow) func(momo.Querier) momo.Compilable {
		return func(q momo.Querier) momo.Compilable {
			return momo.Insert[dbdRow](q).Omit(dbdID, shapeAt).Model(&m)
		}
	}
	zero := compileShape(t, build(dbdRow{}))
	set := compileShape(t, build(dbdRow{ID: "i", Title: "x", At: dbdStamp}))
	const want = `INSERT INTO "dbd_rows" ("title") VALUES ($1)`
	if zero != want || set != want {
		t.Errorf("zero: %s\nset:  %s\nwant: %s", zero, set, want)
	}
}
