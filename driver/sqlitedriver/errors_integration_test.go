// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlitedriver

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/mattn/go-sqlite3"
)

func TestErrorClassificationSQLite(t *testing.T) {
	db, d, err := Open(testdb.SQLitePath(t), Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "CREATE TABLE u (id integer primary key, email text unique not null)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO u (email) VALUES ('a@x')"); err != nil {
		t.Fatal(err)
	}
	_, err = d.Exec(ctx, "INSERT INTO u (email) VALUES ('a@x')", nil)
	if !errors.Is(err, momo.ErrUniqueViolation) {
		t.Fatalf("unique: %v", err)
	}
	var me *momo.Error
	if !errors.As(err, &me) || me.Table != "u" || me.Column != "email" {
		t.Fatalf("table/column not parsed: %+v", me)
	}
	var se sqlite3.Error
	if !errors.As(err, &se) || se.ExtendedCode != sqlite3.ErrConstraintUnique {
		t.Fatalf("cause not reachable: %v", err)
	}
	_, err = d.Exec(ctx, "INSERT INTO u (email) VALUES (NULL)", nil)
	if !errors.Is(err, momo.ErrNotNullViolation) {
		t.Fatalf("not null: %v", err)
	}
}
