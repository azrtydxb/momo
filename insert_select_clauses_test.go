// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

var leaseOwners = momo.TableRef{Name: "owners"}

var ownerName = momo.NewColumn[string]("owners", "name", 0)

// groupedLease is a grouped source: one row per holder with the largest name,
// kept when the holder has more than one row.
func groupedLease(q momo.Querier) momo.InsertQuery[Lease] {
	src := momo.SelectExprs(leaseHolder, momo.Func[string]("max", leaseName), momo.TypedSQL[time.Time]("now()"), momo.Lit(true)).
		From(leaseTable).
		GroupBy(leaseHolder).
		Having(momo.CountAll().Gt(1)).
		OrderBy(leaseHolder.Asc())
	return momo.Insert[Lease](q).FromSelect(src, leaseName, leaseHolder, leaseUntil, leaseLive)
}

// Catches: GROUP BY, HAVING and ORDER BY missing from an ExprSelect, written
// in the wrong order, or numbered wrongly across the statement.
func TestExprSelectGroupHavingOrderGolden(t *testing.T) {
	for d, want := range map[dialect.Name]string{
		dialect.Postgres: `INSERT INTO "jobs_lock" ("name", "holder", "until", "live") SELECT "jobs_lock"."holder", max("jobs_lock"."name"), now(), TRUE FROM "jobs_lock" GROUP BY "jobs_lock"."holder" HAVING COUNT(*) > $1 ORDER BY "jobs_lock"."holder" ASC`,
		dialect.SQLite:   `INSERT INTO "jobs_lock" ("name", "holder", "until", "live") SELECT "jobs_lock"."holder", max("jobs_lock"."name"), now(), 1 FROM "jobs_lock" GROUP BY "jobs_lock"."holder" HAVING COUNT(*) > ? ORDER BY "jobs_lock"."holder" ASC`,
	} {
		got, err := compileFor(t, d, groupedLease)
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		if got.SQL != want || len(got.Args) != 1 {
			t.Errorf("%s:\n got %s %v\nwant %s", d, got.SQL, got.Args, want)
		}
	}
}

// Catches: joins missing from an ExprSelect or rendered after WHERE, and the
// joined table's name unknown to the scope.
func TestExprSelectJoinsGolden(t *testing.T) {
	build := func(q momo.Querier) momo.InsertQuery[Lease] {
		src := momo.SelectExprs(leaseName, ownerName).
			From(leaseTable).
			Join(leaseOwners, leaseHolder.EqExpr(ownerName)).
			LeftJoin(momo.TableRef{Name: "extra"}, leaseName.EqExpr(ownerName)).
			Where(leaseLive.Eq(true))
		return momo.Insert[Lease](q).FromSelect(src, leaseName, leaseHolder)
	}
	got, err := compileFor(t, dialect.Postgres, build)
	if err != nil {
		t.Fatal(err)
	}
	want := `INSERT INTO "jobs_lock" ("name", "holder") SELECT "jobs_lock"."name", "owners"."name" FROM "jobs_lock" INNER JOIN "owners" ON "jobs_lock"."holder" = "owners"."name" LEFT JOIN "extra" ON "jobs_lock"."name" = "owners"."name" WHERE "jobs_lock"."live" = $1`
	if got.SQL != want {
		t.Errorf("got %s\nwant %s", got.SQL, want)
	}
}

// Catches: a clause of an ExprSelect numbering its bound arguments out of SQL
// order, or dropping one: ON, WHERE, HAVING and ORDER BY each bind one value.
func TestExprSelectBoundArgsInClauseOrder(t *testing.T) {
	build := func(q momo.Querier) momo.InsertQuery[Lease] {
		src := momo.SelectExprs(leaseName).
			From(leaseTable).
			Join(leaseOwners, ownerName.Eq("o")).
			Where(leaseLive.Eq(true)).
			GroupBy(leaseName).
			Having(momo.CountAll().Gt(1)).
			OrderBy(momo.Func[string]("coalesce", leaseHolder, momo.Value("z")).Asc())
		return momo.Insert[Lease](q).FromSelect(src, leaseName)
	}
	got, err := compileFor(t, dialect.Postgres, build)
	if err != nil {
		t.Fatal(err)
	}
	want := `INSERT INTO "jobs_lock" ("name") SELECT "jobs_lock"."name" FROM "jobs_lock" INNER JOIN "owners" ON "owners"."name" = $1 WHERE "jobs_lock"."live" = $2 GROUP BY "jobs_lock"."name" HAVING COUNT(*) > $3 ORDER BY coalesce("jobs_lock"."holder", $4) ASC`
	if got.SQL != want {
		t.Errorf("got %s\nwant %s", got.SQL, want)
	}
	wantArgs := []any{"o", true, int64(1), "z"}
	if len(got.Args) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", got.Args, wantArgs)
	}
	for i := range wantArgs {
		if got.Args[i] != wantArgs[i] {
			t.Errorf("arg %d = %v (%T), want %v (%T)", i+1, got.Args[i], got.Args[i], wantArgs[i], wantArgs[i])
		}
	}
}

// Catches: GroupBy or a join on a FROM-less select reaching the database, and
// an As(...) alias accepted in GROUP BY.
func TestExprSelectClauseValidation(t *testing.T) {
	cases := map[string]momo.ExprSelect{
		"group by without from": momo.SelectExprs(momo.Value(1)).GroupBy(leaseHolder),
		"join without from":     momo.SelectExprs(momo.Value(1)).Join(leaseOwners),
		"aliased group by":      momo.SelectExprs(leaseHolder).From(leaseTable).GroupBy(momo.As[string](leaseHolder, "h")),
	}
	for name, src := range cases {
		_, err := compileFor(t, dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).FromSelect(src, leaseName)
		})
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
}
