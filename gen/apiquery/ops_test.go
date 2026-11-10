// SPDX-License-Identifier: Apache-2.0

package apiquery

import "testing"

// Catches: a per-model entry ignored in favour of the default, an unknown
// operation accepted, or an unconfigured model that no longer serves everything.
func TestOps(t *testing.T) {
	var none Ops
	for _, op := range AllOps {
		if !none.For("Document").Has(op) {
			t.Errorf("nil Ops lacks %s", op)
		}
	}
	o := Ops{DefaultModel: {"list", "get"}, "Tag": {"list", "create"}}
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	if s := o.For("Document"); !s.Has(OpList) || !s.Has(OpGet) || s.Has(OpCreate) || s.Has(OpDelete) {
		t.Errorf("default set = %v", s)
	}
	if s := o.For("Tag"); !s.Has(OpCreate) || s.Has(OpGet) {
		t.Errorf("Tag set = %v", s)
	}
	if err := (Ops{"X": {"drop"}}).Validate(); err == nil {
		t.Error("unknown operation accepted")
	}
	if err := (Ops{"X": {}}).Validate(); err == nil {
		t.Error("empty operation list accepted")
	}
	if !(Ops{"X": {"get"}}).For("Y").Has(OpDelete) {
		t.Error("a model without an entry and without a default must serve everything")
	}
}
