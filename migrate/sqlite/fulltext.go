// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"regexp"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/schemautil"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// fts5Hint rides on the statement that creates the FTS5 virtual table, so the
// runner can say that the SQLite build has no FTS5 module instead of passing
// on the driver's "no such module".
const fts5Hint = "requires:fts5"

// ftsIdentRE is the allowlist for the table and key names that CREATE VIRTUAL
// TABLE writes into the content= and content_rowid= options.
var ftsIdentRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ExpandSchema returns a copy of s in which every table with a full-text
// declaration carries what SQLite derives from it: the declaring index is
// renamed to the FTS5 table's name, its description keeps only the options
// SQLite honours, and the three triggers that keep the table in sync are
// added. It is pure and idempotent. A table whose key is not a single INTEGER
// column of a rowid table, a declared column the table lacks, or a name that
// collides with the FTS5 table is ErrValidation naming the table.
func (Dialect) ExpandSchema(s *schema.Schema) (*schema.Schema, error) {
	if s == nil {
		return nil, nil
	}
	out := *s
	out.Tables = make([]schema.Table, len(s.Tables))
	copy(out.Tables, s.Tables)
	out.Triggers = append([]schema.Trigger(nil), s.Triggers...)
	have := map[string]bool{}
	for _, tr := range out.Triggers {
		have[tr.Table+"\x00"+tr.Name] = true
	}
	for i := range out.Tables {
		t := &out.Tables[i]
		ix, ok := t.FullTextIndex()
		if !ok {
			continue
		}
		if err := ix.FullText.Validate(); err != nil {
			return nil, invalid("table %q index %q: %v", t.Name, ix.Name, err)
		}
		for _, c := range ix.FullText.Columns {
			if _, found := t.Column(c); !found {
				return nil, invalid("table %q: full-text column %q is not a column of the table", t.Name, c)
			}
			// FTS5 reserves these names for its hidden columns and the rank
			// option, so a column called one of them cannot be indexed.
			if strings.EqualFold(c, "rank") || strings.EqualFold(c, "rowid") {
				return nil, invalid("table %q: full-text column %q is a name FTS5 reserves", t.Name, c)
			}
		}
		if !ftsIdentRE.MatchString(t.Name) {
			return nil, invalid("table %q: full-text search on SQLite needs a plain identifier as the table name", t.Name)
		}
		if len(t.PrimaryKey) != 1 {
			return nil, invalid("table %q: full-text search on SQLite needs a primary key of one INTEGER column, it has %d key columns", t.Name, len(t.PrimaryKey))
		}
		if t.WithoutRowID {
			return nil, invalid("table %q: full-text search on SQLite needs a rowid table, it is WITHOUT ROWID", t.Name)
		}
		pk := t.PrimaryKey[0]
		if !ftsIdentRE.MatchString(pk) {
			return nil, invalid("table %q: full-text search on SQLite needs a plain identifier as the key column, got %q", t.Name, pk)
		}
		pkCol, found := t.Column(pk)
		if !found || schema.NormalizeType(dialect.SQLite, pkCol.Type) != "INTEGER" {
			return nil, invalid("table %q: full-text search on SQLite needs an INTEGER primary key, column %q is not one", t.Name, pk)
		}
		fts := schemautil.FTS5Table(t.Name)
		for _, other := range out.Tables {
			if other.Name == fts {
				return nil, invalid("table %q: the FTS5 table name %q is already a table", t.Name, fts)
			}
		}
		idx := make([]schema.Index, len(t.Indexes))
		copy(idx, t.Indexes)
		for j := range idx {
			declared := idx[j].Method == "fulltext" && idx[j].FullText != nil
			if idx[j].Name == fts && !declared {
				return nil, invalid("table %q: the index %q collides with the FTS5 table name", t.Name, fts)
			}
			if declared {
				ft := idx[j].FullText.ForDialect(dialect.SQLite)
				idx[j].Name = fts
				idx[j].FullText = &ft
			}
		}
		t.Indexes = idx
		for _, tr := range ftsTriggers(t.Name, ix.FullText.ForDialect(dialect.SQLite), pk) {
			if have[tr.Table+"\x00"+tr.Name] {
				continue
			}
			have[tr.Table+"\x00"+tr.Name] = true
			out.Triggers = append(out.Triggers, tr)
		}
	}
	return &out, nil
}

