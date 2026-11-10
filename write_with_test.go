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

var (
	withSrc      = momo.CTE("src")
	withSrcID    = momo.ColumnOf[int64]("src", "id")
	withSrcTitle = momo.ColumnOf[string]("src", "title")
)

func withDB(d dialect.Name) *momo.DB { return momo.New(fakedriver.New(d)) }

// Catches: the FROM clause missing or placed after WHERE, MySQL rendering the
// PostgreSQL form, and MySQL assigning an unqualified column in the
// multi-table form (ambiguous between the two tables).
func TestUpdateFromGoldenSQL(t *testing.T) {
	build := func(d dialect.Name) momo.UpdateQuery[unitDocument] {
		return momo.Update[unitDocument](withDB(d)).
			Set(DocTitle.SetExpr(withSrcTitle)).
			From(withSrc).
			Where(DocID.EqExpr(withSrcID))
	}
	cases := []struct {
		d    dialect.Name
		want string
	}{
		{dialect.Postgres, `UPDATE "documents" SET "title" = "src"."title" FROM "src" WHERE "documents"."id" = "src"."id"`},
		{dialect.SQLite, `UPDATE "documents" SET "title" = "src"."title" FROM "src" WHERE "documents"."id" = "src"."id"`},
		{dialect.MySQL, "UPDATE `documents`, `src` SET `documents`.`title` = `src`.`title` WHERE `documents`.`id` = `src`.`id`"},
	}
	for _, tc := range cases {
		c, err := build(tc.d).SQL()
		if err != nil || c.SQL != tc.want {
			t.Errorf("%s: got %q %v, want %q", tc.d, c.SQL, err, tc.want)
		}
	}
}

// Catches: a subquery source losing its alias or its bound arguments.
func TestUpdateFromSubquery(t *testing.T) {
	sub := momo.SelectExprs(momo.Value(int64(7)), momo.Value("t"))
	c, err := momo.Update[unitDocument](withDB(dialect.Postgres)).
		Set(DocTitle.SetExpr(momo.ColumnOf[string]("s", "title"))).
		From(momo.FromSubquery(sub, "s")).
		Where(DocID.EqExpr(momo.ColumnOf[int64]("s", "id"))).
		Returning(DocID).SQL()
	want := `UPDATE "documents" SET "title" = "s"."title" FROM (SELECT $1, $2) AS "s" WHERE "documents"."id" = "s"."id" RETURNING "id"`
	if err != nil || c.SQL != want || len(c.Args) != 2 {
		t.Fatalf("got %q %v %v, want %q", c.SQL, c.Args, err, want)
	}
}

// Catches: DELETE ... USING rendered for SQLite, which has none, and MySQL
// missing the table name before FROM.
func TestDeleteUsingGoldenSQL(t *testing.T) {
	build := func(d dialect.Name) momo.DeleteQuery[delNote] {
		return momo.Delete[delNote](withDB(d)).Using(withSrc).Where(delNoteID.EqExpr(withSrcID))
	}
	c, err := build(dialect.Postgres).SQL()
	if want := `DELETE FROM "del_notes" USING "src" WHERE "del_notes"."id" = "src"."id"`; err != nil || c.SQL != want {
		t.Errorf("pg: got %q %v, want %q", c.SQL, err, want)
	}
	c, err = build(dialect.MySQL).SQL()
	if want := "DELETE `del_notes` FROM `del_notes`, `src` WHERE `del_notes`.`id` = `src`.`id`"; err != nil || c.SQL != want {
		t.Errorf("mysql: got %q %v, want %q", c.SQL, err, want)
	}
	if _, err := build(dialect.SQLite).SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("sqlite err = %v, want ErrUnsupported", err)
	}
}

// Catches: the WITH clause dropped from, or placed after, the DML head, and
// the CTE arguments numbered after the statement's own.
func TestDMLWithClause(t *testing.T) {
	body := momo.SelectExprs(momo.Value(int64(1)), momo.Value("t"))
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		c, err := momo.Update[unitDocument](withDB(d)).With("s", body, "id", "title").
			Set(DocTitle.SetExpr(momo.ColumnOf[string]("s", "title"))).
			From(momo.CTE("s")).Where(DocID.EqExpr(momo.ColumnOf[int64]("s", "id")), DocTenantID.Eq(5)).SQL()
		if err != nil || !strings.HasPrefix(c.SQL, "WITH ") || !strings.Contains(c.SQL, "UPDATE") || len(c.Args) != 3 {
			t.Errorf("update %s: %q %v %v", d, c.SQL, c.Args, err)
		}
		c, err = momo.Delete[delNote](withDB(d)).With("s", body, "id", "title").
			Where(delNoteID.Eq(1)).SQL()
		if err != nil || !strings.HasPrefix(c.SQL, "WITH ") || len(c.Args) != 3 {
			t.Errorf("delete %s: %q %v %v", d, c.SQL, c.Args, err)
		}
	}
	rec := momo.Delete[delNote](withDB(dialect.Postgres)).WithRecursive("s", body, "id", "title").Where(delNoteID.Eq(1))
	if c, err := rec.SQL(); err != nil || !strings.HasPrefix(c.SQL, `WITH RECURSIVE "s" ("id", "title") AS (SELECT $1, $2) DELETE FROM`) {
		t.Errorf("recursive: %q %v", c.SQL, err)
	}
	if _, err := momo.Delete[delNote](withDB(dialect.Postgres)).With("s", nil).Where(delNoteID.Eq(1)).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("nil body err = %v, want ErrValidation", err)
	}
}

