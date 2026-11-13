// SPDX-License-Identifier: Apache-2.0

//go:build integration

package types_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/types"
)

// Catches: a JSON null in a nullable column read into *types.JSON[json.RawMessage],
// the generated default type, reporting IsNull false, and so writing back as
// data instead of the JSON null.
func TestJSONRawMessageNullLive(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		tbl := "json_null_raw"
		if _, err := momo.Exec(ctx, db, "CREATE TABLE "+tbl+" (id int PRIMARY KEY, v "+types.JSON[json.RawMessage]{}.SQLType(d)+")"); err != nil {
			t.Fatal(err)
		}
		for id, v := range map[int]any{
			1: (*types.JSON[json.RawMessage])(nil),
			2: &types.JSON[json.RawMessage]{V: json.RawMessage("null")},
			3: &types.JSON[json.RawMessage]{V: json.RawMessage(`{"a":1}`)},
		} {
			if _, err := momo.Exec(ctx, db, "INSERT INTO "+tbl+" (id, v) VALUES (?, ?)", id, v); err != nil {
				t.Fatalf("insert %d: %v", id, err)
			}
		}
		got, err := momo.Scalars[*types.JSON[json.RawMessage]](ctx, db, "SELECT v FROM "+tbl+" ORDER BY id")
		if err != nil || len(got) != 3 {
			t.Fatalf("select: %v (%d rows)", err, len(got))
		}
		if got[0] != nil {
			t.Fatalf("SQL NULL read as %+v", got[0])
		}
		if got[1] == nil || !jsonIsNull(got[1]) {
			t.Fatalf("JSON null read as %+v, want IsNull true", got[1])
		}
		if got[2] == nil || jsonIsNull(got[2]) {
			t.Fatalf("object read as %+v", got[2])
		}
	})
}
