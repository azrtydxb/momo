// SPDX-License-Identifier: Apache-2.0

//go:build integration

package pgxdriver

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/types"
	"github.com/google/uuid"
)

// bindingTable creates a table with a uuid key and a jsonb column and drops it
// at the end of the test. The driver is built with New and no WithExecMode, so
// the tests below run under pgxdriver's default pgx.QueryExecModeExec.
func bindingTable(t *testing.T) (*momo.DB, string) {
	t.Helper()
	ctx := context.Background()
	db := momo.New(New(openPool(t)))
	table := fmt.Sprintf("bind_probe_%d", time.Now().UnixNano())
	if _, err := momo.Exec(ctx, db, "CREATE TABLE "+table+" (id uuid PRIMARY KEY, n int NOT NULL, doc jsonb)"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS "+table) })
	return db, table
}

var bindingIDs = []string{
	"11111111-1111-4111-8111-111111111111",
	"22222222-2222-4222-8222-222222222222",
	"33333333-3333-4333-8333-333333333333",
}

func seedBindingRows(t *testing.T, db *momo.DB, table string) {
	t.Helper()
	for i, id := range bindingIDs {
		if _, err := momo.Exec(context.Background(), db, "INSERT INTO "+table+" (id, n) VALUES (?::uuid, ?)", id, i); err != nil {
			t.Fatal(err)
		}
	}
}

// TestUUIDSliceBindsToAnyArray covers the Kuvryn Orbit pattern
// "id = ANY($1::uuid[])" under the default exec mode, for the two Go slice
// types an application holds: []uuid.UUID and []types.UUID.
func TestUUIDSliceBindsToAnyArray(t *testing.T) {
	db, table := bindingTable(t)
	ctx := context.Background()
	seedBindingRows(t, db, table)
	query := "SELECT id::text FROM " + table + " WHERE id = ANY(?::uuid[]) ORDER BY id"

	t.Run("google uuid", func(t *testing.T) {
		ids := []uuid.UUID{uuid.MustParse(bindingIDs[0]), uuid.MustParse(bindingIDs[2])}
		got, err := momo.Scalars[string](ctx, db, query, ids)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{bindingIDs[0], bindingIDs[2]}; !slices.Equal(got, want) {
			t.Errorf("[]uuid.UUID = %v, want %v", got, want)
		}
	})

	t.Run("types uuid", func(t *testing.T) {
		var ids []types.UUID
		for _, s := range []string{bindingIDs[1], bindingIDs[2]} {
			u, err := types.ParseUUID(s)
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, u)
		}
		got, err := momo.Scalars[string](ctx, db, query, ids)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{bindingIDs[1], bindingIDs[2]}; !slices.Equal(got, want) {
			t.Errorf("[]types.UUID = %v, want %v", got, want)
		}
	})

	t.Run("empty slice matches nothing", func(t *testing.T) {
		got, err := momo.Scalars[string](ctx, db, query, []uuid.UUID{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("empty []uuid.UUID matched %v", got)
		}
	})
}

// TestByteSliceBindsToJSONB covers a []byte of JSON text written to a jsonb
// column and read back, under the default exec mode.
func TestByteSliceBindsToJSONB(t *testing.T) {
	db, table := bindingTable(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, "INSERT INTO "+table+" (id, n, doc) VALUES (?::uuid, 1, ?)", bindingIDs[0], []byte(`{"a": 1, "b": [true, null]}`)); err != nil {
		t.Fatal(err)
	}
	got, err := momo.Scalars[string](ctx, db, "SELECT doc::text FROM "+table+" WHERE id = ?::uuid", bindingIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{`{"a": 1, "b": [true, null]}`}; !slices.Equal(got, want) {
		t.Errorf("jsonb round trip = %v, want %v", got, want)
	}
	// The stored value is jsonb, not a string holding JSON text.
	typ, err := momo.Scalars[string](ctx, db, "SELECT jsonb_typeof(doc) FROM "+table+" WHERE id = ?::uuid", bindingIDs[0])
	if err != nil || len(typ) != 1 || typ[0] != "object" {
		t.Errorf("jsonb_typeof = %v, %v; want object", typ, err)
	}
}
