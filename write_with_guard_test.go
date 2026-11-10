// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

// Catches: the WITH clause dropped from, or placed after, the DML head, and
// the CTE arguments numbered after the statement's own (PostgreSQL $n across
// CTE, SET, FROM and WHERE; positional order on SQLite and MySQL).
func TestDMLWithClause(t *testing.T) {
	body := momo.SelectExprs(momo.Value(int64(1)), momo.Value("t"))
	wantArgs := []any{int64(1), "t", int64(5)}
	upd := []struct {
		d    dialect.Name
		want string
	}{
		{dialect.Postgres, `WITH "s" ("id", "title") AS (SELECT $1, $2) UPDATE "documents" SET "title" = "s"."title" FROM "s" WHERE ("documents"."id" = "s"."id") AND ("documents"."tenant_id" = $3)`},
		{dialect.SQLite, `WITH "s" ("id", "title") AS (SELECT ?, ?) UPDATE "documents" SET "title" = "s"."title" FROM "s" WHERE ("documents"."id" = "s"."id") AND ("documents"."tenant_id" = ?)`},
		{dialect.MySQL, "WITH `s` (`id`, `title`) AS (SELECT ?, ?) UPDATE `documents`, `s` SET `documents`.`title` = `s`.`title` WHERE (`documents`.`id` = `s`.`id`) AND (`documents`.`tenant_id` = ?)"},
	}
	for _, tc := range upd {
		c, err := momo.Update[unitDocument](withDB(tc.d)).With("s", body, "id", "title").
			Set(DocTitle.SetExpr(momo.ColumnOf[string]("s", "title"))).
			From(momo.CTE("s")).Where(DocID.EqExpr(momo.ColumnOf[int64]("s", "id")), DocTenantID.Eq(5)).SQL()
		if err != nil || c.SQL != tc.want || !reflect.DeepEqual(c.Args, wantArgs) {
			t.Errorf("update %s: got %q %v %v, want %q %v", tc.d, c.SQL, c.Args, err, tc.want, wantArgs)
		}
	}
	del := []struct {
		d    dialect.Name
		want string
	}{
		{dialect.Postgres, `WITH "s" ("id", "title") AS (SELECT $1, $2) DELETE FROM "del_notes" WHERE "del_notes"."id" = $3`},
		{dialect.SQLite, `WITH "s" ("id", "title") AS (SELECT ?, ?) DELETE FROM "del_notes" WHERE "del_notes"."id" = ?`},
		{dialect.MySQL, "WITH `s` (`id`, `title`) AS (SELECT ?, ?) DELETE FROM `del_notes` WHERE `del_notes`.`id` = ?"},
	}
	for _, tc := range del {
		c, err := momo.Delete[delNote](withDB(tc.d)).With("s", body, "id", "title").Where(delNoteID.Eq(1)).SQL()
		if err != nil || c.SQL != tc.want || !reflect.DeepEqual(c.Args, []any{int64(1), "t", int64(1)}) {
			t.Errorf("delete %s: got %q %v %v, want %q", tc.d, c.SQL, c.Args, err, tc.want)
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

// Catches: WITH before UPDATE or DELETE reaching a MariaDB server, where it is
// not verified.
func TestDMLWithMariaDBRefused(t *testing.T) {
	body := momo.SelectExprs(momo.Value(int64(1)))
	maria := momo.New(fakedriver.NewDialect(dialect.MySQLFlavor(dialect.FlavorMariaDB)))
	if _, err := momo.Update[unitDocument](maria).With("s", body).Set(DocTitle.Set("x")).Where(DocID.Eq(1)).SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("mariadb update err = %v, want ErrUnsupported", err)
	}
	if _, err := momo.Delete[delNote](maria).With("s", body).Where(delNoteID.Eq(1)).SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("mariadb delete err = %v, want ErrUnsupported", err)
	}
}

// Catches: BulkUpdate silently dropping a With or From.
func TestBulkUpdateRejectsWithAndFrom(t *testing.T) {
	db := withDB(dialect.Postgres)
	rows := []any{int64(2), "new"}
	if _, err := bulkQ(db, rows).With("s", momo.SelectExprs(momo.Value(1))).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("With err = %v, want ErrValidation", err)
	}
	if _, err := bulkQ(db, rows).From(withSrc).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("From err = %v, want ErrValidation", err)
	}
}

