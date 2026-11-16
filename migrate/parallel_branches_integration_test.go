// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
)

// The merged directory of two branches that added different tables applies:
// the base, then both branches' migrations in version order. momo.sum is the
// union-merged one, so Apply also checks it.
func TestParallelBranchesApply(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		var dl migrate.Dialect = mpg.New()
		if d == dialect.SQLite {
			dl = msqlite.New()
		}
		merged := pbMergedIndependent(t)
		e, err := migrate.NewEngine(db.Driver(), merged, migrate.WithDialect(dl))
		if err != nil {
			t.Fatal(err)
		}
		rep, err := e.Apply(context.Background(), migrate.ApplyOptions{})
		if err != nil {
			t.Fatalf("apply the merged directory: %v", err)
		}
		if len(rep.Applied) != 3 {
			t.Fatalf("want 3 migrations applied, got %+v", rep.Applied)
		}
		for _, table := range []string{"base", "a", "b"} {
			mustExecDB(t, db, "INSERT INTO "+table+" (id) VALUES (1)")
		}
	})
}

// A same-table fork is refused by Apply and nothing runs.
func TestParallelBranchesSameTableApplyRefused(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		var dl migrate.Dialect = mpg.New()
		if d == dialect.SQLite {
			dl = msqlite.New()
		}
		base := pbBase()
		a := pbBranch(base, pbVA, "add_x", "ALTER TABLE base ADD COLUMN x text;", pbTable("base", "x"))
		b := pbBranch(base, pbVB, "add_y", "ALTER TABLE base ADD COLUMN y text;", pbTable("base", "y"))
		e, err := migrate.NewEngine(db.Driver(), pbMerge(t, a, b), migrate.WithDialect(dl))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Apply(context.Background(), migrate.ApplyOptions{}); !errors.Is(err, migrate.ErrBranchConflict) {
			t.Fatalf("want ErrBranchConflict, got %v", err)
		}
	})
}
