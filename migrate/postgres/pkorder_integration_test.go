// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"slices"
	"testing"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/schema"
)

// Catches: the key order being sorted or reset to column order on render or
// inspection, so health_sample (PRIMARY KEY (name, scope, at) over columns
// (at, scope, name, value)) would differ from its model on every baseline.
func TestPostgresPrimaryKeyOrderRoundTrip(t *testing.T) {
	db := testopen.Postgres(t)
	got := roundTrip(t, db, &schema.Schema{
		Dialect: "postgres",
		Tables: []schema.Table{{
			Name: "health_sample",
			Columns: []schema.Column{
				{Name: "at", Type: "timestamp with time zone"},
				{Name: "scope", Type: "text"},
				{Name: "name", Type: "text"},
				{Name: "value", Type: "double precision"},
			},
			PrimaryKey: []string{"name", "scope", "at"},
		}},
	})
	if want := []string{"name", "scope", "at"}; !slices.Equal(got.Tables[0].PrimaryKey, want) {
		t.Fatalf("primary key = %v, want %v", got.Tables[0].PrimaryKey, want)
	}
}
