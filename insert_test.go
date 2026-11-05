// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestInsertModelOmitAndConflict(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		doc := suite.Document{TenantID: 3, Title: "m", Secret: "s"}
		if _, err := momo.Insert[suite.Document](db).Model(&doc).Omit(suite.DocBody).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		res, err := momo.Insert[suite.Document](db).
			Values(suite.DocTenantID.Set(3), suite.DocTitle.Set("m")).
			OnConflict(suite.DocID).DoNothing().Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("insert on conflict do nothing with new row: %+v %v", res, err)
		}
		got, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("m")).Count(ctx)
		if err != nil || got != 2 {
			t.Fatalf("count = %d, %v", got, err)
		}
	})
}

// insertSQL compiles q against a PostgreSQL dialect-only DB (no connection).
func insertSQL(t *testing.T, q momo.InsertQuery[suite.Document]) momo.Compiled {
	t.Helper()
	c, err := q.SQL()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func unitDB() *momo.DB { return momo.New(fakedriver.New(dialect.Postgres)) }

func TestInsertModelColumnRules(t *testing.T) {
	doc := suite.Document{ID: 9, TenantID: 1, Title: "t", Secret: "x"}
	c := insertSQL(t, momo.Insert[suite.Document](unitDB()).Model(&doc).Omit(suite.DocBody, suite.DocSecret))
	want := `INSERT INTO "documents" ("tenant_id", "title", "version", "deleted_at", "created_at", "updated_at") VALUES ($1, $2, $3, $4, $5, $6)`
	if c.SQL != want {
		t.Fatalf("SQL = %s\nwant  %s", c.SQL, want)
	}
	if len(c.Args) != 6 || c.Args[0] != int64(1) || c.Args[1] != "t" {
		t.Fatalf("args = %v", c.Args)
	}
	for _, s := range []string{`"id"`, `"body"`, `"secret"`} {
		if strings.Contains(c.SQL, s) {
			t.Fatalf("%s must not be written: %s", s, c.SQL)
		}
	}
}

func TestInsertBulkAndReturning(t *testing.T) {
	ms := []suite.Document{{TenantID: 1, Title: "a"}, {TenantID: 2, Title: "b"}}
	c := insertSQL(t, momo.Insert[suite.Document](unitDB()).
		Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt, suite.DocVersion, suite.DocCreatedAt, suite.DocUpdatedAt).
		Models(ms).Returning(suite.DocID))
	want := `INSERT INTO "documents" ("tenant_id", "title") VALUES ($1, $2), ($3, $4) RETURNING "id"`
	if c.SQL != want {
		t.Fatalf("SQL = %s\nwant  %s", c.SQL, want)
	}
}

func TestInsertDoUpdateExcluded(t *testing.T) {
	c := insertSQL(t, momo.Insert[suite.Document](unitDB()).
		Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("x")).
		OnConflict(suite.DocID).DoUpdate(suite.DocTitle.SetExcluded(), suite.DocTenantID.Set(5)))
	want := `INSERT INTO "documents" ("tenant_id", "title", "created_at", "updated_at") VALUES ($1, $2, $3, $4) ON CONFLICT ("id") DO UPDATE SET "title" = EXCLUDED."title", "tenant_id" = $5`
	if c.SQL != want {
		t.Fatalf("SQL = %s\nwant  %s", c.SQL, want)
	}
}

func TestInsertValidation(t *testing.T) {
	db := unitDB()
	foreign := momo.NewColumn[string]("documents", "not_a_column", 0)
	doc := suite.Document{}
	cases := map[string]momo.InsertQuery[suite.Document]{
		"unknown values column": momo.Insert[suite.Document](db).Values(foreign.Set("x")),
		"unknown omit column":   momo.Insert[suite.Document](db).Model(&doc).Omit(foreign),
		"unknown conflict col":  momo.Insert[suite.Document](db).Model(&doc).OnConflict(foreign).DoNothing(),
		"unknown returning col": momo.Insert[suite.Document](db).Model(&doc).Returning(foreign),
		"identity in values":    momo.Insert[suite.Document](db).Values(suite.DocID.Set(1)),
		"duplicate column":      momo.Insert[suite.Document](db).Values(suite.DocTitle.Set("a"), suite.DocTitle.Set("b")),
		"model plus values":     momo.Insert[suite.Document](db).Model(&doc).Values(suite.DocTitle.Set("a")),
		"nothing to insert":     momo.Insert[suite.Document](db),
		"empty models":          momo.Insert[suite.Document](db).Models(nil),
		"update without target": momo.Insert[suite.Document](db).Model(&doc).OnConflict().DoUpdate(suite.DocTitle.SetExcluded()),
		"update without assigns": momo.Insert[suite.Document](db).Model(&doc).
			OnConflict(suite.DocID).DoUpdate(),
		"excluded in values": momo.Insert[suite.Document](db).Values(suite.DocTitle.SetExcluded()),
	}
	for name, q := range cases {
		if _, err := q.SQL(); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
	// Create-only values are accepted through InsertAssignment.
	co := momo.NewCreateOnlyColumn[string]("documents", "title", momo.FlagCreateOnly)
	if _, err := momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1), co.Set("x")).SQL(); err != nil {
		t.Fatalf("create-only Set rejected: %v", err)
	}
}

func TestInsertValuesNullAndExprAreNotBound(t *testing.T) {
	c := insertSQL(t, momo.Insert[suite.Document](unitDB()).
		Values(suite.DocTenantID.Set(1), suite.DocBody.SetNull(), suite.DocTitle.SetExpr(momo.Value("e"))))
	want := `INSERT INTO "documents" ("tenant_id", "body", "title", "created_at", "updated_at") VALUES ($1, NULL, $2, $3, $4)`
	if c.SQL != want {
		t.Fatalf("SQL = %s\nwant  %s", c.SQL, want)
	}
}

func TestInsertBulkOverParamLimit(t *testing.T) {
	ms := make([]suite.Document, 40000)
	_, err := momo.Insert[suite.Document](unitDB()).Models(ms).SQL()
	if !errors.Is(err, momo.ErrLimitExceeded) {
		t.Fatalf("err = %v, want ErrLimitExceeded (chunking is a separate task)", err)
	}
}

func TestInsertReturningPopulatesIdentity(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		doc := suite.Document{TenantID: 7, Title: "r", Secret: "s"}
		rows, err := momo.Insert[suite.Document](db).Model(&doc).Omit(suite.DocBody).
			Returning(suite.DocID, suite.DocTitle).Rows(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !rows.Next() {
			t.Fatal("no RETURNING row")
		}
		var got suite.Document
		if err := rows.ScanModel(&got); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		if got.ID == 0 || got.Title != "r" || got.TenantID != 0 {
			t.Fatalf("returned %+v", got)
		}
	})
}
