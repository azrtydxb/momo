// SPDX-License-Identifier: Apache-2.0

package apiquery

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testmodels/example/tag"
)

// Catches: a cursor that does not survive encoding, or a malformed cursor
// that is accepted or is not reported as ErrValidation.
func TestCursorRoundTrip(t *testing.T) {
	c := EncodeCursor("42", "2026-01-02T03:04:05Z")
	got, err := DecodeCursor(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Keys) != 2 || got.Keys[0] != "42" || got.Keys[1] != "2026-01-02T03:04:05Z" {
		t.Fatalf("keys=%v", got.Keys)
	}
	for _, bad := range []string{"!!!", "", "e30", "bm90LWpzb24"} {
		if _, err := DecodeCursor(bad); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("DecodeCursor(%q): %v", bad, err)
		}
	}
}

// Catches: a page size outside the default and maximum being passed through.
func TestPageLimit(t *testing.T) {
	cases := map[int]int{0: DefaultLimit, -3: DefaultLimit, 10: 10, 500: MaxLimit, MaxLimit: MaxLimit}
	for in, want := range cases {
		if got := ClampLimit(in); got != want {
			t.Errorf("ClampLimit(%d)=%d want %d", in, got, want)
		}
	}
}

// Catches: a column outside the generated allowlist, or an operator outside
// the fixed set, reaching a predicate.
func TestFilterAllowlist(t *testing.T) {
	lookup := func(name string) (momo.ColumnRef, error) {
		return nil, errors.New("not allowed: " + name)
	}
	if _, err := ParseFilter(lookup, Filter{Column: "password", Op: "eq", Value: "x"}); err == nil {
		t.Fatal("unknown column accepted")
	}
	if _, err := ParseFilter(lookup, Filter{Column: "id", Op: "like", Value: "x"}); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("unknown op: %v", err)
	}

	if _, err := ParseFilter(tag.ColumnByName, Filter{Column: "id", Op: "gt", Value: "7"}); err != nil {
		t.Fatalf("allowlisted filter: %v", err)
	}
	if _, err := ParseFilter(tag.ColumnByName, Filter{Column: "id", Op: "gt", Value: "seven"}); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("bad value: %v", err)
	}
	if _, err := ParseFilter(tag.ColumnByName, Filter{Column: "nope", Op: "eq", Value: "1"}); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("unknown column from generated lookup: %v", err)
	}
}
