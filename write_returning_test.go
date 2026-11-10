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

func TestWriteOneRefusesBulkInsert(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	rows := []suite.Document{{TenantID: 1, Title: "a"}, {TenantID: 1, Title: "b"}}
	_, err := momo.Insert[suite.Document](db).Models(rows).One(context.Background())
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("bulk insert One err = %v, want ErrValidation", err)
	}
	if n := len(drv.Calls()); n != 0 {
		t.Fatalf("the refused bulk insert ran %d statements", n)
	}
}

func TestWriteOneRunsHooks(t *testing.T) {
	var updates, loads int
	boom := errors.New("boom")
	var loadErr error
	setHooks(t, momo.HookSet[hookedDoc]{
		BeforeUpdate: func(context.Context, *hookedDoc) error { updates++; return nil },
		AfterLoad:    func(context.Context, *hookedDoc) error { loads++; return loadErr },
	})
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows(hookedRow())
	db := momo.New(drv)
	ctx := context.Background()
	m := hookedDoc{ID: 1, Title: "hook"}
	got, err := momo.Update[hookedDoc](db).Model(&m).One(ctx)
	if err != nil || got.Title != "hook" {
		t.Fatalf("One = %+v, %v", got, err)
	}
	if updates != 1 || loads != 1 {
		t.Fatalf("BeforeUpdate ran %d times, AfterLoad %d; want 1 and 1", updates, loads)
	}
	// An AfterLoad error fails One, unlike no row at all.
	loadErr = boom
	if _, err := momo.Update[hookedDoc](db).Model(&m).One(ctx); !errors.Is(err, boom) {
		t.Fatalf("AfterLoad error = %v, want boom", err)
	}
}

func TestWriteOneKeepsTheFromGuard(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	src := momo.NewColumn[int64]("src", "id", 0)
	_, err := momo.Update[suite.Document](pg).Set(suite.DocTitle.Set("x")).
		From(momo.TableRef{Name: "src"}).Where(src.Eq(1)).One(context.Background())
	if !errors.Is(err, momo.ErrUnfilteredMutation) {
		t.Fatalf("Update From One err = %v, want ErrUnfilteredMutation", err)
	}
	_, err = momo.ForceDelete[suite.Document](pg).
		Using(momo.TableRef{Name: "src"}).Where(src.Eq(1)).One(context.Background())
	if !errors.Is(err, momo.ErrUnfilteredMutation) {
		t.Fatalf("Delete Using One err = %v, want ErrUnfilteredMutation", err)
	}
}
