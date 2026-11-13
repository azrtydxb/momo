// SPDX-License-Identifier: Apache-2.0

package types

import "testing"

type jsonNullDoc struct {
	Name string `json:"name"`
}

// Catches: Scan of the JSON literal null leaving no trace, so IsNull is false
// and Value writes the encoding of the zero T ({"name":""}) instead of null.
func TestJSONScanNullKeepsNull(t *testing.T) {
	for _, in := range []any{"null", []byte("null"), " null\n"} {
		var j JSON[jsonNullDoc]
		if err := j.Scan(in); err != nil {
			t.Fatalf("Scan(%q): %v", in, err)
		}
		if !j.IsNull() {
			t.Fatalf("Scan(%q): IsNull = false", in)
		}
		if v, err := j.Value(); err != nil || v != "null" {
			t.Fatalf("Scan(%q): Value = %v, %v; want null", in, v, err)
		}
	}
}

// Catches: a stale null marker surviving a later Scan of an object, or a
// change of V, so an object is written as null.
func TestJSONNullMarkerClears(t *testing.T) {
	var j JSON[jsonNullDoc]
	_ = j.Scan("null")
	if err := j.Scan(`{"name":"a"}`); err != nil || j.IsNull() {
		t.Fatalf("after object scan: %v, IsNull %v", err, j.IsNull())
	}
	n := JSONNull[jsonNullDoc]()
	n.V.Name = "b"
	if n.IsNull() {
		t.Fatal("IsNull still true after V changed")
	}
	if v, _ := n.Value(); v != `{"name":"b"}` {
		t.Fatalf("Value after change = %v", v)
	}
}

// Catches: IsNull reporting true for a value that is not the JSON literal
// null (an empty object, or a nil map that merely encodes as null).
func TestJSONIsNullOnlyForLiteralNull(t *testing.T) {
	var zero JSON[jsonNullDoc]
	if zero.IsNull() {
		t.Fatal("zero struct reports IsNull")
	}
	var m JSON[map[string]int]
	if m.IsNull() {
		t.Fatal("nil map not read from null reports IsNull")
	}
	if v, _ := m.Value(); v != "null" {
		t.Fatalf("nil map Value = %v, want null", v)
	}
	if !JSONNull[jsonNullDoc]().IsNull() {
		t.Fatal("JSONNull().IsNull() is false")
	}
	if v, _ := JSONNull[jsonNullDoc]().Value(); v != "null" {
		t.Fatalf("JSONNull Value = %v", v)
	}
}

// Catches: a plain JSON[T] accepting SQL NULL (the non-pointer field rule is
// an error, not a zero value).
func TestJSONScanSQLNullIsError(t *testing.T) {
	var j JSON[jsonNullDoc]
	if err := j.Scan(nil); err == nil {
		t.Fatal("Scan(nil) succeeded")
	}
}

// Catches: Set not clearing the null marker, or a direct V = zero value
// silently ending the JSON null (the zero value cannot be told from an
// untouched V, so only Set and a fresh JSON[T]{V: v} end it).
func TestJSONSetClearsNullButZeroAssignmentDoesNot(t *testing.T) {
	var p JSON[bool]
	if err := p.Scan("null"); err != nil || !p.IsNull() {
		t.Fatalf("scan null: %v, IsNull %v", err, p.IsNull())
	}
	p.V = false
	if !p.IsNull() {
		t.Fatal("assigning the zero value to V ended the JSON null; it must not")
	}
	if v, _ := p.Value(); v != "null" {
		t.Fatalf("Value after V = false: %v, want null", v)
	}
	p.Set(false)
	if p.IsNull() {
		t.Fatal("Set(false) left IsNull true")
	}
	if v, _ := p.Value(); v != "false" {
		t.Fatalf("Value after Set(false): %v, want false", v)
	}
	if v, _ := (JSON[bool]{V: false}).Value(); v != "false" {
		t.Fatalf("fresh JSON{V: false}: %v, want false", v)
	}
}
