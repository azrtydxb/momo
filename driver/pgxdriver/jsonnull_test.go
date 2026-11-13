// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"testing"

	"github.com/azrtydxb/momo/types"
)

type jnDoc struct{ A int }

// Catches: the wrapper replacing destinations that are not a pointer to a
// pointer to types.JSON (pgx must keep scanning those itself).
func TestWrapJSONNullLeavesOtherDestinations(t *testing.T) {
	var n int
	var s *string
	var j types.JSON[jnDoc]
	dest := []any{&n, &s, &j, nil}
	got := wrapJSONNull(dest)
	for i := range dest {
		if got[i] != dest[i] {
			t.Fatalf("dest %d replaced by %T", i, got[i])
		}
	}
}

// Catches: SQL NULL and the JSON literal null both scanning into a nil
// pointer (pgx's own behaviour for **JSON on json and jsonb).
func TestWrapJSONNullScan(t *testing.T) {
	var p *types.JSON[jnDoc]
	dest := wrapJSONNull([]any{&p})
	sc := dest[0].(interface{ Scan(any) error })

	if err := sc.Scan(nil); err != nil || p != nil {
		t.Fatalf("SQL NULL: %v, %+v", err, p)
	}
	if err := sc.Scan([]byte("null")); err != nil || p == nil || !p.IsNull() {
		t.Fatalf("JSON null: %v, %+v", err, p)
	}
	if err := sc.Scan([]byte(`{"A":2}`)); err != nil || p == nil || p.IsNull() || p.V.A != 2 {
		t.Fatalf("object: %v, %+v", err, p)
	}
	if err := sc.Scan(nil); err != nil || p != nil {
		t.Fatalf("SQL NULL after a value: %v, %+v", err, p)
	}
	if err := sc.Scan([]byte("{bad")); err == nil {
		t.Fatal("bad JSON must error")
	}
}
