// SPDX-License-Identifier: Apache-2.0

package types

import (
	"encoding/json"
	"testing"
)

// Catches: IsNull false for JSON[json.RawMessage], the type the generator maps
// json and jsonb to: Unmarshal of null leaves RawMessage("null"), which is not
// the zero value, so a scanned null looked like data.
func TestJSONRawMessageNull(t *testing.T) {
	var j JSON[json.RawMessage]
	if err := j.Scan([]byte("null")); err != nil {
		t.Fatal(err)
	}
	if !j.IsNull() {
		t.Fatalf("IsNull = false for a scanned null, V = %q", j.V)
	}
	if v, err := j.Value(); err != nil || v != "null" {
		t.Fatalf("Value = %v, %v; want null", v, err)
	}
}

// Catches: IsNull true for a RawMessage that holds data, or a stale marker
// after a later Scan.
func TestJSONRawMessageData(t *testing.T) {
	var j JSON[json.RawMessage]
	if err := j.Scan(`{"a":1}`); err != nil {
		t.Fatal(err)
	}
	if j.IsNull() {
		t.Fatal("IsNull true for an object")
	}
	if v, _ := j.Value(); v != `{"a":1}` {
		t.Fatalf("Value = %v", v)
	}
	_ = j.Scan("null")
	_ = j.Scan("[1]")
	if j.IsNull() {
		t.Fatal("marker survived a later Scan")
	}
	j = JSON[json.RawMessage]{V: json.RawMessage(`null`)}
	if j.IsNull() {
		t.Fatal("a fresh value with null data and no marker reports IsNull")
	}
}
