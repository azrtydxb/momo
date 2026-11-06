// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// createSoftVariants creates docs_unix and docs_bool, and the partial unique
// indexes that let a soft-deleted key be re-created for every marker kind
// (the timestamp variant is the documents table).
func createSoftVariants(t *testing.T, db *momo.DB) {
	t.Helper()
	ctx := context.Background()
	id := "integer primary key"
	if suite.CapsOf(db).Dialect == dialect.Postgres {
		id = "bigint generated always as identity primary key"
	}
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS docs_unix",
		"DROP TABLE IF EXISTS docs_bool",
		"CREATE TABLE docs_unix (id " + id + ", tenant_id integer not null, title text not null, deleted_unix bigint null)",
		"CREATE TABLE docs_bool (id " + id + ", tenant_id integer not null, title text not null, deleted boolean not null default false)",
		"CREATE UNIQUE INDEX docs_unix_title ON docs_unix (tenant_id, title) WHERE deleted_unix IS NULL",
		"CREATE UNIQUE INDEX docs_bool_title ON docs_bool (tenant_id, title) WHERE deleted = FALSE",
		"CREATE UNIQUE INDEX documents_title_live ON documents (tenant_id, title) WHERE deleted_at IS NULL",
	} {
		if _, err := momo.Exec(ctx, db, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		_, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS docs_unix")
		_, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS docs_bool")
	})
}

func TestSoftDelete(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'keep'), (1, 'gone')"); err != nil {
			t.Fatal(err)
		}
		res, err := momo.SoftDelete[suite.Document](db).Where(suite.DocTitle.Eq("gone")).Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("SoftDelete: %+v %v", res, err)
		}
		// A second SoftDelete must not touch the already-deleted row.
		if res, err := momo.SoftDelete[suite.Document](db).Where(suite.DocTitle.Eq("gone")).Exec(ctx); err != nil || res.RowsAffected != 0 {
			t.Fatalf("repeat SoftDelete: %+v %v", res, err)
		}
		visible, err := momo.Select[suite.Document](db).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		all, err := momo.Select[suite.Document](db).WithDeleted().All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		only, err := momo.Select[suite.Document](db).OnlyDeleted().All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(visible) != 1 || len(all) != 2 || len(only) != 1 || only[0].Title != "gone" || only[0].DeletedAt == nil {
			t.Fatalf("visible=%d all=%d only=%d", len(visible), len(all), len(only))
		}
	})
}

func TestSoftDeleteVariants(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		createSoftVariants(t, db)
		ctx := context.Background()
		for _, ins := range []string{
			"INSERT INTO documents (tenant_id, title) VALUES (5, 'u')",
			"INSERT INTO docs_unix (tenant_id, title) VALUES (5, 'u')",
			"INSERT INTO docs_bool (tenant_id, title) VALUES (5, 'u')",
		} {
			if _, err := momo.Exec(ctx, db, ins); err != nil {
				t.Fatal(err)
			}
		}
		t.Run("timestamp", func(t *testing.T) {
			sdVariantBody[suite.Document](t, db, suite.DocTitle, "documents")
		})
		t.Run("unix", func(t *testing.T) {
			sdVariantBody[sdUnix](t, db, sdUnixTitle, "docs_unix")
		})
		t.Run("bool", func(t *testing.T) {
			sdVariantBody[sdBool](t, db, sdBoolTitle, "docs_bool")
		})
	})
}

var sdBoolTitle = momo.NewColumn[string]("docs_bool", "title", 0)

func sdVariantBody[T any, PT momo.Modeler[T]](t *testing.T, db *momo.DB, title momo.Column[string], table string) {
	t.Helper()
	ctx := context.Background()
	if res, err := momo.SoftDelete[T, PT](db).Where(title.Eq("u")).Exec(ctx); err != nil || res.RowsAffected != 1 {
		t.Fatalf("SoftDelete: %+v %v", res, err)
	}
	// Hidden by default.
	if n, err := momo.Select[T, PT](db).Where(title.Eq("u")).Count(ctx); err != nil || n != 0 {
		t.Fatalf("visible after soft delete = %d, %v, want 0", n, err)
	}
	if n, err := momo.Select[T, PT](db).OnlyDeleted().Where(title.Eq("u")).Count(ctx); err != nil || n != 1 {
		t.Fatalf("OnlyDeleted = %d, %v, want 1", n, err)
	}
	// A unique index over live rows permits re-creating the key.
	if _, err := momo.Exec(ctx, db, "INSERT INTO "+table+" (tenant_id, title) VALUES (5, 'u')"); err != nil {
		t.Fatalf("re-create soft-deleted key: %v", err)
	}
	if n, err := momo.Select[T, PT](db).Where(title.Eq("u")).Count(ctx); err != nil || n != 1 {
		t.Fatalf("visible after re-create = %d, %v, want 1", n, err)
	}
	if n, err := momo.Select[T, PT](db).WithDeleted().Where(title.Eq("u")).Count(ctx); err != nil || n != 2 {
		t.Fatalf("WithDeleted = %d, %v, want 2", n, err)
	}
	// ForceDelete really removes, soft-deleted rows included.
	if res, err := momo.ForceDelete[T, PT](db).Where(title.Eq("u")).Exec(ctx); err != nil || res.RowsAffected != 2 {
		t.Fatalf("ForceDelete: %+v %v, want 2 rows", res, err)
	}
	if n, err := momo.Select[T, PT](db).WithDeleted().Where(title.Eq("u")).Count(ctx); err != nil || n != 0 {
		t.Fatalf("ForceDelete left %d rows, %v", n, err)
	}
}

func TestPlainDeleteRefusedForceDeleteWorks(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'a')"); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Delete[suite.Document](db).Where(suite.DocTitle.Eq("a")).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("plain Delete err = %v, want ErrValidation", err)
		}
		if n, _ := momo.Select[suite.Document](db).Count(ctx); n != 1 {
			t.Fatalf("row must survive a refused delete, count = %d", n)
		}
		if res, err := momo.ForceDelete[suite.Document](db).Where(suite.DocTitle.Eq("a")).Exec(ctx); err != nil || res.RowsAffected != 1 {
			t.Fatalf("ForceDelete: %+v %v", res, err)
		}
		if n, _ := momo.Select[suite.Document](db).WithDeleted().Count(ctx); n != 0 {
			t.Fatalf("ForceDelete left %d rows", n)
		}
	})
}
