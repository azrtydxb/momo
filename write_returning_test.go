// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

func TestReturningRendersPerDialect(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	c, err := momo.Update[suite.Document](pg).Set(suite.DocTitle.Set("x")).Where(suite.DocID.Eq(1)).
		Returning(suite.DocTitle, suite.DocID).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `UPDATE "documents" SET "title" = $1, "updated_at" = $2 WHERE "documents"."id" = $3 RETURNING "title", "id"`
	if c.SQL != want {
		t.Fatalf("SQL = %s\nwant  %s", c.SQL, want)
	}
	all, err := momo.Insert[suite.Document](pg).Values(suite.DocTenantID.Set(1)).ReturningAll().SQL()
	if err != nil {
		t.Fatal(err)
	}
	if want := `INSERT INTO "documents" ("tenant_id", "created_at", "updated_at") VALUES ($1, $2, $3) RETURNING "id", "tenant_id", "title", "body", "version", "deleted_at", "created_at", "updated_at", "secret"`; all.SQL != want {
		t.Fatalf("SQL = %s\nwant  %s", all.SQL, want)
	}
}

func TestReturningUnsupportedOnMySQL(t *testing.T) {
	my := momo.New(fakedriver.New(dialect.MySQL))
	_, err := momo.Update[suite.Document](my).Set(suite.DocTitle.Set("x")).Where(suite.DocID.Eq(1)).
		Returning(suite.DocID).SQL()
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("update err = %v, want ErrUnsupported", err)
	}
	_, err = momo.Insert[suite.Document](my).Values(suite.DocTenantID.Set(1)).ReturningAll().SQL()
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("insert err = %v, want ErrUnsupported", err)
	}
}

func TestReturningValidation(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	ctx := context.Background()
	foreign := momo.NewColumn[string]("documents", "not_a_column", 0)
	_, err := momo.Update[suite.Document](pg).Set(suite.DocTitle.Set("x")).Where(suite.DocID.Eq(1)).Returning(foreign).SQL()
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("unknown column err = %v", err)
	}
	_, err = momo.Update[suite.Document](pg).Set(suite.DocTitle.Set("x")).Where(suite.DocID.Eq(1)).Rows(ctx)
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("Rows without Returning err = %v", err)
	}
	doc := suite.Document{ID: 1}
	_, err = momo.Update[suite.Document](pg).Model(&doc).Returning(suite.DocID).Rows(ctx)
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("Rows on versioned Model update err = %v", err)
	}
}

func TestWriteOneUnsupportedOnMySQL(t *testing.T) {
	my := momo.New(fakedriver.New(dialect.MySQL))
	ctx := context.Background()
	if _, err := momo.Insert[suite.Document](my).Values(suite.DocTenantID.Set(1)).One(ctx); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("insert One err = %v, want ErrUnsupported", err)
	}
	if _, err := momo.Update[suite.Document](my).Set(suite.DocTitle.Set("x")).Where(suite.DocID.Eq(1)).One(ctx); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("update One err = %v, want ErrUnsupported", err)
	}
	if _, err := momo.ForceDelete[suite.Document](my).Where(suite.DocID.Eq(1)).One(ctx); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("delete One err = %v, want ErrUnsupported", err)
	}
}
