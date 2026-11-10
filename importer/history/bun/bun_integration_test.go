// SPDX-License-Identifier: Apache-2.0

//go:build integration

package bun

import (
	"context"
	"errors"
	"os"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/importer/history"
	"github.com/azrtydxb/momo/internal/importtest"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

const createTable = `CREATE TABLE bun_migrations (id BIGSERIAL PRIMARY KEY, name VARCHAR(180), group_id INTEGER NOT NULL DEFAULT 0, migrated_at TIMESTAMPTZ NOT NULL DEFAULT now())`

func pin(t *testing.T, db *momo.DB) momo.PinnedConn {
	t.Helper()
	pn, ok := db.Driver().(momo.Pinner)
	if !ok {
		t.Fatal("driver does not implement momo.Pinner")
	}
	conn, err := pn.Pin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Release() })
	return conn
}

func TestHistoryImporters(t *testing.T) {
	ctx := context.Background()

	t.Run("bun", func(t *testing.T) {
		live := testopen.Postgres(t)
		if _, err := momo.Exec(ctx, live, createTable); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, live, `INSERT INTO bun_migrations (name, group_id) VALUES ('20260101000000', 1)`); err != nil {
			t.Fatal(err)
		}
		src, err := Read(os.DirFS("../../../internal/compat/bunmigfixture/migrations"), pin(t, live), "")
		if err != nil {
			t.Fatal(err)
		}
		if len(src.Applied) != 1 {
			t.Fatalf("bun_migrations not read: applied=%v", src.Applied)
		}
		importtest.CheckImport(t, live, importtest.Input{Tool: "bun", Source: src, Exclude: []string{"bun_migrations", "bun_migration_locks"}})
	})

	t.Run("bun/go-function-refused", func(t *testing.T) {
		live := testopen.Postgres(t)
		if _, err := momo.Exec(ctx, live, createTable); err != nil {
			t.Fatal(err)
		}
		fsys := fstest.MapFS{
			"20260101000000_go_step.go": {Data: []byte("package migrations\n")},
		}
		src, err := Read(fsys, pin(t, live), "")
		if err != nil {
			t.Fatal(err)
		}
		if len(src.Unsupported) != 1 {
			t.Fatalf("Go-function migration not listed as unsupported: %+v", src.Unsupported)
		}
		if _, _, err := history.Convert(src); !errors.Is(err, history.ErrUnsupportedSource) {
			t.Fatalf("Go-function migration must be refused, got %v", err)
		}
	})

	t.Run("bun/pending-refused", func(t *testing.T) {
		live := testopen.Postgres(t)
		if _, err := momo.Exec(ctx, live, createTable); err != nil {
			t.Fatal(err)
		}
		src, err := Read(os.DirFS("../../../internal/compat/bunmigfixture/migrations"), pin(t, live), "")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := history.Convert(src); !errors.Is(err, history.ErrSourcePending) {
			t.Fatalf("a migration bun never applied must be refused, got %v", err)
		}
	})

	t.Run("bun/conflicting-up-files-refused", func(t *testing.T) {
		live := testopen.Postgres(t)
		if _, err := momo.Exec(ctx, live, createTable); err != nil {
			t.Fatal(err)
		}
		fsys := fstest.MapFS{
			"20260101000000_a.up.sql":    {Data: []byte("SELECT 1;")},
			"20260101000000_a.tx.up.sql": {Data: []byte("SELECT 1;")},
		}
		if _, err := Read(fsys, pin(t, live), ""); err == nil {
			t.Fatal(".up.sql and .tx.up.sql for one version must be refused")
		}
	})

	t.Run("bun/custom-table", func(t *testing.T) {
		live := testopen.Postgres(t)
		if _, err := momo.Exec(ctx, live, `CREATE TABLE my_migrations (id BIGSERIAL PRIMARY KEY, name VARCHAR(180), group_id INTEGER NOT NULL DEFAULT 0, migrated_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, live, `INSERT INTO my_migrations (name, group_id) VALUES ('20260101000000', 1)`); err != nil {
			t.Fatal(err)
		}
		src, err := Read(os.DirFS("../../../internal/compat/bunmigfixture/migrations"), pin(t, live), "my_migrations")
		if err != nil {
			t.Fatal(err)
		}
		if len(src.Applied) != 1 {
			t.Fatalf("custom table not read: applied=%v", src.Applied)
		}
	})
}
