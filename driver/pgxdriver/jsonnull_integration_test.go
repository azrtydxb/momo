// SPDX-License-Identifier: Apache-2.0

//go:build integration

package pgxdriver

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/types"
)

const jsonNullRowsSQL = `SELECT v FROM (VALUES (1, NULL::jsonb), (2, 'null'::jsonb), (3, '{"A":1}'::jsonb)) AS t(id, v) ORDER BY id`

// readJSONNullRows scans the three rows of jsonNullRowsSQL, each into a
// *types.JSON[jnDoc], and checks SQL NULL, JSON null and an object.
func readJSONNullRows(t *testing.T, rows momo.Rows) {
	t.Helper()
	defer rows.Close()
	var got []*types.JSON[jnDoc]
	for rows.Next() {
		var p *types.JSON[jnDoc]
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		got = append(got, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3", len(got))
	}
	if got[0] != nil {
		t.Fatalf("SQL NULL read as %+v, want a nil pointer", got[0])
	}
	if got[1] == nil || !got[1].IsNull() {
		t.Fatalf("JSON null read as %+v, want a non-nil value with IsNull true", got[1])
	}
	if got[2] == nil || got[2].IsNull() || got[2].V.A != 1 {
		t.Fatalf("object read as %+v", got[2])
	}
}

// Catches: the native driver's Query path reading a JSON null as a nil
// pointer (pgx's own plan for a pointer to a pointer on jsonb).
func TestNativeQueryReadsJSONNull(t *testing.T) {
	d := New(openPool(t))
	rows, err := d.Query(context.Background(), jsonNullRowsSQL, nil)
	if err != nil {
		t.Fatal(err)
	}
	readJSONNullRows(t, rows)
}

// Catches: the buffered rows of a batch (bufferedRows.Scan, which has its own
// pgx type-map call) reading a JSON null as a nil pointer.
func TestBatchReadsJSONNull(t *testing.T) {
	d := New(openPool(t)).(momo.Batcher)
	out, err := d.SendBatch(context.Background(), []momo.Compiled{{SQL: jsonNullRowsSQL}})
	if err != nil || len(out) != 1 {
		t.Fatalf("SendBatch: %v (%d results)", err, len(out))
	}
	readJSONNullRows(t, out[0])
}

// Catches: the FromSQLDB path (database/sql conversion) losing the JSON null
// or the SQL NULL.
func TestFromSQLDBReadsJSONNull(t *testing.T) {
	_, d := openSQLDriver(t)
	rows, err := d.Query(context.Background(), jsonNullRowsSQL, nil)
	if err != nil {
		t.Fatal(err)
	}
	readJSONNullRows(t, rows)
}
