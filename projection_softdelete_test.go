// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/examples/crud-zero-null/models/task"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// A projection reads FROM a table whose generated TableRef names the
// soft-delete column, so the live-row filter comes from the table, not from
// the projection's own metadata (which has none).
func TestProjectionSoftDeleteFilter(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	title := momo.NewColumn[string]("docs", "title", 0)
	ts := momo.TableRef{Name: "docs", SoftDelete: "deleted_at"}
	unix := momo.TableRef{Name: "docs", SoftDelete: "deleted_unix", SoftDeleteKind: momo.SoftDeleteUnix}
	flag := momo.TableRef{Name: "docs", SoftDelete: "deleted", SoftDeleteKind: momo.SoftDeleteBool}
	plain := momo.TableRef{Name: "docs"}
	sel := func(from momo.TableRef) momo.SelectQuery[unitTally] {
		return momo.Select[unitTally](pg).From(from).Columns(title)
	}
	cases := []struct {
		name string
		q    momo.SelectQuery[unitTally]
		want string
	}{
		{"timestamp", sel(ts), `SELECT "docs"."title" FROM "docs" WHERE "docs"."deleted_at" IS NULL`},
		{"unix", sel(unix), `SELECT "docs"."title" FROM "docs" WHERE "docs"."deleted_unix" IS NULL`},
		{"bool", sel(flag), `SELECT "docs"."title" FROM "docs" WHERE "docs"."deleted" = FALSE`},
		{"with deleted", sel(ts).WithDeleted(), `SELECT "docs"."title" FROM "docs"`},
		{"only deleted", sel(ts).OnlyDeleted(), `SELECT "docs"."title" FROM "docs" WHERE "docs"."deleted_at" IS NOT NULL`},
		{"only deleted bool", sel(flag).OnlyDeleted(), `SELECT "docs"."title" FROM "docs" WHERE "docs"."deleted" = TRUE`},
		{"with where", sel(ts).Where(title.Eq("x")), `SELECT "docs"."title" FROM "docs" WHERE ("docs"."title" = $1) AND ("docs"."deleted_at" IS NULL)`},
		{"aliased", momo.Select[unitTally](pg).From(momo.TableRef{Name: "docs", Alias: "d", SoftDelete: "deleted_at"}).Columns(momo.NewColumn[string]("d", "title", 0)),
			`SELECT "d"."title" FROM "docs" AS "d" WHERE "d"."deleted_at" IS NULL`},
		// Unchanged: no soft-delete column, a join, a subquery source.
		{"no soft delete", sel(plain), `SELECT "docs"."title" FROM "docs"`},
		// The FROM table is filtered whatever is joined; the joined table is not.
		{"join", sel(ts).LeftJoin(momo.TableRef{Name: "other", SoftDelete: "gone"}, title.EqExpr(momo.NewColumn[string]("other", "title", 0))),
			`SELECT "docs"."title" FROM "docs" LEFT JOIN "other" ON "docs"."title" = "other"."title" WHERE "docs"."deleted_at" IS NULL`},
		{"join plain from", sel(plain).Join(momo.TableRef{Name: "other", SoftDelete: "gone"}, title.EqExpr(momo.NewColumn[string]("other", "title", 0))),
			`SELECT "docs"."title" FROM "docs" INNER JOIN "other" ON "docs"."title" = "other"."title"`},
		{"as alias", momo.Select[unitTally](pg).From(ts.As("d")).Columns(momo.NewColumn[string]("d", "title", 0)),
			`SELECT "d"."title" FROM "docs" AS "d" WHERE "d"."deleted_at" IS NULL`},
		{"generated table", momo.Select[unitTally](pg).From(task.Table).Columns(task.Title),
			`SELECT "tasks"."title" FROM "tasks" WHERE "tasks"."deleted_at" IS NULL`},
		{"in subquery", momo.Select[unitTally](pg).From(plain).Columns(title).Where(title.InQuery(sel(ts))),
			`SELECT "docs"."title" FROM "docs" WHERE "docs"."title" IN (SELECT "docs"."title" FROM "docs" WHERE "docs"."deleted_at" IS NULL)`},
		{"subquery", momo.Select[unitTally](pg).From(momo.FromSubquery(sel(ts), "s")).Columns(momo.NewColumn[string]("s", "title", 0)),
			`SELECT "s"."title" FROM (SELECT "docs"."title" FROM "docs" WHERE "docs"."deleted_at" IS NULL) AS "s"`},
	}
	for _, c := range cases {
		got, err := c.q.SQL()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !strings.EqualFold(got.SQL, c.want) {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got.SQL, c.want)
		}
	}
}

// Count and Exists wrap the projection query, so the filter applies there too.
func TestProjectionSoftDeleteCountAndExists(t *testing.T) {
	d := fakedriver.New(dialect.Postgres)
	pg := momo.New(d)
	title := momo.NewColumn[string]("docs", "title", 0)
	q := momo.Select[unitTally](pg).From(momo.TableRef{Name: "docs", SoftDelete: "deleted_at"}).Columns(title)
	if _, err := q.Count(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Exists(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := d.Calls()
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(calls))
	}
	for _, c := range calls {
		if !strings.Contains(c.SQL, `WHERE "docs"."deleted_at" IS NULL`) {
			t.Errorf("no soft-delete filter: %s", c.SQL)
		}
	}
}
