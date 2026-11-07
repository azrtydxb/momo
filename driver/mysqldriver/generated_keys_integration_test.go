// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysqldriver_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
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

			// Multi-row insert that needs generated keys: rejected before any statement runs.
			many := []keyRow{{Name: "a"}, {Name: "b"}, {Name: "c"}}
			if _, err := momo.Insert[keyRow](db).Models(many).Exec(ctx); !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("multi-row generated keys: err = %v, want momo.ErrUnsupported", err)
			}

			// Multi-row insert with explicit primary keys is allowed and keeps the keys.
			explicit := []keyRow{{ID: 100, Name: "x"}, {ID: 101, Name: "y"}, {ID: 102, Name: "z"}}
			if _, err := momo.Insert[keyRow](db).Models(explicit).Exec(ctx); err != nil {
				t.Fatalf("explicit keys: %v", err)
			}
			rows, err := momo.Select[keyRow](db).All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			kept := 0
			for _, r := range rows {
				if r.ID >= 100 && r.ID <= 102 {
					kept++
				}
			}
			if kept != 3 {
				t.Fatalf("explicit keys were not kept: rows=%v", rows)
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
