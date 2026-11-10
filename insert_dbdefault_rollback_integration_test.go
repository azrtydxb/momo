// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// TestDBDefaultDoNothingRollsBackAndSkipsCleanly runs on PostgreSQL: a bulk
// insert whose conflict action skips a row fails and leaves no row behind (also
// inside a caller's transaction, which survives), while a single skipped Model
// is just 0 rows affected.
func TestDBDefaultDoNothingRollsBackAndSkipsCleanly(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, name dialect.Name) {
		if name != dialect.Postgres {
			t.Skip("gen_random_uuid() defaults are PostgreSQL")
		}
		ctx := context.Background()
		for _, stmt := range []string{
			"DROP TABLE IF EXISTS dbd_uuid",
			"CREATE TABLE dbd_uuid (id uuid primary key default gen_random_uuid(), name text not null unique, created_at timestamptz not null default now())",
			"INSERT INTO dbd_uuid (name) VALUES ('taken')",
		} {
			if _, err := db.Driver().Exec(ctx, stmt, nil); err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
		}
		nameCol := momo.NewColumn[string]("dbd_uuid", "name", momo.FlagUnique)
		present := func(q momo.Querier, n string) bool {
			ok, err := momo.Select[dbdUUID](q).Where(nameCol.Eq(n)).Exists(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return ok
		}

		// One Model that conflicts: 0 rows, no error, model untouched.
		one := dbdUUID{Name: "taken"}
		res, err := momo.Insert[dbdUUID](db).Model(&one).OnConflict(nameCol).DoNothing().Exec(ctx)
		if err != nil || res.RowsAffected != 0 || one.ID != "" {
			t.Fatalf("single skipped Model: res %+v err %v model %+v", res, err, one)
		}

		// Several Models, one conflicts: the insert fails and "fresh" is not stored.
		ms := []dbdUUID{{Name: "fresh"}, {Name: "taken"}}
		_, err = momo.Insert[dbdUUID](db).Models(ms).OnConflict(nameCol).DoNothing().Exec(ctx)
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("err = %v, want ErrUnsupported", err)
		}
		if present(db, "fresh") || ms[0].ID != "" {
			t.Fatalf("the rolled-back row is still there or the model changed: %+v", ms)
		}

		// Inside a caller's transaction only the savepoint rolls back.
		err = db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if _, err := momo.Insert[dbdUUID](tx).Model(&dbdUUID{Name: "kept"}).Exec(ctx); err != nil {
				return err
			}
			more := []dbdUUID{{Name: "fresh2"}, {Name: "taken"}}
			if _, err := momo.Insert[dbdUUID](tx).Models(more).OnConflict(nameCol).DoNothing().Exec(ctx); !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("in Tx: err = %v, want ErrUnsupported", err)
			}
			if present(tx, "fresh2") || !present(tx, "kept") {
				t.Errorf("in Tx: fresh2 must be absent and kept present")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if !present(db, "kept") || present(db, "fresh2") {
			t.Fatal("after commit: kept must be stored and fresh2 absent")
		}
	})
}
