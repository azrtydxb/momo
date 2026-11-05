// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func populated() *Schema {
	return &Schema{
		FormatVersion: FormatVersion,
		Dialect:       dialect.Postgres,
		Schemas:       []SchemaNamespace{{Name: "audit"}},
		Extensions:    []Extension{{Name: "vector", Version: "0.8.7"}},
		Enums:         []Enum{{Name: "status", Values: []string{"open", "closed"}}},
		Tables: []Table{{
			Name: "documents",
			Columns: []Column{
				{Name: "id", Type: "bigint", Identity: "by_default"},
				{Name: "embedding", Type: "vector(3)", Nullable: true, Default: "NULL"},
				{Name: "gen", Type: "integer", Generated: "(id * 2)"},
				{Name: "st", Type: "status", Enum: "status"},
			},
			PrimaryKey:   []string{"id"},
			Uniques:      []Unique{{Name: "documents_u", Columns: []string{"st"}}},
			Indexes:      []Index{{Name: "documents_i", Unique: true, Method: "hnsw", Where: "st IS NOT NULL", With: []KV{{Key: "m", Value: "16"}}, Concurrently: true, Parts: []IndexPart{{Column: "embedding", OpClass: "vector_cosine_ops", Desc: true, Nulls: "last"}, {Expr: "lower(st)"}}}},
			ForeignKeys:  []ForeignKey{{Name: "documents_fk", Columns: []string{"id"}, RefTable: "owners", RefColumns: []string{"id"}, OnDelete: "CASCADE", OnUpdate: "NO ACTION"}},
			Checks:       []Check{{Name: "documents_c", Expr: "id > 0"}},
			WithoutRowID: true,
			Schema:       "audit",
			RLS:          &RLS{Enabled: true, Forced: true},
			Partition:    &Partition{Strategy: "range", Key: "RANGE (id)"},
			PartitionOf:  &PartitionOf{Parent: "p", Bound: "FOR VALUES FROM (1) TO (2)"},
		}},
		Views: []View{{Schema: "audit", Name: "v", Materialized: true, Query: "SELECT 1", Depends: []string{"documents"},
			Indexes: []Index{{Name: "v_i", Parts: []IndexPart{{Column: "x"}}}}}},
		Functions: []Function{{Schema: "audit", Name: "f", Args: "a integer", Returns: "integer", Language: "sql", Volatility: "STABLE", Body: "SELECT a"}},
		Triggers: []Trigger{{Schema: "audit", Table: "documents", Name: "tg", Timing: "BEFORE", Events: []string{"INSERT", "UPDATE"},
			ForEachRow: true, When: "(true)", Function: "f()", Args: []string{"x"}, Definition: "CREATE TRIGGER tg"}},
		Policies: []Policy{{Schema: "audit", Table: "documents", Name: "pol", Command: "ALL", Permissive: true,
			Roles: []string{"app"}, Using: "tenant = 1", Check: "tenant = 1"}},
	}
}

func TestSchemaJSONRoundTrip(t *testing.T) {
	s := populated()
	first, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back Schema
	if err := json.Unmarshal(first, &back); err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(&back)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("round trip changed the JSON:\n%s\n%s", first, second)
	}
	if !reflect.DeepEqual(s, &back) {
		t.Fatal("round trip changed the value")
	}
}

// TestSchemaEveryFieldSerialized walks the type tree and fails when a field
// has no json tag, since it would be silently dropped from snapshots.
func TestSchemaEveryFieldSerialized(t *testing.T) {
	var walk func(reflect.Type, string)
	walk = func(rt reflect.Type, path string) {
		for rt.Kind() == reflect.Slice || rt.Kind() == reflect.Ptr {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			tag := f.Tag.Get("json")
			if tag == "" || tag == "-" {
				t.Errorf("%s.%s has no json tag", path, f.Name)
				continue
			}
			walk(f.Type, path+"."+f.Name)
		}
	}
	walk(reflect.TypeOf(Schema{}), "Schema")
}

func TestSchemaFieldSet(t *testing.T) {
	b, err := json.Marshal(populated())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"format_version":1`, `"extensions"`, `"enums"`, `"without_rowid":true`,
		`"schemas"`, `"views"`, `"functions"`, `"triggers"`, `"policies"`, `"rls"`, `"partition"`, `"partition_of"`, `"schema":"audit"`} {
		if !strings.Contains(string(b), key) {
			t.Errorf("snapshot JSON lacks %s", key)
		}
	}
}

func TestColumnCollateCommentJSONRoundTrip(t *testing.T) {
	in := &Schema{FormatVersion: FormatVersion, Dialect: "postgres", Tables: []Table{{
		Name:    "t",
		Columns: []Column{{Name: "c", Type: "text", Collate: "C", Comment: "a \"quoted\" note"}},
	}}}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Schema
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	c := out.Tables[0].Columns[0]
	if c.Collate != "C" || c.Comment != "a \"quoted\" note" {
		t.Fatalf("lost: %+v", c)
	}
	if !Equal(in, &out) {
		t.Fatal("round trip not Equal")
	}
	out.Tables[0].Columns[0].Comment = "other"
	if Equal(in, &out) {
		t.Fatal("Equal ignores Comment")
	}
	out.Tables[0].Columns[0].Comment = in.Tables[0].Columns[0].Comment
	out.Tables[0].Columns[0].Collate = "POSIX"
	if Equal(in, &out) {
		t.Fatal("Equal ignores Collate")
	}
}
