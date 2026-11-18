// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// embeddingHashType is the type of the hash column the generator adds beside an
// embedding: the 64 lowercase hexadecimal characters of a SHA-256 digest.
const embeddingHashType = "varchar(64)"

// embeddingDecl is one embed_from declaration, resolved against its model.
type embeddingDecl struct {
	// vec is the index in Model.Fields of the vector field; sources are the
	// indexes of the source fields, in tag order.
	vec     int
	sources []int
	// dim is the declared dimension of the vector, 0 when it is not declared.
	dim int
	// hash is the name of the hash column the generator adds.
	hash string
}

// embeddingsOf resolves the embed_from tags of m. The result holds the valid
// declarations in field order; each problem is a diagnostic at the tagged
// field. A field with a problem contributes no declaration.
func embeddingsOf(m *Model) ([]embeddingDecl, []Diagnostic) {
	var out []embeddingDecl
	var diags []Diagnostic
	bad := func(f *Field, format string, a ...any) {
		diags = append(diags, Diagnostic{Pos: f.Pos, Msg: "field " + f.GoName + ": " + fmt.Sprintf(format, a...)})
	}
	cols := map[string]bool{}
	for _, f := range m.Fields {
		cols[f.Column] = true
	}
	for i, f := range m.Fields {
		if f.Tags.EmbedFrom == "" {
			continue
		}
		if m.IsView {
			bad(f, "embed_from is not valid on a view model: a view has no column to hold the hash")
			continue
		}
		if f.GoType != "*vector.Vector" {
			bad(f, "embed_from needs a *vector.Vector field: a new row has no embedding yet, so the column must be nullable, and only float32 vectors are embedded")
			continue
		}
		ok := true
		var srcs []int
		seen := map[int]bool{}
		for _, name := range strings.Split(f.Tags.EmbedFrom, "|") {
			name = strings.TrimSpace(name)
			j := embedSourceField(m, name)
			switch {
			case name == "":
				bad(f, "embed_from has an empty source name; separate the sources with |")
				ok = false
			case j < 0:
				bad(f, "embed_from: %q is not a column or field of %s", name, m.GoName)
				ok = false
			case j == i || m.Fields[j].Tags.EmbedFrom != "":
				bad(f, "embed_from: source %q is an embedding itself", name)
				ok = false
			case unwrapNull(embedBaseType(m.Fields[j])) != "string":
				bad(f, "embed_from: source %q must be a string field (string, *string or sql.NullString), but it is %s", name, m.Fields[j].GoType)
				ok = false
			case seen[j]:
				bad(f, "embed_from names source %q twice", name)
				ok = false
			default:
				seen[j] = true
				srcs = append(srcs, j)
			}
		}
		hash := f.Column + "_hash"
		if cols[hash] {
			bad(f, "embed_from adds the hash column %q, which is already a column of table %s; rename that column", hash, m.Table)
			ok = false
		}
		if ok {
			out = append(out, embeddingDecl{vec: i, sources: srcs, dim: vectorDim(f), hash: hash})
		}
	}
	return out, diags
}

// embedBaseType is the Go type of f with named types resolved, as the schema
// builder reads it: BaseType, or for a hand-built field the spelled type
// without its pointer.
func embedBaseType(f *Field) string {
	if f.BaseType != "" {
		return f.BaseType
	}
	return strings.TrimPrefix(f.GoType, "*")
}

// embedSourceField returns the index in m.Fields of the field that name refers
// to: the field with that column name, else the field with that Go name. It
// returns -1 when there is none.
func embedSourceField(m *Model, name string) int {
	if i := slices.IndexFunc(m.Fields, func(f *Field) bool { return f.Column == name }); i >= 0 {
		return i
	}
	return slices.IndexFunc(m.Fields, func(f *Field) bool { return f.GoName == name })
}

// applyEmbeddings adds the hash column of each embedding of m to t, right
// after its vector column, and refuses a dialect that cannot keep embeddings
// in sync. The Go code that computes the hash and writes the embedding lives
// in package momo, which reads the declaration from Meta.Embeddings.
func (b *schemaBuilder) applyEmbeddings(m *Model, t *schema.Table) {
	decls, diags := embeddingsOf(m)
	b.diags = append(b.diags, diags...)
	for _, d := range decls {
		f := m.Fields[d.vec]
		for _, dn := range append(slices.Clone(b.dialects), b.primary) {
			if dn == dialect.MySQL {
				b.diag(f.Pos, fmt.Sprintf("field %s: embed_from is not supported on %s; Momo keeps embeddings in sync on PostgreSQL and SQLite", f.GoName, dn))
				break
			}
		}
		at := slices.IndexFunc(t.Columns, func(c schema.Column) bool { return c.Name == f.Column })
		if at < 0 {
			continue
		}
		t.Columns = slices.Insert(t.Columns, at+1, schema.Column{Name: d.hash, Type: embeddingHashType, Nullable: true})
	}
}

// embeddingsLiteral returns the Embeddings field of the Meta literal of m, or
// "" when m declares none.
func embeddingsLiteral(m *Model) string {
	decls, _ := embeddingsOf(m)
	if len(decls) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Embeddings: []momo.EmbeddingMeta{\n")
	for _, d := range decls {
		srcs := make([]string, len(d.sources))
		for i, s := range d.sources {
			srcs[i] = strconv.Itoa(s)
		}
		fmt.Fprintf(&b, "{Vector: %d, Dim: %d, HashColumn: %s, Sources: []int{%s}},\n", d.vec, d.dim, strconv.Quote(d.hash), strings.Join(srcs, ", "))
	}
	b.WriteString("},\n")
	return b.String()
}
