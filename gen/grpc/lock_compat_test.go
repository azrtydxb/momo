// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/schema"
)

func writeLock(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "momo.proto.lock")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// Catches: a lock written before reserved_names existed being refused, or
// regenerating from it renumbering a field or reusing the retired number.
func TestOldFormatLockRegenerates(t *testing.T) {
	old := `{"version":1,"messages":{"Thing":{"fields":{"id":1,"note":2},"reserved":[3]}}}`
	l, err := LoadLock(writeLock(t, old))
	if err != nil {
		t.Fatalf("old-format lock refused: %v", err)
	}
	m := &gen.Model{GoName: "Thing", Table: "things", Fields: []*gen.Field{
		{GoName: "ID", Column: "id", BaseType: "int64"},
		{GoName: "Note", Column: "note", BaseType: "string"},
		{GoName: "Extra", Column: "extra", BaseType: "string"},
	}}
	g := &gen.Graph{Models: []*gen.Model{m}, Schema: &schema.Schema{Tables: []schema.Table{{Name: "things", PrimaryKey: []string{"id"}}}}}
	src, diags := Proto(g, l, "acme.v1")
	if len(diags) != 0 {
		t.Fatalf("diags = %v", diags)
	}
	s := string(src)
	for _, want := range []string{"  reserved 3;\n", "  int64 id = 1;\n", "  string note = 2;\n", "  string extra = 4;\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("proto lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "reserved \"") {
		t.Errorf("old lock has no names to reserve:\n%s", s)
	}
}

// Catches: a reserved_names list that is not a set of identifiers, or that
// overlaps a live field, being trusted.
func TestLoadLockValidatesReservedNames(t *testing.T) {
	for name, body := range map[string]string{
		"not an identifier": `{"version":1,"messages":{"M":{"fields":{},"reserved_names":["a b"]}}}`,
		"repeated":          `{"version":1,"messages":{"M":{"fields":{},"reserved_names":["a","a"]}}}`,
		"is a field":        `{"version":1,"messages":{"M":{"fields":{"a":1},"reserved_names":["a"]}}}`,
	} {
		if _, err := LoadLock(writeLock(t, body)); err == nil {
			t.Errorf("%s: lock was accepted", name)
		}
	}
	if _, err := LoadLock(writeLock(t, `{"version":1,"messages":{"M":{"fields":{"a":1},"reserved":[2],"reserved_names":["b"]}}}`)); err != nil {
		t.Errorf("valid reserved names refused: %v", err)
	}
}

// Catches: two columns whose proto3 JSON names are equal (foo_bar and fooBar)
// reaching the .proto, where protoc rejects the message.
func TestProtoJSONNameCollision(t *testing.T) {
	m := &gen.Model{GoName: "Thing", Table: "things", Fields: []*gen.Field{
		{GoName: "ID", Column: "id", BaseType: "int64"},
		{GoName: "A", Column: "foo_bar", BaseType: "string"},
		{GoName: "B", Column: "fooBar", BaseType: "string"},
	}}
	g := &gen.Graph{Models: []*gen.Model{m}, Schema: &schema.Schema{Tables: []schema.Table{{Name: "things", PrimaryKey: []string{"id"}}}}}
	lock := &Lock{}
	src, diags := Proto(g, lock, "acme.v1")
	s := string(src)
	if !strings.Contains(s, " foo_bar = ") || strings.Contains(s, " fooBar = ") {
		t.Errorf("the first column should win:\n%s", s)
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, `same proto3 JSON name "fooBar" as column "foo_bar"`) {
		t.Fatalf("diags = %v", diags)
	}
	if _, ok := lock.Messages["Thing"].Fields["fooBar"]; ok {
		t.Error("the rejected column was numbered")
	}

	// protoc compares the JSON names without regard to case, so foo_bar and
	// foobar (JSON name fooBar and foobar) and Foo and foo conflict too.
	for _, pair := range [][2]string{{"foo_bar", "foobar"}, {"Foo", "foo"}, {"foobar", "foo_bar"}} {
		m := &gen.Model{GoName: "Thing", Table: "things", Fields: []*gen.Field{
			{GoName: "ID", Column: "id", BaseType: "int64"},
			{GoName: "A", Column: pair[0], BaseType: "string"},
			{GoName: "B", Column: pair[1], BaseType: "string"},
		}}
		g := &gen.Graph{Models: []*gen.Model{m}, Schema: &schema.Schema{Tables: []schema.Table{{Name: "things", PrimaryKey: []string{"id"}}}}}
		src, diags := Proto(g, &Lock{}, "acme.v1")
		s := string(src)
		if !strings.Contains(s, " "+pair[0]+" = ") || strings.Contains(s, " "+pair[1]+" = ") {
			t.Errorf("%v: the first column should win:\n%s", pair, s)
		}
		if len(diags) != 1 || !strings.Contains(diags[0].Msg, "same proto3 JSON name") {
			t.Errorf("%v: diags = %v", pair, diags)
		}
	}
}

// Catches: a new column placed before an existing one taking the field of the
// column the lock already numbers, which would silently drop that field from
// the wire.
func TestProtoJSONNameCollisionKeepsLockedColumn(t *testing.T) {
	lock := &Lock{Version: 1, Messages: map[string]*MessageLock{
		"Thing": {Fields: map[string]int{"id": 1, "fooBar": 2}},
	}}
	m := &gen.Model{GoName: "Thing", Table: "things", Fields: []*gen.Field{
		{GoName: "ID", Column: "id", BaseType: "int64"},
		{GoName: "New", Column: "foo_bar", BaseType: "string"},
		{GoName: "Old", Column: "fooBar", BaseType: "string"},
	}}
	g := &gen.Graph{Models: []*gen.Model{m}, Schema: &schema.Schema{Tables: []schema.Table{{Name: "things", PrimaryKey: []string{"id"}}}}}
	src, diags := Proto(g, lock, "acme.v1")
	s := string(src)
	if !strings.Contains(s, " string fooBar = 2;") || strings.Contains(s, "foo_bar") {
		t.Errorf("the locked column should keep its field:\n%s", s)
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, `column "foo_bar" of Thing has the same proto3 JSON name "fooBar" as column "fooBar"`) {
		t.Fatalf("diags = %v", diags)
	}
	ml := lock.Messages["Thing"]
	if ml.Fields["fooBar"] != 2 || ml.Fields["id"] != 1 {
		t.Errorf("lock changed the recorded numbers: %+v", ml)
	}
	if _, ok := ml.Fields["foo_bar"]; ok || len(ml.Reserved) != 0 {
		t.Errorf("the newcomer was numbered or a number was reserved: %+v", ml)
	}
}
