// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

const sample = `       0      120 T example.com/momo.Select[go.shape.struct { momo.Model; ID int64 }].All
       0      200 T example.com/momo.Select[go.shape.struct { momo.Model; Name string }].All
       0       80 T example.com/momo.Column[go.shape.int64].Eq
       0       60 t example.com/momo.plain
       0       10 D example.com/momo..dict.Select[go.shape.int64]
         U runtime.morestack
`

// Catches: shape names that contain spaces being cut short, which would put
// every instantiation of one generic function in a different group.
func TestParseKeepsSpacesInNames(t *testing.T) {
	syms, err := parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 5 {
		t.Fatalf("parsed %d symbols, want 5 (the undefined symbol is skipped)", len(syms))
	}
	if want := "example.com/momo.Select[go.shape.struct { momo.Model; ID int64 }].All"; syms[0].name != want {
		t.Errorf("name = %q, want %q", syms[0].name, want)
	}
}

// Catches: instantiations of one generic function counted as several, and
// plain functions counted as generic.
func TestGroupCountsInstantiationsPerGenericFunction(t *testing.T) {
	syms, _ := parse(strings.NewReader(sample))
	aggs, inst, instBytes, textBytes, dicts := group(syms)
	if inst != 3 || instBytes != 400 || textBytes != 460 || dicts != 1 {
		t.Errorf("inst=%d instBytes=%d textBytes=%d dicts=%d, want 3 400 460 1", inst, instBytes, textBytes, dicts)
	}
	var sel *agg
	for i := range aggs {
		if aggs[i].name == "example.com/momo.Select.All" {
			sel = &aggs[i]
		}
	}
	if sel == nil || sel.count != 2 || sel.size != 320 {
		t.Errorf("Select.All = %+v, want 2 instantiations of 320 bytes", sel)
	}
	if out := report(syms, 5, "x"); !strings.Contains(out, "| 1 | `example.com/momo.Select.All` | 2 | 320 |") {
		t.Errorf("report lacks the top row:\n%s", out)
	}
}

func TestBaseName(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{"a.F[int]", "a.F", true},
		{"a.T[go.shape.int].M", "a.T.M", true},
		{"a.F[x[y]]", "a.F", true},
		{"a.plain", "a.plain", false},
		{"a.F[unbalanced", "a.F[unbalanced", false},
	} {
		got, ok := baseName(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("baseName(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