// Catches: the unfiltered-mutation guard passing a statement whose only
// condition is on the second source (a cross join that touches every target
// row), or refusing it when AllRows confirms.
func TestFromUsingGuard(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.MySQL} {
		del := momo.Delete[delNote](withDB(d)).Using(withSrc)
		if _, err := del.SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Errorf("%s using, no Where: err = %v", d, err)
		}
		if _, err := del.Where(withSrcTitle.Eq("x")).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Errorf("%s using, Where on source only: err = %v", d, err)
		}
		if _, err := del.Where(withSrcTitle.Eq("x")).AllRows().SQL(); err != nil {
			t.Errorf("%s using, AllRows: %v", d, err)
		}
		if _, err := del.Where(withSrcTitle.Eq("x"), delNoteID.EqExpr(withSrcID)).SQL(); err != nil {
			t.Errorf("%s using, joined: %v", d, err)
		}
	}
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		upd := momo.Update[unitDocument](withDB(d)).Set(DocTitle.SetExpr(withSrcTitle)).From(withSrc)
		if _, err := upd.SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Errorf("%s from, no Where: err = %v", d, err)
		}
		if _, err := upd.Where(withSrcID.Eq(1)).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Errorf("%s from, Where on source only: err = %v", d, err)
		}
		if _, err := upd.Where(withSrcID.Eq(1)).AllRows().SQL(); err != nil {
			t.Errorf("%s from, AllRows: %v", d, err)
		}
		if _, err := upd.Where(DocID.EqExpr(withSrcID)).SQL(); err != nil {
			t.Errorf("%s from, joined: %v", d, err)
		}
	}
}

// Catches: the version column unqualified in the MySQL multi-table form
// (ambiguous between the two tables) and the version bump lost with From.
func TestVersionedUpdateFrom(t *testing.T) {
	doc := &suite.Document{ID: 1, Title: "x"}
	my, err := momo.Update[suite.Document](withDB(dialect.MySQL)).Model(doc).From(withSrc).SQL()
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{"UPDATE `documents`, `src` SET `documents`.`tenant_id` = ?", "`documents`.`version` = `documents`.`version` + 1"} {
		if !strings.Contains(my.SQL, frag) {
			t.Errorf("mysql SQL lacks %q:\n%s", frag, my.SQL)
		}
	}
	pg, err := momo.Update[suite.Document](withDB(dialect.Postgres)).Model(doc).From(withSrc).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if frag := `"version" = "documents"."version" + 1 FROM "src" WHERE`; !strings.Contains(pg.SQL, frag) {
		t.Errorf("pg SQL lacks %q:\n%s", frag, pg.SQL)
	}
}

// Catches: RETURNING silently dropped with a second source: SQLite renders it,
// MySQL and MariaDB refuse it.
func TestFromUsingReturning(t *testing.T) {
	q := func(d dialect.Name) momo.UpdateQuery[unitDocument] {
		return momo.Update[unitDocument](withDB(d)).Set(DocTitle.SetExpr(withSrcTitle)).From(withSrc).Where(DocID.EqExpr(withSrcID)).Returning(DocID)
	}
	c, err := q(dialect.SQLite).SQL()
	if want := `UPDATE "documents" SET "title" = "src"."title" FROM "src" WHERE "documents"."id" = "src"."id" RETURNING "id"`; err != nil || c.SQL != want {
		t.Errorf("sqlite: got %q %v, want %q", c.SQL, err, want)
	}
	if _, err := q(dialect.MySQL).SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("mysql update err = %v, want ErrUnsupported", err)
	}
	for _, d := range []dialect.Name{dialect.MySQL, dialect.SQLite} {
		_, err := momo.Delete[delNote](withDB(d)).Using(withSrc).Where(delNoteID.EqExpr(withSrcID)).Returning(delNoteID).SQL()
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s delete using returning err = %v, want ErrUnsupported", d, err)
		}
	}
}

// Catches: an INSERT used as a CTE body skipping the bind-parameter limit
// that Exec enforces, so the server rejects it instead.
func TestInsertCTEBodyParamLimit(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithMaxParams(4))
	ins := momo.Insert[delNote](db).Models([]delNote{{ID: 1, Title: "a"}, {ID: 2, Title: "b"}, {ID: 3, Title: "c"}}).Returning(delNoteID)
	_, err := momo.Select[delNote](db).With("x", ins).From(momo.CTE("x")).SQL()
	if !errors.Is(err, momo.ErrLimitExceeded) {
		t.Errorf("err = %v, want ErrLimitExceeded", err)
	}
}
