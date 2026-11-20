// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"reflect"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/schemautil"
)

// Catches: a deprecated forwarder drifting from the internal function it
// stands for, so code that has not moved yet changes behavior.
func TestDeprecatedForwarders(t *testing.T) {
	if p := Int64(7); p == nil || *p != 7 || *schemautil.Int64(7) != 7 {
		t.Errorf("Int64(7) = %v", p)
	}
	for _, s := range []string{"", "users", "a_b"} {
		if got, want := FTS5Table(s), schemautil.FTS5Table(s); got != want {
			t.Errorf("FTS5Table(%q) = %q, want %q", s, got, want)
		}
		if got, want := TSVectorColumn(s), schemautil.TSVectorColumn(s); got != want {
			t.Errorf("TSVectorColumn(%q) = %q, want %q", s, got, want)
		}
	}
	for _, v := range [][]string{nil, {"b", "a", "b"}, {"x"}} {
		if got, want := SortedValues(v), schemautil.SortedValues(v); !reflect.DeepEqual(got, want) {
			t.Errorf("SortedValues(%v) = %v, want %v", v, got, want)
		}
		for _, d := range []dialect.Name{dialect.Postgres, dialect.MySQL, dialect.SQLite} {
			if got, want := CheckInExpr(d, "col", v), schemautil.CheckInExpr(d, "col", v); got != want {
				t.Errorf("CheckInExpr(%s, %v) = %q, want %q", d, v, got, want)
			}
		}
	}
	for _, e := range []string{"", "qty > 0", `"s" IN ('a','b')`, `s = ANY (ARRAY['a'::text, 'b'::text])`} {
		c1, v1, ok1 := ParseCheckIn(e)
		c2, v2, ok2 := schemautil.ParseCheckIn(e)
		if c1 != c2 || !reflect.DeepEqual(v1, v2) || ok1 != ok2 {
			t.Errorf("ParseCheckIn(%q) differs from schemautil", e)
		}
	}
	a := Check{Name: "c", In: &CheckIn{Column: "s", Values: []string{"a", "b"}}}
	b := Check{Name: "c", Expr: `"s" IN ('b','a')`}
	if got, want := SameCheck(a, b), sameCheck(a, b); got != want || !got {
		t.Errorf("SameCheck = %v, sameCheck = %v, want both true", got, want)
	}
}

// Catches: the deprecated SchemaNamespace alias becoming a distinct type, which
// would stop old code from filling Schema.Schemas.
func TestSchemaNamespaceIsAnAliasOfNamespace(t *testing.T) {
	s := Schema{Schemas: []SchemaNamespace{{Name: "audit"}}}
	takeNamespace := func(n Namespace) string { return n.Name }
	if takeNamespace(s.Schemas[0]) != "audit" || reflect.TypeOf(SchemaNamespace{}) != reflect.TypeOf(Namespace{}) {
		t.Fatalf("SchemaNamespace is not Namespace: %+v", s.Schemas)
	}
}
