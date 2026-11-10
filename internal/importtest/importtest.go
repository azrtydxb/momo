// SPDX-License-Identifier: Apache-2.0

// Package importtest is the shared end-to-end check for history importers.
// It is a regular package so the importers' tests can share it; it imports
// testing and is used only by test files.
package importtest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/importer/history"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

// Input is one imported history. The live database already holds the source
// tool's history table (created by the test); Exclude names the source tool's
// tables so drift does not report them as unmanaged.
type Input struct {
	Tool    string
	Source  history.Source
	Exclude []string
}

// CheckImport converts the source, builds the snapshots on an empty scratch
// database, applies the converted SQL to live, records the history rows, and
// asserts that every converted version reads as applied and that drift is
// clean.
func CheckImport(t *testing.T, live *momo.DB, in Input) {
	t.Helper()
	ctx := context.Background()
	conv, _, err := history.Convert(in.Source)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	scratch := testopen.Postgres(t)
	snaps, err := history.Baseline(ctx, scratch, mpg.New(), conv)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}
	dir := filepath.Join(t.TempDir(), "out")
	if err := history.WriteImport(dir, conv, snaps); err != nil {
		t.Fatalf("write import: %v", err)
	}
	fsys := os.DirFS(dir)
	sum, err := migrate.ComputeSum(fsys)
	if err != nil {
		t.Fatal(err)
	}
	pn, ok := live.Driver().(momo.Pinner)
	if !ok {
		t.Fatal("driver does not implement momo.Pinner")
	}
	conn, err := pn.Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Release() }()
	for _, cv := range conv {
		stmts, err := migrate.SplitStatements(cv.Up, "postgres")
		if err != nil {
			t.Fatalf("split %s: %v", cv.Version, err)
		}
		for _, stmt := range stmts {
			if _, err := conn.Exec(ctx, stmt, nil); err != nil {
				t.Fatalf("apply %s: %v", cv.Version, err)
			}
		}
	}
	if err := migrate.EnsureHistory(ctx, conn, mpg.New()); err != nil {
		t.Fatal(err)
	}
	rows, err := history.HistoryRows(conv, sum, in.Tool, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if _, err := conn.Exec(ctx,
			`INSERT INTO momo_migrations (version, name, checksum, status, statements_applied, error, applied_at, execution_ms, apply_id, kind) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			[]any{row.Version, row.Name, row.Checksum, row.Status, row.StatementsApplied, row.Error, row.AppliedAt.Format(time.RFC3339Nano), row.ExecutionMS, row.ApplyID, row.Kind}); err != nil {
			t.Fatalf("history row %s: %v", row.Version, err)
		}
	}
	e := migratetest.NewEngine(t, mpg.New(), live.Driver(), fsys, migrate.WithConfig(migrate.Config{
		Exclude: append([]string{"momo_migrations", "momo_migration_lock"}, in.Exclude...),
	}))
	states, err := e.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, cv := range conv {
		found := false
		for _, st := range states {
			if st.Version == cv.Version {
				found = true
				if st.State != "applied" {
					t.Fatalf("%s: status %q, want applied", cv.Version, st.State)
				}
			}
		}
		if !found {
			t.Fatalf("%s missing from status", cv.Version)
		}
	}
	rep, err := e.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() {
		t.Fatalf("imported history must drift to zero: %+v", rep)
	}
}
