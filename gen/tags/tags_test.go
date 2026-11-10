// SPDX-License-Identifier: Apache-2.0

package tags

import "testing"

func TestParseTags(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantMsg string
		check   func(*testing.T, Tags)
	}{
		{name: "column size notnull", raw: "column:title,size:255,notnull", check: func(t *testing.T, g Tags) {
			if g.Column != "title" || g.Size != 255 || !g.NotNull {
				t.Fatalf("got %+v", g)
			}
		}},
		{name: "dim index method", raw: "dim:1536,index:docs_emb,method:hnsw", check: func(t *testing.T, g Tags) {
			if g.Dim != 1536 || !g.Index || g.IndexName != "docs_emb" || g.Method != "hnsw" {
				t.Fatalf("got %+v", g)
			}
		}},
		{name: "sort nulls", raw: "index,sort:desc,nulls:first", check: func(t *testing.T, g Tags) {
			if !g.Index || g.Sort != "desc" || g.Nulls != "first" {
				t.Fatalf("got %+v", g)
			}
		}},
		{name: "skip", raw: "-", check: func(t *testing.T, g Tags) {
			if !g.Skip {
				t.Fatalf("got %+v", g)
			}
		}},
		{name: "timestamp unit", raw: "created_at:unixmilli", check: func(t *testing.T, g Tags) {
			if !g.CreatedAt || g.TimeUnit != "unixmilli" {
				t.Fatalf("got %+v", g)
			}
		}},
		{name: "view", raw: "view:doc_view,materialized", check: func(t *testing.T, g Tags) {
			if g.View != "doc_view" || !g.Materialized {
				t.Fatalf("got %+v", g)
			}
		}},
		{name: "idgen", raw: "pk,idgen:ulid", check: func(t *testing.T, g Tags) {
			if !g.PK || g.IDGen != "ulid" {
				t.Fatalf("got %+v", g)
			}
		}},
		{name: "dbdefault", raw: "pk,default:now(),dbdefault", check: func(t *testing.T, g Tags) {
			if !g.DBDefault || !g.PK {
				t.Fatalf("got %+v", g)
			}
		}},
		{name: "dbdefault with value", raw: "dbdefault:yes", wantMsg: "dbdefault does not take a value"},
		{name: "idgen without value", raw: "idgen", wantMsg: "idgen requires a value"},
		{name: "unknown key", raw: "bogus", wantMsg: `unknown tag option "bogus"`},
		{name: "retired vector key", raw: "vector:3", wantMsg: `unknown tag option "vector"`},
		{name: "bad integer", raw: "size:abc", wantMsg: "size must be an integer"},
		{name: "valued key without value", raw: "dim", wantMsg: "dim requires a value"},
		{name: "comma in value", raw: "default:now(),x", wantMsg: `unknown tag option "x"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, probs := Parse(tc.raw)
			if tc.wantMsg != "" {
				if len(probs) == 0 || probs[0].Msg != tc.wantMsg {
					t.Fatalf("want problem %q, got %+v", tc.wantMsg, probs)
				}
				return
			}
			if len(probs) != 0 {
				t.Fatalf("unexpected problems %+v", probs)
			}
			tc.check(t, got)
		})
	}
}

func TestTagGrammar(t *testing.T) {
	// every key in the S-70 vocabulary must be accepted by the grammar
	for _, raw := range []string{
		"pk", "identity", "serial", "version", "notnull", "nullable", "read_only", "create_only",
		"immutable", "sensitive", "embedded", "no_constraint", "belongs_to", "has_one",
		"has_many", "many2many", "created_at", "updated_at", "column:c", "type:citext",
		"collate:C", "default:now()", "check:x > 0", "comment:hello", "enum:e", "table:t",
		"schema:s", "view:v,materialized", "prefix:p_", "fk:AuthorID", "fk_name:t_a_fkey1", "join:document_tags", "rel:has_many", "polytype:owner_type", "polyvalue:document",
		"on_delete:cascade", "on_update:cascade", "soft_delete:timestamp", "method:gin",
		"size:1", "precision:10", "scale:2", "dim:3", "index", "unique", "sort:asc", "nulls:last", "idgen:cuid", "dbdefault",
	} {
		if _, probs := Parse(raw); len(probs) != 0 {
			t.Fatalf("%q rejected: %+v", raw, probs)
		}
	}
}
