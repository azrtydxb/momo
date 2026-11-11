// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Catches: UnnestColumn accepted as the first FROM item, which renders
// FROM LATERAL unnest(...) and only fails at the server.
func TestUnnestColumnAsFirstFromIsRejected(t *testing.T) {
	flags := momo.ColumnOf[[]string]("q2", "flags")
	src := momo.UnnestColumn(flags).As("fl", "flag")
	_, err := momo.Select[anKT](typedDB(dialect.Postgres)).From(src).
		Columns(momo.ColumnOf[int64]("fl", "id"), src.Column()).SQL()
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

// Catches: the element column not usable in WHERE, and LeftJoin with an ON
// condition rendering without LATERAL or without its ON.
func TestUnnestColumnWhereAndLeftJoinGolden(t *testing.T) {
	pg := typedDB(dialect.Postgres)
	flags := momo.ColumnOf[[]string]("q2", "flags")
	src := momo.UnnestColumn(flags).As("fl", "flag")
	owners := momo.TableRef{Name: "owners", Alias: "q2"}
	id := momo.ColumnOf[int64]("q2", "id")

	got, err := momo.Select[anKT](pg).From(owners).Columns(id, src.Column()).
		CrossJoin(src).Where(src.Column().Eq("x")).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "q2"."id", "fl"."flag" FROM "owners" AS "q2" CROSS JOIN LATERAL unnest("q2"."flags") AS "fl" ("flag") WHERE "fl"."flag" = $1`
	if got.SQL != want || len(got.Args) != 1 || got.Args[0] != "x" {
		t.Fatalf("where:\n got %s %#v\nwant %s [x]", got.SQL, got.Args, want)
	}

	got, err = momo.Select[anKT](pg).From(owners).Columns(id, src.Column()).
		LeftJoin(src, momo.SQL("true")).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want = `SELECT "q2"."id", "fl"."flag" FROM "owners" AS "q2" LEFT JOIN LATERAL unnest("q2"."flags") AS "fl" ("flag") ON true`
	if got.SQL != want {
		t.Fatalf("left join:\n got %s\nwant %s", got.SQL, want)
	}
}
