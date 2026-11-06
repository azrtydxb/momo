// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func idxSchema(typ string, ix Index) *Schema {
	return &Schema{Dialect: dialect.Postgres, Tables: []Table{{
		Name:    "d",
		Columns: []Column{{Name: "c", Type: typ}},
		Indexes: []Index{ix},
	}}}
}

func TestEqualDefaultOpClass(t *testing.T) {
	cases := []struct {
		name, typ, method, opclass string
		equal                      bool
	}{
		{"ivfflat vector l2 is default", "vector(3)", "ivfflat", "vector_l2_ops", true},
		{"ivfflat vector cosine is not", "vector(3)", "ivfflat", "vector_cosine_ops", false},
		{"hnsw has no default opclass", "vector(3)", "hnsw", "vector_l2_ops", false},
		{"btree text", "text", "", "text_ops", true},
		{"btree explicit method", "text", "btree", "text_ops", true},
		{"btree bigint", "int8", "", "int8_ops", true},
		{"btree pattern ops is not default", "text", "", "text_pattern_ops", false},
		{"gin array", "text[]", "gin", "array_ops", true},
		{"gin jsonb", "jsonb", "gin", "jsonb_ops", true},
		{"gin jsonb_path_ops is not default", "jsonb", "gin", "jsonb_path_ops", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			model := idxSchema(c.typ, Index{Name: "i", Method: c.method, Parts: []IndexPart{{Column: "c", OpClass: c.opclass}}})
			// What inspection reports: the method btree is spelled "" and a default opclass is omitted.
			inspected := idxSchema(c.typ, Index{Name: "i", Method: c.method, Parts: []IndexPart{{Column: "c"}}})
			if got := Equal(model, inspected); got != c.equal {
				t.Fatalf("Equal = %v, want %v", got, c.equal)
			}
			if again := Normalized(Normalized(model)); !Equal(again, model) {
				t.Fatal("normalization is not idempotent")
			}
		})
	}
}

func TestEqualIgnoresIndexWithOrder(t *testing.T) {
	a := idxSchema("vector(3)", Index{Name: "i", Method: "hnsw", Parts: []IndexPart{{Column: "c", OpClass: "vector_l2_ops"}},
		With: []KV{{Key: "m", Value: "16"}, {Key: "ef_construction", Value: "64"}}})
	b := idxSchema("vector(3)", Index{Name: "i", Method: "hnsw", Parts: []IndexPart{{Column: "c", OpClass: "vector_l2_ops"}},
		With: []KV{{Key: "ef_construction", Value: "64"}, {Key: "m", Value: "16"}}})
	if !Equal(a, b) {
		t.Fatal("storage parameter order must not matter")
	}
}
