// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func TestDiffRefusesWhileGoMigrationHasNoSnapshot(t *testing.T) {
	g := migrate.GoMigration{Version: "20260102000000", Name: "fill", Up: func(context.Context, *momo.Tx) error { return nil }}
	e := migratetest.NewEngine(t, mpg.New(), nil, fstest.MapFS{}, migrate.WithGoMigrations(g))
	_, err := e.Diff(context.Background(), &schema.Schema{}, "next", migrate.DiffOptions{})
	if err == nil || !strings.Contains(err.Error(), "20260102000000_fill.snapshot.json") {
		t.Fatalf("want an error naming the missing snapshot, got %v", err)
	}
}

func TestRefuseGoRollback(t *testing.T) {
	rows := []migrate.HistoryRow{{Version: "20260101000000", Name: "a", Kind: "sql"}, {Version: "20260102000000", Name: "fill", Kind: "go"}}
	err := migrate.RefuseGoRollback(rows)
	if !errors.Is(err, migrate.ErrGoMigrationNotReversible) || !strings.Contains(err.Error(), "20260102000000") {
		t.Fatalf("want ErrGoMigrationNotReversible naming the version, got %v", err)
	}
	if err := migrate.RefuseGoRollback(rows[:1]); err != nil {
		t.Fatalf("sql rows are reversible, got %v", err)
	}
}
