// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlite_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
)

// TestSQLiteNoTxForeignKeyCheckFails: a no-transaction migration that leaves an
// orphan row and then runs PRAGMA foreign_key_check must fail, as a
// transactional one does, and say that the earlier statements stay committed.
func TestSQLiteNoTxForeignKeyCheckFails(t *testing.T) {
	db := testopen.SQLite(t)
	ctx := context.Background()
	up := "-- momo:no-transaction\n-- momo:sqlite-foreign-keys-off\n" +
		"CREATE TABLE parent (id integer PRIMARY KEY);\n" +
		"CREATE TABLE child (id integer PRIMARY KEY, parent_id integer REFERENCES parent (id));\n" +
		"INSERT INTO child (id, parent_id) VALUES (1, 99);\n" +
		"PRAGMA foreign_key_check;\n"
	dir := migratetest.FS(map[string][]byte{"20260101000000_orphan.up.sql": []byte(up)})
	eng := migratetest.NewEngine(t, msqlite.New(), db.Driver(), dir)
	_, err := eng.Apply(ctx, migrate.ApplyOptions{})
	if !errors.Is(err, momo.ErrForeignKeyViolation) {
		t.Fatalf("apply: err = %v, want momo.ErrForeignKeyViolation", err)
	}
	if !strings.Contains(err.Error(), "already committed") {
		t.Fatalf("the error must say earlier statements are committed: %v", err)
	}
	states, err := eng.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || string(states[0].State) != "dirty" {
		t.Fatalf("status = %+v, want one dirty version", states)
	}
}
