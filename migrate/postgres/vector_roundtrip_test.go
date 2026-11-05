// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"fmt"
	"strings"
	"testing"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/schema"
)

func vectorSchema() *schema.Schema {
	return &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}},
		Tables: []schema.Table{
			{
				Name:       "docs",
				PrimaryKey: []string{"id"},
				Columns: []schema.Column{
					{Name: "id", Type: "bigint", Identity: "by_default"},
					{Name: "title", Type: "text", Nullable: true},
					{Name: "emb", Type: "vector(1536)"},
					{Name: "half", Type: "halfvec(3072)"},
					{Name: "sp", Type: "sparsevec(1000)"},
					{Name: "bits", Type: "bit(64)"},
				},
				Indexes: []schema.Index{{
					Name:   "docs_emb_hnsw",
					Method: "hnsw",
					Parts:  []schema.IndexPart{{Column: "emb", OpClass: "vector_cosine_ops"}},
					With:   []schema.KV{{Key: "m", Value: "16"}, {Key: "ef_construction", Value: "64"}},
				}},
			},
			{
				Name:       "docs_ivf",
				PrimaryKey: []string{"id"},
				Columns: []schema.Column{
					{Name: "id", Type: "bigint"},
					{Name: "half", Type: "halfvec(64)"},
				},
				Indexes: []schema.Index{{
					Name:   "docs_half_ivf",
					Method: "ivfflat",
					Parts:  []schema.IndexPart{{Column: "half", OpClass: "halfvec_l2_ops"}},
					With:   []schema.KV{{Key: "lists", Value: "100"}},
				}},
			},
		},
	}
}

// TestVectorMigrationRoundTrip applies the vector schema through Diff,
// OrderWithViews, Render and exec (not through the engine), inspects it back,
// requires schema.Equal and requires an empty second diff.
func TestVectorMigrationRoundTrip(t *testing.T) {
	db := testopen.Postgres(t)
	got := roundTrip(t, db, vectorSchema())
	docs, ok := got.Table("docs")
	if !ok {
		t.Fatal("docs missing after round trip")
	}
	wantTypes := map[string]string{"emb": "vector(1536)", "half": "halfvec(3072)", "sp": "sparsevec(1000)", "bits": "bit(64)"}
	for name, typ := range wantTypes {
		c, ok := docs.Column(name)
		if !ok || c.Type != typ {
			t.Errorf("column %s = %+v, want type %s", name, c, typ)
		}
	}
	if len(docs.Indexes) != 1 || docs.Indexes[0].Method != "hnsw" {
		t.Fatalf("hnsw index lost: %+v", docs.Indexes)
	}
}

// TestVectorIndexMatrixRoundTrip creates one table per column type and per
// index method and operator class that pgvector accepts, then round-trips
// all of them in one schema.
func TestVectorIndexMatrixRoundTrip(t *testing.T) {
	type combo struct {
		typ, method, opclass string
		with                 []schema.KV
	}
	hnswWith := []schema.KV{{Key: "ef_construction", Value: "64"}, {Key: "m", Value: "16"}}
	ivfWith := []schema.KV{{Key: "lists", Value: "10"}}
	var combos []combo
	for _, f := range []struct{ typ, prefix string }{{"vector(8)", "vector"}, {"halfvec(8)", "halfvec"}} {
		for _, m := range []string{"l2", "ip", "cosine", "l1"} {
			combos = append(combos, combo{f.typ, "hnsw", f.prefix + "_" + m + "_ops", hnswWith})
		}
		for _, m := range []string{"l2", "ip", "cosine"} {
			combos = append(combos, combo{f.typ, "ivfflat", f.prefix + "_" + m + "_ops", ivfWith})
		}
	}
	for _, m := range []string{"l2", "ip", "cosine", "l1"} {
		combos = append(combos, combo{"sparsevec(8)", "hnsw", "sparsevec_" + m + "_ops", hnswWith})
	}
	for _, m := range []string{"hamming", "jaccard"} {
		combos = append(combos, combo{"bit(8)", "hnsw", "bit_" + m + "_ops", hnswWith})
	}
	combos = append(combos, combo{"bit(8)", "ivfflat", "bit_hamming_ops", ivfWith})

	desired := &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "vector"}}}
	for i, c := range combos {
		base := strings.NewReplacer("(", "_", ")", "").Replace(c.typ)
		name := fmt.Sprintf("m%02d_%s_%s", i, base, c.method)
		desired.Tables = append(desired.Tables, schema.Table{
			Name:    name,
			Columns: []schema.Column{{Name: "id", Type: "bigint"}, {Name: "v", Type: c.typ}},
			Indexes: []schema.Index{{
				Name:   name + "_idx",
				Method: c.method,
				Parts:  []schema.IndexPart{{Column: "v", OpClass: c.opclass}},
				With:   c.with,
			}},
		})
	}
	db := testopen.Postgres(t)
	got := roundTrip(t, db, desired)
	want := schema.Normalized(desired)
	for i, c := range combos {
		tbl := desired.Tables[i]
		wt, _ := want.Table(tbl.Name)
		gt, ok := got.Table(tbl.Name)
		if !ok || len(gt.Indexes) != 1 {
			t.Fatalf("%s: table or index missing", tbl.Name)
		}
		ix := gt.Indexes[0]
		// Inspection reports a default operator class as empty, so compare
		// with the normalized declaration (ivfflat on vector, l2).
		if ix.Method != c.method || ix.Parts[0].OpClass != wt.Indexes[0].Parts[0].OpClass {
			t.Errorf("%s: got %s/%s, want %s/%s", tbl.Name, ix.Method, ix.Parts[0].OpClass, c.method, wt.Indexes[0].Parts[0].OpClass)
		}
	}
}
