// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/schema"
)

// Catches: model or column names that are not protobuf identifiers, a model
// named Filter or after a derived message, and a duplicate of a derived
// message name reaching the .proto.
func TestProtoHostileNames(t *testing.T) {
	mk := func(name, table string, cols ...string) *gen.Model {
		m := &gen.Model{GoName: name, Table: table}
		for _, c := range cols {
			m.Fields = append(m.Fields, &gen.Field{GoName: "F", Column: c, BaseType: "int64"})
		}
		return m
	}
	g := &gen.Graph{
		Models: []*gen.Model{
			mk("Thing", "things", "id", "has space", "1st", "naïve", `q"x`),
			mk("ThingGetRequest", "tgr", "id"),
			mk("Filter", "f", "id"),
			mk("Näive", "n", "id"),
			mk("Fine", "fine", "id"),
		},
		Schema: &schema.Schema{Tables: []schema.Table{{Name: "things", PrimaryKey: []string{"id"}}, {Name: "fine", PrimaryKey: []string{"id"}}}},
	}
	src, diags := Proto(g, &Lock{}, "acme.v1")
	s := string(src)
	for _, leak := range []string{"has space", "1st", "naïve", `q"x`, "Näive", "service FilterService", "service ThingGetRequestService"} {
		if strings.Contains(s, leak) {
			t.Errorf("proto contains %q:\n%s", leak, s)
		}
	}
	if strings.Count(s, "message Filter {") != 1 || strings.Count(s, "message ThingGetRequest {") != 1 || !strings.Contains(s, "service FineService") {
		t.Errorf("shared or derived messages wrong:\n%s", s)
	}
	joined := ""
	for _, d := range diags {
		joined += d.Msg + "\n"
	}
	for _, want := range []string{
		`column "has space" is not a valid protobuf field name`,
		`column "naïve" is not a valid protobuf field name`,
		"model name Näive is not a valid protobuf identifier",
		"protobuf messages of model Filter collide: Filter is the shared Filter message",
		"protobuf messages of model ThingGetRequest collide: ThingGetRequest is also derived from model Thing",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("no diagnostic containing %q in:\n%s", want, joined)
		}
	}
}

// Catches: a sensitive or unmapped column losing its number when it is hidden,
// which would let a later column take it, and a retired name not written as
// reserved.
func TestProtoHiddenColumnsKeepNumbers(t *testing.T) {
	g := syntheticGraph()
	lock := &Lock{}
	Proto(g, lock, "acme.v1")
	if _, ok := lock.Messages["Thing"].Fields["secret"]; ok {
		t.Fatal("sensitive column was numbered")
	}
	// A column that is visible, then hidden by a sensitive tag, keeps its number.
	lock2 := &Lock{}
	lock2.Assign("Thing", []string{"id", "note"})
	g.Models[0].Fields[3].Tags.Sensitive = true // note
	src, _ := Proto(g, lock2, "acme.v1")
	if lock2.Messages["Thing"].Fields["note"] != 2 || len(lock2.Messages["Thing"].Reserved) != 0 {
		t.Fatalf("hidden column lost its number: %+v", lock2.Messages["Thing"])
	}
	if strings.Contains(string(src), "reserved") {
		t.Errorf("a hidden column was reserved:\n%s", src)
	}
	// A column gone from the model is retired by number and by name.
	lock3 := &Lock{}
	lock3.Assign("Thing", []string{"id", "gone"})
	src, _ = Proto(syntheticGraph(), lock3, "acme.v1")
	if !strings.Contains(string(src), "  reserved 2;\n  reserved \"gone\";\n") {
		t.Errorf("retired column not reserved by number and name:\n%s", src)
	}
}

// Catches: a change of the lock file format, which is committed and read back
// by later runs, and a retired name left reserved after its column returns.
func TestLockMarshalFormatAndNameReuse(t *testing.T) {
	l := &Lock{}
	l.Assign("M", []string{"id", "title"})
	l.Assign("M", []string{"id"})
	got, err := l.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	const want = `{
  "version": 1,
  "messages": {
    "M": {
      "fields": {
        "id": 1
      },
      "reserved": [
        2
      ],
      "reserved_names": [
        "title"
      ]
    }
  }
}
`
	if string(got) != want {
		t.Fatalf("lock format changed:\n%s", got)
	}
	back, diags := l.Assign("M", []string{"id", "title"})
	if len(diags) != 0 || back["title"] != 3 || len(l.Messages["M"].ReservedNames) != 0 || len(l.Messages["M"].Reserved) != 1 {
		t.Fatalf("returning column: %v %v %+v", back, diags, l.Messages["M"])
	}
}
