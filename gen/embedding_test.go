// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func loadEmbedFrom(t *testing.T) *Graph {
	t.Helper()
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./embedfrom"}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("load diagnostics: %v", diags)
	}
	return g
}

// Catches: the hash column missing from the table, placed away from its
// vector, or declared NOT NULL (a row that was never embedded has no hash).
func TestEmbedFromAddsHashColumn(t *testing.T) {
	g := loadEmbedFrom(t)
	tbl, ok := g.Schema.Table("docs")
	if !ok {
		t.Fatal("docs table missing")
	}
	var names []string
	for _, c := range tbl.Columns {
		names = append(names, c.Name)
	}
	if got, want := strings.Join(names, ","), "id,title,body,summary,embedding,embedding_hash"; got != want {
		t.Fatalf("columns = %s, want %s", got, want)
	}
	h := mustCol(t, g.Schema, "docs", "embedding_hash")
	if h.Type != "varchar(64)" || !h.Nullable {
		t.Fatalf("hash column = %+v, want a nullable varchar(64)", *h)
	}
}

// Catches: the declaration not reaching the generated Meta, or its indexes
// pointing at the wrong columns (the vector, then the sources in tag order,
// whether a source is named by column or by Go name).
func TestEmbedFromEmitsMetaDeclaration(t *testing.T) {
	g := loadEmbedFrom(t)
	src, diags := emitMeta(g.Models, g)
	if len(diags) != 0 {
		t.Fatalf("emit diagnostics: %v", diags)
	}
	want := `{Vector: 4, Dim: 3, HashColumn: "embedding_hash", Sources: []int{1, 2, 3}},`
	if !strings.Contains(string(src), "Embeddings: []momo.EmbeddingMeta{") || !strings.Contains(string(src), want) {
		t.Fatalf("meta does not declare the embedding %s:\n%s", want, src)
	}
}

// Catches: a model without embed_from getting an Embeddings field, which would
// change every generated file.
func TestEmbeddingsLiteralOmittedWithoutTag(t *testing.T) {
	m := mdl("Plain", "plain", fld("ID", "int64", "pk"), fld("Emb", "*vector.Vector", "dim:3"))
	if got := embeddingsLiteral(m); got != "" {
		t.Fatalf("literal = %q, want none", got)
	}
}

// Catches: each way an embed_from declaration is wrong being accepted, or a
// dialect that cannot keep embeddings in sync being generated for.
func TestEmbedFromDiagnostics(t *testing.T) {
	model := func(tag string, extra ...*Field) *Model {
		fields := []*Field{
			fld("ID", "int64", "pk"),
			fld("Title", "string", ""),
			fld("Count", "int64", ""),
			fld("Emb", "*vector.Vector", tag),
		}
		return mdl("Doc", "docs", append(fields, extra...)...)
	}
	cases := []struct {
		name string
		m    *Model
		cfg  Config
		want string
	}{
		{"not nullable", mdl("Doc", "docs", fld("ID", "int64", "pk"), fld("Title", "string", ""), fld("Emb", "vector.Vector", "dim:3,embed_from:title")), pg(), "embed_from needs a *vector.Vector field"},
		{"unknown source", model("dim:3,embed_from:nope"), pg(), `"nope" is not a column or field of Doc`},
		{"empty source", model("dim:3,embed_from:title|"), pg(), "empty source name"},
		{"not a string", model("dim:3,embed_from:count"), pg(), `source "count" must be a string field`},
		{"twice", model("dim:3,embed_from:title|Title"), pg(), `names source "Title" twice`},
		{"itself", model("dim:3,embed_from:emb"), pg(), `source "emb" is an embedding itself`},
		{"hash column taken", model("dim:3,embed_from:title", fld("EmbHash", "string", "")), pg(), `hash column "emb_hash", which is already a column`},
		{"mysql", model("dim:3,embed_from:title"), Config{Dialects: []dialect.Name{dialect.MySQL}}, "embed_from is not supported on mysql"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, diags := build(t, c.cfg, c.m)
			if got := sdiagsString(diags); !strings.Contains(got, c.want) {
				t.Fatalf("diagnostics do not contain %q:\n%s", c.want, got)
			}
		})
	}

	view := model("dim:3,embed_from:title")
	view.IsView = true
	_, diags := build(t, pg(), view)
	if got := sdiagsString(diags); !strings.Contains(got, "not valid on a view model") {
		t.Fatalf("view diagnostics:\n%s", got)
	}
}
