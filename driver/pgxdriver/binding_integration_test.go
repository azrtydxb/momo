// SPDX-License-Identifier: Apache-2.0

//go:build integration

package pgxdriver

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/types"
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

// googleUUID has the shape of github.com/google/uuid.UUID: a named [16]byte
// with a canonical String method. The module does not depend on google/uuid
// directly, so the test stands in for it with the same shape.
type googleUUID [16]byte

func (u googleUUID) String() string { return types.UUID(u).String() }

// TestUUIDSliceBindsToAnyArray covers the Kuvryn Orbit pattern
// "id = ANY($1::uuid[])" under the default exec mode, for the slice types an
// application holds: []types.UUID and a slice of a google/uuid-shaped type.
func TestUUIDSliceBindsToAnyArray(t *testing.T) {
	db, table := bindingTable(t)
	ctx := context.Background()
	seedBindingRows(t, db, table)
	query := "SELECT id::text FROM " + table + " WHERE id = ANY(?::uuid[]) ORDER BY id"
	uncast := "SELECT id::text FROM " + table + " WHERE id = ANY(?) ORDER BY id"

	parse := func(s string) types.UUID {
		t.Helper()
		u, err := types.ParseUUID(s)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}

	t.Run("google uuid shape", func(t *testing.T) {
		ids := []googleUUID{googleUUID(parse(bindingIDs[0])), googleUUID(parse(bindingIDs[2]))}
		got, err := momo.Scalars[string](ctx, db, query, ids)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{bindingIDs[0], bindingIDs[2]}; !slices.Equal(got, want) {
			t.Errorf("[]uuid.UUID = %v, want %v", got, want)
		}
	})

	t.Run("types uuid", func(t *testing.T) {
		ids := []types.UUID{parse(bindingIDs[1]), parse(bindingIDs[2])}
		got, err := momo.Scalars[string](ctx, db, query, ids)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{bindingIDs[1], bindingIDs[2]}; !slices.Equal(got, want) {
			t.Errorf("[]types.UUID = %v, want %v", got, want)
		}
		// The server infers uuid[] with no cast, too.
		got, err = momo.Scalars[string](ctx, db, uncast, ids)
		if err != nil || !slices.Equal(got, []string{bindingIDs[1], bindingIDs[2]}) {
			t.Errorf("[]types.UUID without a cast = %v, %v", got, err)
		}
	})

	t.Run("empty and nil slices match nothing", func(t *testing.T) {
		for name, ids := range map[string][]types.UUID{"empty": {}, "nil": nil} {
			got, err := momo.Scalars[string](ctx, db, query, ids)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if len(got) != 0 {
				t.Errorf("%s []types.UUID matched %v", name, got)
			}
		}
	})
}

// TestJSONBBinding records how a jsonb column is written under the default
// exec mode. The mode sends no parameter types, so a Go []byte is bytea text
// and is rejected by jsonb; json.RawMessage and string carry the JSON text.
func TestJSONBBinding(t *testing.T) {
	db, table := bindingTable(t)
	ctx := context.Background()
	const doc = `{"a": 1, "b": [true, null]}`

	for name, v := range map[string]any{
		"json.RawMessage": json.RawMessage(doc),
		"string":          doc,
	} {
		if _, err := momo.Exec(ctx, db, "DELETE FROM "+table); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, db, "INSERT INTO "+table+" (id, n, doc) VALUES (?::uuid, 1, ?)", bindingIDs[0], v); err != nil {
			t.Errorf("%s into jsonb: %v", name, err)
			continue
		}
		got, err := momo.Scalars[string](ctx, db, "SELECT doc::text FROM "+table+" WHERE id = ?::uuid", bindingIDs[0])
		if err != nil || !slices.Equal(got, []string{doc}) {
			t.Errorf("%s round trip = %v, %v; want %s", name, got, err, doc)
		}
	}

	// []byte is bytea: it needs a decode on the SQL side to reach jsonb.
	if _, err := momo.Exec(ctx, db, "DELETE FROM "+table); err != nil {
		t.Fatal(err)
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO "+table+" (id, n, doc) VALUES (?::uuid, 1, convert_from(?::bytea, 'UTF8')::jsonb)", bindingIDs[1], []byte(doc)); err != nil {
		t.Fatalf("[]byte through convert_from: %v", err)
	}
	got, err := momo.Scalars[string](ctx, db, "SELECT doc::text FROM "+table+" WHERE id = ?::uuid", bindingIDs[1])
	if err != nil || !slices.Equal(got, []string{doc}) {
		t.Errorf("[]byte round trip = %v, %v; want %s", got, err, doc)
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO "+table+" (id, n, doc) VALUES (?::uuid, 2, ?)", bindingIDs[2], []byte(doc)); err == nil {
		t.Errorf("a bare []byte into jsonb succeeded; the docs say it is rejected")
	}
}