// createFTS5SQL renders the CREATE VIRTUAL TABLE of the external-content FTS5
// table for ix over table, whose rowid alias is the column pk. Every
// identifier is quoted; the names that go into the content= and
// content_rowid= options are single-quoted after the plain-identifier check.
func createFTS5SQL(table string, ix schema.Index, pk string) (string, error) {
	ft := ix.FullText
	if ft == nil {
		return "", invalid("index %q has no full-text description", ix.Name)
	}
	if err := ft.Validate(); err != nil {
		return "", invalid("table %q index %q: %v", table, ix.Name, err)
	}
	if !ftsIdentRE.MatchString(table) || !ftsIdentRE.MatchString(pk) {
		return "", invalid("table %q: full-text search on SQLite needs plain identifiers for the table and its key, got %q and %q", table, table, pk)
	}
	sql := "CREATE VIRTUAL TABLE " + ident(schemautil.FTS5Table(table)) + " USING fts5(" + identList(ft.Columns) +
		", content='" + table + "', content_rowid='" + pk + "'"
	if ft.Tokenizer != "" {
		sql += ", tokenize='" + ft.Tokenizer + "'"
	}
	return sql + ")", nil
}

// rebuildFTS5SQL fills a new FTS5 table from its content table.
func rebuildFTS5SQL(table string) string {
	fts := ident(schemautil.FTS5Table(table))
	return "INSERT INTO " + fts + "(" + fts + ") VALUES('rebuild')"
}

// dropFTS5SQL drops the FTS5 table fts; its shadow tables go with it.
func dropFTS5SQL(fts string, ifExists bool) string {
	sql := "DROP TABLE "
	if ifExists {
		sql += "IF EXISTS "
	}
	return sql + ident(fts)
}

// ftsTriggers returns the three triggers that keep the FTS5 table of table in
// step with it. The update trigger issues the 'delete' command before the
// insert: the reverse order corrupts the index. The text is written once, here,
// and compared textually on both sides of a diff.
func ftsTriggers(table string, ft schema.FullText, pk string) []schema.Trigger {
	fts := schemautil.FTS5Table(table)
	cols := identList(ft.Columns)
	prefix := func(p string) string {
		q := make([]string, len(ft.Columns))
		for i, c := range ft.Columns {
			q[i] = p + "." + ident(c)
		}
		return strings.Join(q, ", ")
	}
	ins := "  INSERT INTO " + ident(fts) + "(rowid, " + cols + ") VALUES (new." + ident(pk) + ", " + prefix("new") + ");"
	del := "  INSERT INTO " + ident(fts) + "(" + ident(fts) + ", rowid, " + cols + ") VALUES ('delete', old." + ident(pk) + ", " + prefix("old") + ");"
	head := func(suffix, event string) string {
		return "CREATE TRIGGER " + ident(fts+suffix) + " AFTER " + event + " ON " + ident(table) + " BEGIN\n"
	}
	mk := func(suffix, event, body string) schema.Trigger {
		return schema.Trigger{Name: fts + suffix, Table: table, Definition: head(suffix, event) + body + "\nEND"}
	}
	return []schema.Trigger{
		mk("_ai", "INSERT", ins),
		mk("_ad", "DELETE", del),
		mk("_au", "UPDATE", del+"\n"+ins),
	}
}

// ftsIndexChange returns the full-text index a change creates or drops.
func ftsIndexChange(c migrate.Change) (schema.Index, bool) {
	var payload any
	switch c.Kind {
	case migrate.CreateIndex:
		payload = c.After
	case migrate.DropIndex:
		payload = c.Before
	default:
		return schema.Index{}, false
	}
	ix, err := get[schema.Index](payload)
	if err != nil || ix.FullText == nil {
		return schema.Index{}, false
	}
	return ix, true
}

// isFTSChange reports whether c creates or drops a full-text index.
func isFTSChange(c migrate.Change) bool {
	_, ok := ftsIndexChange(c)
	return ok
}

// renderFTSChange renders the creation or drop of the FTS5 table.
func (r *renderer) renderFTSChange(c migrate.Change, ix schema.Index) ([]migrate.Statement, error) {
	if c.Kind == migrate.DropIndex {
		// The index name is the FTS5 table's name. After a table rename it is
		// the name the old table had, while c.Table is already the new one.
		fts := c.Name
		if fts == "" {
			fts = schemautil.FTS5Table(c.Table)
		}
		return one(dropFTS5SQL(fts, c.IfExists)), nil
	}
	if r.to == nil {
		return nil, invalid("index %q is a full-text declaration; render it through RenderSchema", ix.Name)
	}
	t, ok := r.to.Table(c.Table)
	if !ok || len(t.PrimaryKey) != 1 {
		return nil, invalid("table %q: full-text search on SQLite needs a primary key of one INTEGER column", c.Table)
	}
	sql, err := createFTS5SQL(c.Table, ix, t.PrimaryKey[0])
	if err != nil {
		return nil, err
	}
	return []migrate.Statement{{SQL: sql, Hint: fts5Hint}, {SQL: rebuildFTS5SQL(c.Table)}}, nil
}
