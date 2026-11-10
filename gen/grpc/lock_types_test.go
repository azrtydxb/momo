// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/schema"
)

func typedModel(goName, table string, fields ...*gen.Field) *gen.Graph {
	m := &gen.Model{GoName: goName, Table: table, Fields: fields}
	return &gen.Graph{Models: []*gen.Model{m}, Schema: &schema.Schema{Tables: []schema.Table{{Name: table, PrimaryKey: []string{"id"}}}}}
}

// Catches: a field that changes type (or becomes optional or repeated) keeping
// its number, which makes a reader of the old schema decode new bytes as the
// old type.
func TestLockRetiresNumberOnTypeChange(t *testing.T) {
	lock := &Lock{}
	g := typedModel("Thing", "things",
		&gen.Field{GoName: "ID", Column: "id", BaseType: "int64"},
		&gen.Field{GoName: "Count", Column: "count", BaseType: "int32"},
		&gen.Field{GoName: "Name", Column: "name", BaseType: "string"},
	)
	if _, ds := Proto(g, lock, "acme.v1"); len(ds) != 0 {
		t.Fatal(ds)
	}
	ml := lock.Messages["Thing"]
	if ml.Fields["count"] != 2 || ml.Types["count"] != "int32" || ml.Table != "things" {
		t.Fatalf("lock after the first run: %+v", ml)
	}

	// int32 -> int64 for count, and name becomes optional
	g = typedModel("Thing", "things",
		&gen.Field{GoName: "ID", Column: "id", BaseType: "int64"},
		&gen.Field{GoName: "Count", Column: "count", BaseType: "int64"},
		&gen.Field{GoName: "Name", Column: "name", BaseType: "string", Nullable: true},
	)
	src, ds := Proto(g, lock, "acme.v1")
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	ml = lock.Messages["Thing"]
	if ml.Fields["id"] != 1 || ml.Fields["count"] != 4 || ml.Fields["name"] != 5 {
		t.Errorf("numbers after the type changes: %v", ml.Fields)
	}
	if !reflect.DeepEqual(ml.Reserved, []int{2, 3}) {
		t.Errorf("reserved = %v, want [2 3]", ml.Reserved)
	}
	if len(ml.ReservedNames) != 0 {
		t.Errorf("a column that still exists must not have its name reserved: %v", ml.ReservedNames)
	}
	for _, want := range []string{"reserved 2, 3;", "int64 count = 4;", "optional string name = 5;"} {
		if !strings.Contains(string(src), want) {
			t.Errorf("proto lacks %q:\n%s", want, src)
		}
	}
	if ml.Types["count"] != "int64" || ml.Types["name"] != "optional string" {
		t.Errorf("types = %v", ml.Types)
	}

	// an unchanged run keeps everything
	if _, ds := Proto(g, lock, "acme.v1"); len(ds) != 0 || lock.Messages["Thing"].Fields["count"] != 4 || len(lock.Messages["Thing"].Reserved) != 2 {
		t.Errorf("a second run changed the lock: %+v %v", lock.Messages["Thing"], ds)
	}
}

// Catches: a lock written before types were recorded losing numbers, or a type
// being recorded as a change on the first run that sees it.
func TestLockWithoutTypesAdoptsThem(t *testing.T) {
	lock := &Lock{Version: 1, Messages: map[string]*MessageLock{"Thing": {Fields: map[string]int{"id": 1, "count": 2}}}}
	g := typedModel("Thing", "things",
		&gen.Field{GoName: "ID", Column: "id", BaseType: "int64"},
		&gen.Field{GoName: "Count", Column: "count", BaseType: "int64"},
	)
	if _, ds := Proto(g, lock, "acme.v1"); len(ds) != 0 {
		t.Fatal(ds)
	}
	ml := lock.Messages["Thing"]
	if ml.Fields["count"] != 2 || len(ml.Reserved) != 0 || ml.Types["count"] != "int64" {
		t.Errorf("lock = %+v", ml)
	}
}

// Catches: a renamed Go type getting fresh field numbers, which changes the
// wire format of every field.
func TestLockMessagesFollowTheTable(t *testing.T) {
	lock := &Lock{}
	first := typedModel("Document", "documents",
		&gen.Field{GoName: "ID", Column: "id", BaseType: "int64"},
		&gen.Field{GoName: "Title", Column: "title", BaseType: "string"},
	)
	if _, ds := Proto(first, lock, "acme.v1"); len(ds) != 0 {
		t.Fatal(ds)
	}
	if lock.Messages["Document"].Fields["title"] != 2 || lock.Messages["DocumentGetRequest"] == nil {
		t.Fatalf("first lock: %+v", lock.Messages)
	}
	renamed := typedModel("Doc", "documents",
		&gen.Field{GoName: "ID", Column: "id", BaseType: "int64"},
		&gen.Field{GoName: "Summary", Column: "summary", BaseType: "string"},
		&gen.Field{GoName: "Title", Column: "title", BaseType: "string"},
	)
	src, ds := Proto(renamed, lock, "acme.v1")
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	if _, old := lock.Messages["Document"]; old {
		t.Error("the old message entry was left behind")
	}
	ml := lock.Messages["Doc"]
	if ml == nil || ml.Fields["id"] != 1 || ml.Fields["title"] != 2 || ml.Fields["summary"] != 3 {
		t.Fatalf("the renamed message did not adopt the numbers: %+v", ml)
	}
	if lock.Messages["DocGetRequest"] == nil || lock.Messages["DocGetRequest"].Fields["id"] != 1 {
		t.Errorf("the request messages did not follow: %+v", lock.Messages)
	}
	if !strings.Contains(string(src), "string title = 2;") {
		t.Errorf("title lost its number:\n%s", src)
	}

	// two models of one run never adopt each other's entry
	both := &gen.Graph{Models: []*gen.Model{
		{GoName: "A", Table: "a", Fields: []*gen.Field{{GoName: "ID", Column: "id", BaseType: "int64"}}},
		{GoName: "B", Table: "b", Fields: []*gen.Field{{GoName: "ID", Column: "id", BaseType: "int64"}}},
	}, Schema: &schema.Schema{Tables: []schema.Table{{Name: "a", PrimaryKey: []string{"id"}}, {Name: "b", PrimaryKey: []string{"id"}}}}}
	l2 := &Lock{}
	Proto(both, l2, "acme.v1")
	if l2.Messages["A"].Table != "a" || l2.Messages["B"].Table != "b" {
		t.Errorf("tables = %q %q", l2.Messages["A"].Table, l2.Messages["B"].Table)
	}
}
