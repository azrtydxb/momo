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

// Catches: a locked member sent to PostgreSQL, which rejects FOR UPDATE inside
// a set operation at run time, instead of ErrUnsupported at build time; and an
// outer ORDER BY on a qualified column or an expression, which the database
// rejects at run time, instead of ErrValidation at build time.
func TestSelectUnionRejectsWhatTheDatabaseRejects(t *testing.T) {
	docs := momo.TableRef{Name: "documents"}
	pg := momo.New(fakedriver.New(dialect.Postgres))
	m := momo.Select[momo.Row1[string]](pg).From(docs).Columns(DocTitle)
	if _, err := momo.SelectUnion[momo.Row1[string]](pg, true, m.ForUpdate(), m).SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("locked member on PostgreSQL = %v, want ErrUnsupported", err)
	}
	if _, err := momo.SelectUnion[momo.Row1[string]](pg, true, m, momo.Union(true, m.ForShare(), m)).SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("locked member of a nested union on PostgreSQL = %v, want ErrUnsupported", err)
	}
	if _, err := momo.SelectUnion[momo.Row1[string]](pg, true, m, m).OrderBy(momo.ColumnOf[string]("", "title").Asc()).SQL(); err != nil {
		t.Errorf("unqualified outer ORDER BY: %v", err)
	}
	for name, o := range map[string]momo.Orderable{
		"qualified column": DocTitle.Asc(),
		"aliased column":   momo.ColumnOf[string]("u", "title").Desc(),
		"expression":       momo.Asc(momo.TypedSQL[string](`lower(title)`)),
	} {
		_, err := momo.SelectUnion[momo.Row1[string]](pg, true, m, m).OrderBy(o).SQL()
		if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "ColumnOf") {
			t.Errorf("outer ORDER BY on %s = %v, want ErrValidation naming ColumnOf", name, err)
		}
	}
}

// Catches: a member with its own WITH clause rendered bare, which leaves the
// WITH to bind to the whole union (or fails on SQLite), instead of being grouped.
func TestUnionMemberWithClauseGrouping(t *testing.T) {
	docs := momo.TableRef{Name: "documents"}
	for _, c := range []struct {
		d    dialect.Name
		want string
	}{
		{dialect.Postgres, `(WITH "c" AS (SELECT "documents"."title" FROM "documents") SELECT "c"."title" FROM "c") UNION ALL SELECT "documents"."title" FROM "documents"`},
		{dialect.MySQL, "(WITH `c` AS (SELECT `documents`.`title` FROM `documents`) SELECT `c`.`title` FROM `c`) UNION ALL SELECT `documents`.`title` FROM `documents`"},
		{dialect.SQLite, `SELECT * FROM (WITH "c" AS (SELECT "documents"."title" FROM "documents") SELECT "c"."title" FROM "c") UNION ALL SELECT "documents"."title" FROM "documents"`},
	} {
		db := momo.New(fakedriver.New(c.d))
		plain := momo.Select[momo.Row1[string]](db).From(docs).Columns(DocTitle)
		withC := momo.Select[momo.Row1[string]](db).With("c", plain).From(momo.CTE("c")).Columns(momo.ColumnOf[string]("c", "title"))
		got, err := momo.SelectUnion[momo.Row1[string]](db, true, withC, plain).SQL()
		if err != nil {
			t.Fatalf("%s: %v", c.d, err)
		}
		if got.SQL != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.d, got.SQL, c.want)
		}
	}
}

// Catches: a nested Union used as a member losing its grouping (or its own
// UNION versus UNION ALL), and bound arguments or PostgreSQL $n numbers going
// out of order through the nested union.
func TestSelectUnionNestedMember(t *testing.T) {
	docs := momo.TableRef{Name: "documents"}
	const (
		pgSel = `SELECT "documents"."tenant_id", "documents"."title" FROM "documents" WHERE "documents"."tenant_id" = `
		myAll = "SELECT `documents`.`tenant_id`, `documents`.`title` FROM `documents` WHERE `documents`.`tenant_id` = "
	)
	for _, c := range []struct {
		d    dialect.Name
		want string
	}{
		{dialect.Postgres, pgSel + `$1 UNION ((` + pgSel + `$2 LIMIT $3) UNION ALL ` + pgSel + `$4)`},
		{dialect.MySQL, myAll + "? UNION ((" + myAll + "? LIMIT ?) UNION ALL " + myAll + "?)"},
		{dialect.SQLite, pgSel + `? UNION SELECT * FROM (SELECT * FROM (` + pgSel + `? LIMIT ?) UNION ALL ` + pgSel + `?)`},
	} {
		db := momo.New(fakedriver.New(c.d))
		byTenant := func(id int64) momo.SelectQuery[momo.Row2[int64, string]] {
			return momo.Select[momo.Row2[int64, string]](db).From(docs).Columns(DocTenantID, DocTitle).Where(DocTenantID.Eq(id))
		}
		got, err := momo.SelectUnion[momo.Row2[int64, string]](db, false,
			byTenant(1), momo.Union(true, byTenant(2).Limit(1), byTenant(3))).SQL()
		if err != nil {
			t.Fatalf("%s: %v", c.d, err)
		}
		if got.SQL != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.d, got.SQL, c.want)
		}
		if len(got.Args) != 4 || got.Args[0] != int64(1) || got.Args[1] != int64(2) || got.Args[2] != int64(1) || got.Args[3] != int64(3) {
			t.Errorf("%s: args = %v, want [1 2 1 3]", c.d, got.Args)
		}
	}
}
