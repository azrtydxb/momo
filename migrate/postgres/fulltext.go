// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"fmt"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// maxIdentBytes is PostgreSQL's identifier limit (NAMEDATALEN - 1).
const maxIdentBytes = 63

func ftError(table, format string, args ...any) error {
	return &momo.Error{Kind: momo.ErrValidation, Op: "migrate/postgres", Table: table, Err: fmt.Errorf(format, args...)}
}

// ExpandSchema returns a copy of s with the objects a full-text declaration
// implies: for a table with a full-text index, a stored generated tsvector
// column and a GIN index on it, or, for an expression declaration, a GIN index
// on the tsvector expression. It is pure and idempotent.
func (Dialect) ExpandSchema(s *schema.Schema) (*schema.Schema, error) {
	if s == nil {
		return nil, nil
	}
	out := *s
	out.Tables = append([]schema.Table(nil), s.Tables...)
	for i := range out.Tables {
		t := &out.Tables[i]
		if _, ok := t.FullTextIndex(); !ok {
			continue
		}
		t.Columns = append([]schema.Column(nil), t.Columns...)
		t.Indexes = append([]schema.Index(nil), t.Indexes...)
		ix, _ := t.FullTextIndex()
		if err := expandTable(t, ix); err != nil {
			return nil, err
		}
	}
	return &out, nil
}

func expandTable(t *schema.Table, ix *schema.Index) error {
	if err := ix.FullText.Validate(); err != nil {
		return ftError(t.Name, "table %q index %q: %w", t.Name, ix.Name, err)
	}
	ft := ix.FullText.ForDialect(dialect.Postgres)
	if ft.Config == "" {
		return ftError(t.Name, "table %q index %q: a PostgreSQL full-text declaration needs Config", t.Name, ix.Name)
	}
	for _, c := range ft.Columns {
		if _, ok := t.Column(c); !ok {
			return ftError(t.Name, "table %q index %q: full-text column %q is not a column of the table", t.Name, ix.Name, c)
		}
	}
	ix.FullText = &ft
	if ft.Expression {
		ix.Parts = []schema.IndexPart{{Expr: expressionIndexExpr(ft)}}
		return nil
	}
	name := schema.TSVectorColumn(ix.Name)
	if len(name) > maxIdentBytes {
		return ftError(t.Name, "table %q index %q: the generated column %q is %d bytes, over PostgreSQL's %d-byte identifier limit; shorten the index name", t.Name, ix.Name, name, len(name), maxIdentBytes)
	}
	expr := generationExpr(ft)
	if c, ok := t.Column(name); ok {
		if c.Generated != expr {
			return ftError(t.Name, "table %q index %q: the generated column %q collides with an existing column", t.Name, ix.Name, name)
		}
	} else {
		t.Columns = append(t.Columns, schema.Column{Name: name, Type: "tsvector", Nullable: true, Generated: expr})
	}
	ix.Parts = []schema.IndexPart{{Column: name}}
	return nil
}

// dropsWithColumn reports whether ix is a full-text index on a generated
// tsvector column. Dropping it goes together with dropping or recreating that
// column, so the drop is never CONCURRENTLY.
func dropsWithColumn(ix schema.Index) bool {
	return ix.FullText != nil && !ix.FullText.Expression
}

// renderRecreateGenerated changes the generation expression of the generated
// column of a full-text index. PostgreSQL cannot alter such an expression in
// place, so the column is dropped and added again. The change is marked by the
// diff (migrate.Change.FullTextColumn) from the desired schema, so it renders
// the same whether or not the index change shares its plan or its migration:
// the plan drops the index that depends on it first, in the same transaction
// (see dropsWithColumn), and creates it again afterwards, which may be a
// CONCURRENTLY build in a later no-transaction migration.
func renderRecreateGenerated(c migrate.Change) ([]migrate.Statement, error) {
	after, err := get[schema.Column](c.After)
	if err != nil {
		return nil, err
	}
	def, err := columnDef(after)
	if err != nil {
		return nil, err
	}
	pre := "ALTER TABLE " + tableRef(c.Schema, c.Table)
	return []migrate.Statement{
		{SQL: pre + " DROP COLUMN " + ident(after.Name)},
		{SQL: pre + " ADD COLUMN " + def},
	}, nil
}

// expressionIndexExpr is the key of the GIN expression index of an expression
// declaration: one to_tsvector over the source columns joined by a space.
func expressionIndexExpr(ft schema.FullText) string {
	parts := make([]string, len(ft.Columns))
	for i, c := range ft.Columns {
		parts[i] = `coalesce(` + ident(c) + `, '')`
	}
	return "to_tsvector(" + configLiteral(ft.Config) + ", " + strings.Join(parts, " || ' ' || ") + ")"
}

// configLiteral is the configuration name as a quoted literal. The name was
// validated as a plain identifier, so no escaping applies.
func configLiteral(config string) string { return "'" + config + "'" }

// generationExpr is the expression of the generated tsvector column: one
// to_tsvector per source column (NULL read as the empty string), wrapped in
// setweight when weights are declared, concatenated with the || operator.
func generationExpr(ft schema.FullText) string {
	parts := make([]string, len(ft.Columns))
	for i, c := range ft.Columns {
		p := "to_tsvector(" + configLiteral(ft.Config) + ", coalesce(" + ident(c) + ", ''))"
		if len(ft.Weights) == len(ft.Columns) {
			p = "setweight(" + p + ", '" + ft.Weights[i] + "')"
		}
		parts[i] = p
	}
	return strings.Join(parts, " || ")
}