// Catches: INSERT ... WITH ordering, a MySQL WITH before INSERT reaching the
// server, and the CTE scope not covering the FromSelect select.
func TestInsertWith(t *testing.T) {
	body := momo.SelectExprs(momo.Value(int64(1)), momo.Value("t"))
	build := func(d dialect.Name) momo.InsertQuery[delNote] {
		sel := momo.SelectExprs(momo.ColumnOf[int64]("s", "id"), momo.ColumnOf[string]("s", "title")).From(momo.CTE("s"))
		return momo.Insert[delNote](withDB(d)).With("s", body, "id", "title").FromSelect(sel, delNoteID, delNoteTitle)
	}
	c, err := build(dialect.Postgres).SQL()
	want := `WITH "s" ("id", "title") AS (SELECT $1, $2) INSERT INTO "del_notes" ("id", "title") SELECT "s"."id", "s"."title" FROM "s"`
	if err != nil || c.SQL != want {
		t.Errorf("pg: got %q %v, want %q", c.SQL, err, want)
	}
	if c, err = build(dialect.SQLite).SQL(); err != nil || !strings.HasPrefix(c.SQL, "WITH ") {
		t.Errorf("sqlite: %q %v", c.SQL, err)
	}
	if _, err = build(dialect.MySQL).SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("mysql err = %v, want ErrUnsupported", err)
	}
}

// Catches: a DML statement rendered without parentheses as a CTE body, its
// RETURNING list lost, its arguments mis-numbered, and a non-PostgreSQL
// dialect letting the statement through.
func TestDMLAsCTEBody(t *testing.T) {
	ins := func(q momo.Querier) momo.InsertQuery[delNote] {
		return momo.Insert[delNote](q).Values(delNoteID.Set(9), delNoteTitle.Set("n")).Returning(delNoteID)
	}
	pg := withDB(dialect.Postgres)
	c, err := momo.Delete[delNote](pg).With("x", ins(pg)).Using(momo.CTE("x")).
		Where(delNoteID.EqExpr(momo.ColumnOf[int64]("x", "id")), delNoteTitle.Ne("z")).SQL()
	want := `WITH "x" AS (INSERT INTO "del_notes" ("id", "title") VALUES ($1, $2) RETURNING "id") DELETE FROM "del_notes" USING "x" WHERE ("del_notes"."id" = "x"."id") AND ("del_notes"."title" <> $3)`
	if err != nil || c.SQL != want || len(c.Args) != 3 {
		t.Errorf("insert body: got %q %v %v, want %q", c.SQL, c.Args, err, want)
	}

	// An UPDATE and a DELETE body, read by a SELECT.
	upd := momo.Update[delNote](pg).Set(delNoteTitle.Set("u")).Where(delNoteID.Eq(1)).Returning(delNoteID)
	del := momo.Delete[delNote](pg).Where(delNoteID.Eq(2)).Returning(delNoteID)
	c, err = momo.Select[delNote](pg).With("u", upd).With("d", del).From(momo.CTE("u")).SQL()
	for _, frag := range []string{
		`WITH "u" AS (UPDATE "del_notes" SET "title" = $1 WHERE "del_notes"."id" = $2 RETURNING "id"), "d" AS (DELETE FROM "del_notes" WHERE "del_notes"."id" = $3 RETURNING "id") SELECT `,
	} {
		if err != nil || !strings.HasPrefix(c.SQL, frag) || len(c.Args) != 3 {
			t.Errorf("select over update/delete: %q %v %v", c.SQL, c.Args, err)
		}
	}

	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		db := withDB(d)
		_, err := momo.Select[delNote](db).With("x", ins(db)).From(momo.CTE("x")).SQL()
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: data-modifying CTE err = %v, want ErrUnsupported", d, err)
		}
	}
}

// Catches: a builder mutating its receiver instead of returning a copy.
func TestDMLWithFromAreValues(t *testing.T) {
	db := withDB(dialect.Postgres)
	base := momo.Delete[delNote](db).Where(delNoteID.Eq(1))
	_ = base.Using(withSrc).With("s", momo.SelectExprs(momo.Value(1)))
	c, err := base.SQL()
	if want := `DELETE FROM "del_notes" WHERE "del_notes"."id" = $1`; err != nil || c.SQL != want {
		t.Fatalf("base changed: %q %v", c.SQL, err)
	}
}
