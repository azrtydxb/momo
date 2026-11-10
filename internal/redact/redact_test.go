// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"
)

func TestMaskReplacesSensitivePositions(t *testing.T) {
	got := Mask([]any{"a", "secret", 3}, []int{1})
	want := []any{"a", Arg, 3}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMaskNilStaysNil(t *testing.T) {
	if got := Mask(nil, []int{0}); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

// A hook that mutates the masked arguments must not reach the slice the driver
// receives, including the bytes behind a []byte or json.RawMessage.
func TestMaskReturnsCopies(t *testing.T) {
	raw := []byte("abc")
	msg := json.RawMessage(`{"a":1}`)
	args := []any{raw, msg, "x"}
	out := Mask(args, nil)

	out[2] = "changed"
	out[0].([]byte)[0] = 'Z'
	out[1].(json.RawMessage)[0] = '['
	if args[2] != "x" {
		t.Fatalf("driver slice element changed: %v", args[2])
	}
	if !bytes.Equal(raw, []byte("abc")) {
		t.Fatalf("driver []byte changed: %q", raw)
	}
	if string(msg) != `{"a":1}` {
		t.Fatalf("driver json.RawMessage changed: %s", msg)
	}
}

func TestMaskIgnoresOutOfRangeIndexes(t *testing.T) {
	args := []any{"a", "b"}
	got := Mask(args, []int{-1, 2, 99, 0})
	want := []any{Arg, "b"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if args[0] != "a" {
		t.Fatalf("input slice was masked in place: %v", args)
	}
}
