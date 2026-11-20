// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"slices"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/schemautil"
	"github.com/azrtydxb/momo/schema"
)

// prepareFullText rewrites the inspected full-text indexes of m's table into
// the declaration the models hold, which is also what the snapshot records:
// the key parts are the source columns, the generated tsvector column of a
// PostgreSQL stored-form declaration is not a field (the expansion derives it),
// and on SQLite the index is named like the tag names it and the sync triggers
// are left out (the expansion derives them).
func (g *generator) prepareFullText(m *model) {
	t := m.t
	for i := range t.Indexes {
		ix := &t.Indexes[i]
		if ix.Method != "fulltext" || ix.FullText == nil {
			continue
		}
		ft := ix.FullText
		if g.o.Dialect == dialect.Postgres && !ft.Expression {
			name := schemautil.TSVectorColumn(ix.Name)
			t.Columns = slices.DeleteFunc(t.Columns, func(c schema.Column) bool { return c.Name == name && c.Generated != "" })
		}
		ix.Parts = make([]schema.IndexPart, len(ft.Columns))
		for j, c := range ft.Columns {
			ix.Parts[j] = schema.IndexPart{Column: c}
		}
		if g.o.Dialect != dialect.SQLite {
			continue
		}
		fts := schemautil.FTS5Table(t.Name)
		if ix.Name == fts {
			want := t.Name + "_search"
			taken := false
			for _, o := range t.Indexes {
				taken = taken || o.Name == want
			}
			if !taken {
				ix.Name = want
			}
		}
		sync := map[string]bool{fts + "_ai": true, fts + "_ad": true, fts + "_au": true}
		g.clean.Triggers = slices.DeleteFunc(g.clean.Triggers, func(x schema.Trigger) bool {
			return x.Schema == t.Schema && x.Table == t.Name && sync[x.Name]
		})
	}
}

// fullText declares a full-text index through the search tag when the tag can
// say all of it, and through c.FullText otherwise. It reports whether the
// models can declare the index.
func (g *generator) fullText(m *model, ix schema.Index) bool {
	t := m.t
	obj := t.Name + " index " + ix.Name
	ft := *ix.FullText
	drop := func(why string) bool {
		g.add(SevWarn, obj, "%s: the models cannot declare this index, so it is left out of the models and the snapshot and the baseline will report it", why)
		return false
	}
	refs, ok := m.refs(ft.Columns)
	if !ok {
		return drop("it uses a column the models do not have")
	}
	for _, c := range ft.Columns {
		col := colByName(t, c)
		if col == nil || !g.textColumn(col.Type) {
			return drop("a full-text column is not a text column")
		}
	}
	if g.fullTextTag(m, ix, ft) {
		return true
	}
	s := "c.FullText(" + strconv.Quote(ix.Name) + ", " + strings.Join(refs, ", ") + ")"
	if ft.Config != "" {
		s += ".Config(" + strconv.Quote(ft.Config) + ")"
	}
	if len(ft.Weights) > 0 {
		s += ".Weights(" + quoteAll(ft.Weights) + ")"
	}
	if ft.Tokenizer != "" {
		s += ".Tokenizer(" + strconv.Quote(ft.Tokenizer) + ")"
	}
	if ft.Parser != "" {
		s += ".Parser(" + strconv.Quote(ft.Parser) + ")"
	}
	if ft.Expression {
		s += ".Expression()"
	}
	m.stmts = append(m.stmts, s)
	return true
}

// fullTextTag writes the declaration as the search tag of its fields. The tag
// carries no option, and PostgreSQL needs a configuration, so it serves only
// a plain declaration on the other dialects, named <table>_search, whose
// columns are in field order.
func (g *generator) fullTextTag(m *model, ix schema.Index, ft schema.FullText) bool {
	if g.o.Dialect == dialect.Postgres || ft.Config != "" || len(ft.Weights) > 0 || ft.Tokenizer != "" || ft.Parser != "" || ft.Expression {
		return false
	}
	if ix.Name != m.t.Name+"_search" {
		return false
	}
	pos := -1
	var fields []*field
	for _, c := range ft.Columns {
		f := m.byCol[c]
		at := slices.Index(m.fields, f)
		if f == nil || at <= pos {
			return false
		}
		pos = at
		fields = append(fields, f)
	}
	for _, f := range fields {
		f.opts = append(f.opts, "search")
	}
	return true
}

// textColumn reports whether a column type is one the search tag and
// c.FullText accept.
func (g *generator) textColumn(typ string) bool {
	base := strings.ToLower(schema.NormalizeType(g.o.Dialect, typ))
	if i := strings.IndexAny(base, "( "); i >= 0 {
		base = base[:i]
	}
	switch base {
	case "text", "varchar", "char", "character", "tinytext", "mediumtext", "longtext", "citext":
		return true
	}
	return false
}
