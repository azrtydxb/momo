// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/types"
)

// digest is a [16]byte type with a String method that is not a UUID.
type digest [16]byte

func (d digest) String() string { return "md5:abcdef" }

func TestConvertUUIDSliceArgs(t *testing.T) {
	a, b := types.UUID{1}, types.UUID{2}
	digests := []digest{{9}}
	args := []any{"x", []types.UUID{a, b}, []types.UUID{}, []types.UUID(nil), []byte("raw"), [][16]byte{{3}}, 7, digests}
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
	if orig, ok := args[1].([]types.UUID); !ok || len(orig) != 2 || orig[0] != a || orig[1] != b {
		t.Errorf("input args were modified: %v", args[1])
	}

	if _, ok := got[7].([]digest); !ok {
		t.Errorf("a non-UUID [16]byte digest slice was converted: %T", got[7])
	}

	plain := []any{1, "a", []byte("b")}
	if out := convertUUIDSliceArgs(plain); &out[0] != &plain[0] {
		t.Error("args without UUID slices were copied")
	}
}

// Catches: the batch and bulk path missing the conversion, since it shares
// convertVectorArgs only if the conversion lives there.
func TestConvertVectorStmtsConvertsUUIDSlices(t *testing.T) {
	u := types.UUID{5}
	stmts := []momo.Compiled{{SQL: "SELECT 1 WHERE id = ANY($1)", Args: []any{[]types.UUID{u}}}}
	for _, vf := range []VectorFormat{VectorText, VectorBinary} {
		out := convertVectorStmts(vf, stmts)
		if want := "{" + u.String() + "}"; out[0].Args[0] != want {
			t.Errorf("format %v: arg = %v, want %s", vf, out[0].Args[0], want)
		}
	}
	if _, ok := stmts[0].Args[0].([]types.UUID); !ok {
		t.Error("input statements were modified")
	}
}
