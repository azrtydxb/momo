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

func TestSelectGolden(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	sq := momo.New(fakedriver.New(dialect.SQLite))
	my := momo.New(fakedriver.New(dialect.MySQL))

	cases := []struct {
		name string
		q    func() (momo.Compiled, error)
		want string
	}{
		{"where and order with nulls", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](pg).Where(DocTitle.Eq("a")).OrderBy(DocTitle.Asc().NullsLast()).SQL()
		}, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE ("documents"."title" = $1) AND ("documents"."deleted_at" IS NULL) ORDER BY "documents"."title" ASC NULLS LAST`},
		{"limit and offset postgres", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](pg).Limit(10).Offset(20).SQL()
		}, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE "documents"."deleted_at" IS NULL LIMIT $1 OFFSET $2`},
		{"offset without limit on sqlite", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](sq).Offset(5).SQL()
		}, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE "documents"."deleted_at" IS NULL LIMIT -1 OFFSET ?`},
		{"self join with alias", func() (momo.Compiled, error) {
			p := momo.TableRef{Name: "documents", Alias: "parent"}
			return momo.Select[unitDocument](pg).Join(p, DocTenantID.As("documents").EqExpr(DocTenantID.As("parent"))).SQL()
		}, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" INNER JOIN "documents" AS "parent" ON "documents"."tenant_id" = "parent"."tenant_id" WHERE "documents"."deleted_at" IS NULL`},
	}
	for _, c := range cases {
		got, err := c.q()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got.SQL != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got.SQL, c.want)
		}
	}

	if _, err := momo.Select[unitDocument](my).OrderBy(DocTitle.Asc().NullsFirst()).SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("NULLS FIRST on MySQL = %v, want ErrUnsupported", err)
	}
	if _, err := momo.Select[unitDocument](sq).ForUpdate().SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("FOR UPDATE on SQLite = %v, want ErrUnsupported", err)
	}
}

// Catches: OrderByPredicate rendering the predicate unparenthesised, losing the
// direction, or binding its arguments out of order with the other arguments.
func TestOrderByPredicate(t *testing.T) {
	cases := []struct {
		name string
		d    dialect.Name
		want string
	}{
		{"postgres", dialect.Postgres, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE ("documents"."tenant_id" = $1) AND ("documents"."deleted_at" IS NULL) ORDER BY ("documents"."title" IS NULL) ASC, ("documents"."title" = $2) DESC, "documents"."id" ASC`},
		{"sqlite", dialect.SQLite, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE ("documents"."tenant_id" = ?) AND ("documents"."deleted_at" IS NULL) ORDER BY ("documents"."title" IS NULL) ASC, ("documents"."title" = ?) DESC, "documents"."id" ASC`},
		{"mysql", dialect.MySQL, "SELECT `documents`.`id`, `documents`.`tenant_id`, `documents`.`title`, `documents`.`published`, `documents`.`deleted_at` FROM `documents` WHERE (`documents`.`tenant_id` = ?) AND (`documents`.`deleted_at` IS NULL) ORDER BY (`documents`.`title` IS NULL) ASC, (`documents`.`title` = ?) DESC, `documents`.`id` ASC"},
	}
	for _, c := range cases {
		db := momo.New(fakedriver.New(c.d))
		got, err := momo.Select[unitDocument](db).
			Where(DocTenantID.Eq(7)).
			OrderBy(momo.OrderByPredicate(DocTitle.IsNull()).Asc(), momo.OrderByPredicate(DocTitle.Eq("x")).Desc(), DocID.Asc()).
			SQL()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got.SQL != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got.SQL, c.want)
		}
		if len(got.Args) != 2 || got.Args[0] != int64(7) || got.Args[1] != "x" {
			t.Errorf("%s: args = %v, want [7 x]", c.name, got.Args)
		}
	}

	// Bare OrderByPredicate is Orderable and ascends; NULLS placement chains.
	pg := momo.New(fakedriver.New(dialect.Postgres))
	got, err := momo.Select[unitDocument](pg).OrderBy(momo.OrderByPredicate(DocTitle.IsNull())).SQL()
	if err != nil || !strings.HasSuffix(got.SQL, `ORDER BY ("documents"."title" IS NULL) ASC`) {
		t.Errorf("bare: %v, %v", got.SQL, err)
	}
	got, err = momo.Select[unitDocument](pg).OrderBy(momo.OrderByPredicate(DocTitle.IsNull()).Desc().NullsLast()).SQL()
	if err != nil || !strings.HasSuffix(got.SQL, `ORDER BY ("documents"."title" IS NULL) DESC NULLS LAST`) {
		t.Errorf("nulls: %v, %v", got.SQL, err)
	}
}

// Catches: After over a predicate ORDER BY key building the cursor
// "(pred) > $n". A NULL-able predicate then skips or repeats rows, so the
// build must refuse it with ErrValidation, whatever the predicate is.
func TestKeysetRejectsPredicateKey(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		db := momo.New(fakedriver.New(d))
		for name, o := range map[string]momo.Order{
			"asc":  momo.OrderByPredicate(DocTitle.IsNull()).Asc(),
			"desc": momo.OrderByPredicate(DocTitle.Eq("x")).Desc(),
		} {
			_, err := momo.Select[unitDocument](db).OrderBy(o).After(true, int64(1)).SQL()
			if !errors.Is(err, momo.ErrValidation) {
				t.Errorf("%s %s: After over a predicate key = %v, want ErrValidation", d, name, err)
			}
		}
		// A column key still builds.
		if _, err := momo.Select[unitDocument](db).OrderBy(DocTitle.Asc()).After("x", int64(1)).SQL(); err != nil {
			t.Errorf("%s: After over a column key: %v", d, err)
		}
	}
}

// Catches: a predicate in a union's outer ORDER BY reaching the database, a
// nil predicate panicking at build time, and NULLS FIRST/LAST on a predicate
// rendered on MySQL, which has no such clause.
func TestOrderByPredicateLimits(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	docs := momo.TableRef{Name: "documents"}
	m := momo.Select[momo.Row1[string]](pg).From(docs).Columns(DocTitle)
	_, err := momo.SelectUnion[momo.Row1[string]](pg, true, m, m).OrderBy(momo.OrderByPredicate(DocTitle.IsNull()).Asc()).SQL()
	if !errors.Is(err, momo.ErrValidation) {
		t.Errorf("predicate in a union outer ORDER BY = %v, want ErrValidation", err)
	}

	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		db := momo.New(fakedriver.New(d))
		_, err := momo.Select[unitDocument](db).OrderBy(momo.OrderByPredicate(nil).Asc()).SQL()
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: nil predicate = %v, want ErrValidation", d, err)
		}
	}

	my := momo.New(fakedriver.New(dialect.MySQL))
	for name, o := range map[string]momo.Order{
		"nulls first": momo.OrderByPredicate(DocTitle.IsNull()).Asc().NullsFirst(),
		"nulls last":  momo.OrderByPredicate(DocTitle.IsNull()).Desc().NullsLast(),
	} {
		if _, err := momo.Select[unitDocument](my).OrderBy(o).SQL(); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("mysql %s on a predicate = %v, want ErrUnsupported", name, err)
		}
	}
}
