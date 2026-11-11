// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

func unnestColumnQuery(d *momo.DB, src momo.UnnestColumnSource[string]) (momo.Compiled, error) {
	return momo.Select[anKT](d).From(momo.TableRef{Name: "owners", Alias: "q2"}).
		Columns(momo.ColumnOf[int64]("q2", "id"), src.Column()).
		CrossJoin(src).SQL()
}

// Catches: UnnestColumn rendering without LATERAL or without the column
// alias list, or binding the array as a parameter.
func TestUnnestColumnRendersLateral(t *testing.T) {
	flags := momo.ColumnOf[[]string]("q2", "flags")
	got, err := unnestColumnQuery(typedDB(dialect.Postgres), momo.UnnestColumn(flags).As("fl", "flag"))
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "q2"."id", "fl"."flag" FROM "owners" AS "q2" CROSS JOIN LATERAL unnest("q2"."flags") AS "fl" ("flag")`
	if got.SQL != want {
		t.Fatalf("got  %s\nwant %s", got.SQL, want)
	}
	if len(got.Args) != 0 {
		t.Fatalf("args = %#v, want none", got.Args)
	}
}

// Catches: UnnestColumn building on a dialect without unnest, or letting an
// unnamed or unsafe alias reach the SQL text.
func TestUnnestColumnErrors(t *testing.T) {
	flags := momo.ColumnOf[[]string]("q2", "flags")
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		if _, err := unnestColumnQuery(typedDB(d), momo.UnnestColumn(flags).As("fl", "flag")); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", d, err)
		}
	}
	pg := typedDB(dialect.Postgres)
	for name, src := range map[string]momo.UnnestColumnSource[string]{
		"no alias":    momo.UnnestColumn(flags),
		"bad alias":   momo.UnnestColumn(flags).As(`f"; DROP`, "flag"),
		"nil array":   momo.UnnestColumn[string](nil).As("fl", "flag"),
		"empty named": momo.UnnestColumn(flags).As("fl", ""),
	} {
		if _, err := unnestColumnQuery(pg, src); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
}
