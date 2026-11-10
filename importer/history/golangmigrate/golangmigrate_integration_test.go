// SPDX-License-Identifier: Apache-2.0

//go:build integration

package golangmigrate

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/importer/history"
	"github.com/azrtydxb/momo/internal/importtest"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

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
	fixtures := os.DirFS("../../../internal/compat/gmfixture/migrations")
	ctx := context.Background()

	t.Run("golangmigrate", func(t *testing.T) {
		live := testopen.Postgres(t)
		if _, err := momo.Exec(ctx, live, `CREATE TABLE schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)`); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, live, `INSERT INTO schema_migrations (version, dirty) VALUES (2, false)`); err != nil {
			t.Fatal(err)
		}
		src, err := Read(fixtures, pin(t, live), "schema_migrations")
		if err != nil {
			t.Fatal(err)
		}
		// golang-migrate stores only the current version, 2, and treats
		// every migration up to it as applied.
		if src.Dirty || len(src.Applied) != 2 {
			t.Fatalf("clean state misread: dirty=%v applied=%v", src.Dirty, src.Applied)
		}
		if src.Migrations[0].Down == nil {
			t.Fatal("the down file of version 1 was not read")
		}
		importtest.CheckImport(t, live, importtest.Input{Tool: "golangmigrate", Source: src, Exclude: []string{"schema_migrations"}})
	})

	t.Run("golangmigrate/dirty-refused", func(t *testing.T) {
		live := testopen.Postgres(t)
		if _, err := momo.Exec(ctx, live, `CREATE TABLE schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)`); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, live, `INSERT INTO schema_migrations (version, dirty) VALUES (1, true)`); err != nil {
			t.Fatal(err)
		}
		src, err := Read(fixtures, pin(t, live), "schema_migrations")
		if err != nil {
			t.Fatal(err)
		}
		if !src.Dirty {
			t.Fatal("dirty row not read")
		}
		if _, _, err := history.Convert(src); !errors.Is(err, history.ErrSourceDirty) {
			t.Fatalf("dirty source must be refused, got %v", err)
		}
	})

	t.Run("golangmigrate/pending-refused", func(t *testing.T) {
		live := testopen.Postgres(t)
		if _, err := momo.Exec(ctx, live, `CREATE TABLE schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)`); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, live, `INSERT INTO schema_migrations (version, dirty) VALUES (1, false)`); err != nil {
			t.Fatal(err)
		}
		src, err := Read(fixtures, pin(t, live), "schema_migrations")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := history.Convert(src); !errors.Is(err, history.ErrSourcePending) {
			t.Fatalf("a migration the source never applied must be refused, got %v", err)
		}
	})
}
