// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"testing"

	"github.com/azrtydxb/momo/types"
)

func TestConvertUUIDSliceArgs(t *testing.T) {
	a, b := types.UUID{1}, types.UUID{2}
	args := []any{"x", []types.UUID{a, b}, []types.UUID{}, []types.UUID(nil), []byte("raw"), [][16]byte{{3}}, 7}
	got := convertUUIDSliceArgs(args)
	if want := "{" + a.String() + "," + b.String() + "}"; got[1] != want {
		t.Errorf("[]types.UUID = %v, want %s", got[1], want)
	}
	if got[2] != "{}" {
		t.Errorf("empty = %v, want {}", got[2])
	}
	if got[3] != nil {
		t.Errorf("nil slice = %v, want nil (NULL)", got[3])
	}
	if got[0] != "x" || got[6] != 7 {
		t.Errorf("scalars changed: %v", got)
	}
	// Bytes and a [16]byte element without a String method are left alone.
	if _, ok := got[4].([]byte); !ok {
		t.Errorf("[]byte was converted: %T", got[4])
	}
	if _, ok := got[5].([][16]byte); !ok {
		t.Errorf("[][16]byte was converted: %T", got[5])
	}
	if args[1].([]types.UUID) == nil {
		t.Error("input args were modified")
	}

	plain := []any{1, "a", []byte("b")}
	if out := convertUUIDSliceArgs(plain); &out[0] != &plain[0] {
		t.Error("args without UUID slices were copied")
	}
}
