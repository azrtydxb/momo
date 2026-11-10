// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/vector"
)

// TestPostgresNilVectorPointerInsert: a nil *vector.Vector inserts as NULL on
// PostgreSQL. It used to panic in the DialectValue method the pointer inherits.
func TestPostgresNilVectorPointerInsert(t *testing.T) {
	db := testopen.Postgres(t)
	mustExecPG(t, db, "CREATE TABLE ndocs (id bigint PRIMARY KEY, emb vector(3))")
	first := vector.Vector{1, 2, 3}
	rows := []sqvNullDoc{{ID: 1, Emb: &first}, {ID: 2}}
	if _, err := momo.Insert[sqvNullDoc](db).Models(rows).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	col := momo.NewNullVectorColumn[vector.Vector]("ndocs", "emb", 3, 0)
	nulls, err := momo.Select[sqvNullDoc](db).Where(col.IsNull()).All(t.Context())
	if err != nil || len(nulls) != 1 || nulls[0].ID != 2 || nulls[0].Emb != nil {
		t.Fatalf("NULL rows = %v, %v, want only id 2", nulls, err)
	}
	set, err := momo.Select[sqvNullDoc](db).Where(col.IsNotNull()).All(t.Context())
	if err != nil || len(set) != 1 || set[0].ID != 1 || set[0].Emb == nil || !slices.Equal(*set[0].Emb, first) {
		t.Fatalf("set rows = %v, %v, want only id 1 reading back", set, err)
	}
}
