// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysql_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
)

// TestMySQLHistoryTableMustBeBaseTable: a view named momo_migrations would
// pass CREATE TABLE IF NOT EXISTS and be written through; the engine must
// refuse it before it runs anything.
func TestMySQLHistoryTableMustBeBaseTable(t *testing.T) {
	ctx := context.Background()
	db := testopen.MySQL84(t)
	mustExec(t, db, "CREATE VIEW momo_migrations AS SELECT 1 AS version")
	dir := t.TempDir()
	writeFile(t, dir, "20260101000000_t.up.sql", "CREATE TABLE should_not_exist (id int);\n")
	eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
	_, err := eng.Apply(ctx, migrate.ApplyOptions{})
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("apply over a view named momo_migrations: err = %v, want momo.ErrValidation", err)
	}
	if _, err := momo.Exec(ctx, db, "SELECT 1 FROM should_not_exist LIMIT 1"); err == nil {
		t.Fatal("a migration ran although the history table is a view")
	}
}
