// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"reflect"
	"testing"
)

// Catches: generated code that names a descriptor by the Go field name when
// the descriptor package renamed it (a field called Table or As).
func TestDescriptorIdents(t *testing.T) {
	m := &Model{GoName: "Thing", Table: "things", PkgPath: "example.com/m", PkgName: "m", SubPkg: "thing",
		Fields: []*Field{
			{GoName: "ID", Column: "id", GoType: "int64", BaseType: "int64"},
			{GoName: "Table", Column: "table_x", GoType: "string", BaseType: "string"},
			{GoName: "As", Column: "as_x", GoType: "string", BaseType: "string"},
		},
		Relations: []*Relation{{GoName: "Table"}, {GoName: "Parts"}},
	}
	fields, rels := DescriptorIdents(m, &Graph{})
	if want := []string{"ID", "Table_", "As_"}; !reflect.DeepEqual(fields, want) {
		t.Errorf("fields = %v, want %v", fields, want)
	}
	if want := []string{"Table__", "Parts"}; !reflect.DeepEqual(rels, want) {
		t.Errorf("relations = %v, want %v", rels, want)
	}
}
