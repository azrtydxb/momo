// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"testing"

	"github.com/azrtydxb/momo/types"
)

type jnDoc struct{ A int }

// userNuller is not a types.JSON but has Scan and IsNull.
type userNuller struct{}

func (*userNuller) Scan(any) error { return nil }

func (userNuller) IsNull() bool { return false }

// Catches: the wrapper replacing destinations that are not a pointer to a
// pointer to types.JSON (pgx must keep scanning those itself), including a
// user type that merely has Scan and IsNull.
func TestWrapJSONNullLeavesOtherDestinations(t *testing.T) {
	var n int
	var s *string
	var j types.JSON[jnDoc]
	var u *userNuller
	dest := []any{&n, &s, &j, nil, &u}
	got := wrapJSONNull(dest)
	for i := range dest {
		if got[i] != dest[i] {
			t.Fatalf("dest %d replaced by %T", i, got[i])
		}
	}
}

// Catches: the wrapper mutating the caller's dest slice instead of copying it
// before it replaces a destination.
func TestWrapJSONNullDoesNotMutateDest(t *testing.T) {
	var p *types.JSON[jnDoc]
	var n int
	dest := []any{&n, &p}
	orig := []any{&n, &p}
	got := wrapJSONNull(dest)
	if _, ok := got[1].(ptrScanner); !ok {
		t.Fatalf("dest 1 not wrapped: %T", got[1])
	}
	for i := range dest {
		if dest[i] != orig[i] {
			t.Fatalf("caller's dest[%d] changed to %T", i, dest[i])
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
