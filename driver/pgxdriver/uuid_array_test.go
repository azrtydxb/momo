// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"reflect"
	"testing"

	"github.com/azrtydxb/momo/types"
)

// Catches: a []types.UUID reaching pgx unconverted (pgx cannot encode its
// elements), a nil slice turned into an empty array, and the caller's argument
// slice being modified.
func TestConvertUUIDArrayArgs(t *testing.T) {
	a, b := types.UUID{1}, types.UUID{0xff}
	in := []any{int64(7), []types.UUID{a, b}, []types.UUID{}, []types.UUID(nil), "x"}
	orig := append([]any(nil), in...)
	got := convertUUIDArrayArgs(in)
	want := []any{int64(7), "{" + a.String() + "," + b.String() + "}", "{}", []types.UUID(nil), "x"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(in, orig) {
		t.Error("input args were modified")
	}
	plain := []any{1, "a"}
	if out := convertUUIDArrayArgs(plain); &out[0] != &plain[0] {
		t.Error("args without a uuid slice must come back unchanged")
	}
}
