// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysqldriver_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/schema"
)

type keyRow struct {
	ID   int64
	Name string
}

func (r *keyRow) MomoMeta() *momo.Meta[keyRow] {
	return &momo.Meta[keyRow]{
		Table:    "keys_t",
		PK:       []int{0},
		Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		Columns: []momo.ColumnMeta{
			{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity},
			{Name: "name"},
		},
		Ptr: func(r *keyRow, i int) any {
			return [...]any{&r.ID, &r.Name}[i]
		},
		Value: func(r *keyRow, i int) any {
			return [...]any{r.ID, r.Name}[i]
		},
	}
}

func TestMySQLGeneratedKeys(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			ctx := context.Background()
			db := s.open(t)
			mustExecMySQL(t, db, "CREATE TABLE keys_t (id bigint PRIMARY KEY AUTO_INCREMENT, name varchar(40) NOT NULL)")

			// Single row: LAST_INSERT_ID is written back to the model.
			one := keyRow{Name: "one"}
			if _, err := momo.Insert[keyRow](db).Model(&one).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			if one.ID == 0 {
				t.Fatal("generated id was not written back to the model")
			}

			// The same inside a transaction.
			two := keyRow{Name: "two"}
			err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
				_, err := momo.Insert[keyRow](tx).Model(&two).Exec(ctx)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if two.ID == 0 || two.ID == one.ID {
				t.Fatalf("transaction insert id = %d (first id %d)", two.ID, one.ID)
			}

			// A multi-row insert that asks for no keys is a plain bulk insert.
			many := []keyRow{{Name: "a"}, {Name: "b"}, {Name: "c"}}
			if _, err := momo.Insert[keyRow](db).Models(many).Exec(ctx); err != nil {
				t.Fatalf("bulk insert without keys: %v", err)
			}

			// Asking for the generated keys of a multi-row insert: MariaDB
			// renders INSERT ... RETURNING, MySQL has no way to return them.
			more := []keyRow{{Name: "d"}, {Name: "e"}}
			rows, err := momo.Insert[keyRow](db).Models(more).ReturningAll().Rows(ctx)
			if db.Dialect().Flavor == dialect.FlavorMariaDB {
				if err != nil {
					t.Fatalf("MariaDB RETURNING: %v", err)
				}
				var got []keyRow
				for rows.Next() {
					var r keyRow
					if err := rows.ScanModel(&r); err != nil {
						t.Fatal(err)
					}
					got = append(got, r)
				}
				if err := rows.Close(); err != nil || len(got) != 2 || got[0].ID == 0 || got[1].ID == got[0].ID {
					t.Fatalf("MariaDB returned keys = %+v, %v", got, err)
				}
			} else if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("MySQL RETURNING: err = %v, want ErrUnsupported", err)
			}

			// Multi-row insert with explicit primary keys is allowed and keeps the keys.
			explicit := []keyRow{{ID: 100, Name: "x"}, {ID: 101, Name: "y"}, {ID: 102, Name: "z"}}
			if _, err := momo.Insert[keyRow](db).Models(explicit).Exec(ctx); err != nil {
				t.Fatalf("explicit keys: %v", err)
			}
			all, err := momo.Select[keyRow](db).All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			kept := 0
			for _, r := range all {
				if r.ID >= 100 && r.ID <= 102 {
					kept++
				}
			}
			if kept != 3 {
				t.Fatalf("explicit keys were not kept: rows=%v", all)
			}
		})
	}
}

func TestMySQLUpsert(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			ctx := context.Background()
			db := s.open(t)
			mustExecMySQL(t, db, "CREATE TABLE keys_t (id bigint PRIMARY KEY AUTO_INCREMENT, name varchar(40) NOT NULL)")
			row := keyRow{ID: 7, Name: "first"}
			if _, err := momo.Upsert[keyRow](db).Model(&row).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			row.Name = "second"
			if _, err := momo.Upsert[keyRow](db).Model(&row).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			got, err := momo.Select[keyRow](db).All(ctx)
			if err != nil || len(got) != 1 || got[0].Name != "second" || got[0].ID != 7 {
				t.Fatalf("upsert result = %+v, err = %v", got, err)
			}
		})
	}
}

