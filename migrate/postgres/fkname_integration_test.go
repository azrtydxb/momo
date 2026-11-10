// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"testing"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a foreign key name other than the derived one (PostgreSQL's _fkey1
// on a partitioned table) being lost between the model and the database.
func TestPostgresForeignKeyNameRoundTrip(t *testing.T) {
	db := testopen.Postgres(t)
	cols := []schema.Column{{Name: "id", Type: "bigint"}, {Name: "tenant_id", Type: "bigint"}, {Name: "created_at", Type: "timestamp with time zone"}}
	roundTrip(t, db, &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}}, // installed in the test database; inspection reports every extension
		Tables: []schema.Table{
			{Name: "tenant", Columns: []schema.Column{{Name: "id", Type: "bigint"}}, PrimaryKey: []string{"id"}},
			{
				Name: "audit_event", Columns: cols, PrimaryKey: []string{"id", "created_at"},
				Partition:   &schema.Partition{Strategy: "range", Key: "RANGE (created_at)"},
				ForeignKeys: []schema.ForeignKey{{Name: "audit_event_tenant_id_fkey1", Columns: []string{"tenant_id"}, RefTable: "tenant", RefColumns: []string{"id"}}},
			},
		},
	})
}