// Catches: an upsert that targets a column other than the primary key running
// without a schema to prove the table has no other unique key.
func TestMySQLUpsertNonPrimaryTargetNeedsSchema(t *testing.T) {
	db := servers[0].open(t)
	nameCol := momo.NewColumn[string]("keys_t", "name", 0)
	_, err := momo.Insert[keyRow](db).Model(&keyRow{Name: "x"}).OnConflict(nameCol).DoNothing().Exec(t.Context())
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

type counterRow struct {
	Name string
	Hits int64
}

func (r *counterRow) MomoMeta() *momo.Meta[counterRow] {
	return &momo.Meta[counterRow]{
		Table:    "counters",
		Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		Columns: []momo.ColumnMeta{{Name: "name"}, {Name: "hits"}},
		Ptr: func(r *counterRow, i int) any {
			return [...]any{&r.Name, &r.Hits}[i]
		},
		Value: func(r *counterRow, i int) any {
			return [...]any{r.Name, r.Hits}[i]
		},
		TableSchema: &schema.Table{
			Name:    "counters",
			Columns: []schema.Column{{Name: "name", Type: "varchar(40)"}, {Name: "hits", Type: "bigint"}},
			Uniques: []schema.Unique{{Name: "counters_name", Columns: []string{"name"}}},
		},
	}
}

// TestMySQLUpsertTargetsAndRowsAffected covers an upsert on the sole unique
// key (not the primary key), DoNothing, and the RowsAffected values that
// clientFoundRows produces: 1 for an insert, 2 for a changed row, 1 for an
// existing row set to its current values.
func TestMySQLUpsertTargetsAndRowsAffected(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			ctx := context.Background()
			db := s.open(t)
			mustExecMySQL(t, db, "CREATE TABLE counters (name varchar(40) NOT NULL UNIQUE, hits bigint NOT NULL)")
			name := momo.NewColumn[string]("counters", "name", 0)
			hits := momo.NewColumn[int64]("counters", "hits", 0)
			upsert := func(n int64) momo.Result {
				t.Helper()
				r, err := momo.Insert[counterRow](db).Model(&counterRow{Name: "a", Hits: n}).
					OnConflict(name).DoUpdate(hits.SetExcluded()).Exec(ctx)
				if err != nil {
					t.Fatal(err)
				}
				return r
			}
			if r := upsert(1); r.RowsAffected != 1 {
				t.Errorf("insert RowsAffected = %d, want 1", r.RowsAffected)
			}
			if r := upsert(2); r.RowsAffected != 2 {
				t.Errorf("changed row RowsAffected = %d, want 2", r.RowsAffected)
			}
			if r := upsert(2); r.RowsAffected != 1 {
				t.Errorf("unchanged row RowsAffected = %d, want 1 (clientFoundRows)", r.RowsAffected)
			}
			if _, err := momo.Insert[counterRow](db).Model(&counterRow{Name: "a", Hits: 99}).OnConflict(name).DoNothing().Exec(ctx); err != nil {
				t.Fatal(err)
			}
			got, err := momo.Select[counterRow](db).All(ctx)
			if err != nil || len(got) != 1 || got[0].Hits != 2 {
				t.Fatalf("after DoNothing: %+v, %v, want hits 2", got, err)
			}
		})
	}
}

// Catches: the generated-key insert bypassing WithTxSetup and WithImplicitTx,
// which would run a tenant-scoped insert outside its setup.
func TestMySQLGeneratedKeyInsertRunsTxSetup(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			ctx := context.Background()
			setups := 0
			db := s.open(t, momo.WithImplicitTx(), momo.WithTxSetup(func(context.Context, *momo.Tx) error {
				setups++
				return nil
			}))
			mustExecMySQL(t, db, "CREATE TABLE keys_t (id bigint PRIMARY KEY AUTO_INCREMENT, name varchar(40) NOT NULL)")
			setups = 0
			row := keyRow{Name: "one"}
			if _, err := momo.Insert[keyRow](db).Model(&row).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			if setups != 1 || row.ID == 0 {
				t.Fatalf("setups = %d, id = %d, want one setup and a written-back id", setups, row.ID)
			}
		})
	}
}

// Catches: a pinned connection, which cannot run WithTxSetup, still executing a
// generated-key insert (fail open).
func TestMySQLGeneratedKeyInsertFailsClosedOnPinnedConn(t *testing.T) {
	ctx := context.Background()
	db := testopen.MySQL84(t, momo.WithTxSetup(func(context.Context, *momo.Tx) error { return nil }))
	mustExecMySQL(t, db, "CREATE TABLE keys_t (id bigint PRIMARY KEY AUTO_INCREMENT, name varchar(40) NOT NULL)")
	pc, err := db.Driver().(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Release() }()
	row := keyRow{Name: "one"}
	_, err = momo.Insert[keyRow](db.OnConn(pc)).Model(&row).Exec(ctx)
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	if row.ID != 0 {
		t.Fatalf("a failed insert wrote back id %d", row.ID)
	}
}
