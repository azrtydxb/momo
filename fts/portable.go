// SPDX-License-Identifier: Apache-2.0

package fts

import (
	"slices"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/schemautil"
)

// SnippetOpts configures Snippet.
type SnippetOpts struct {
	// Column is the text column to excerpt. On a target from Over it must be
	// one of the declared columns.
	Column momo.Column[string]
	// Open and Close mark a match. They default to "<b>" and "</b>".
	Open  string
	Close string
	// Ellipsis marks the cut ends of an excerpt. SQLite only; default "...".
	Ellipsis string
	// Tokens is the excerpt length in words, 1 to 64. SQLite only; default 16.
	Tokens int
}

const (
	defaultOpen     = "<b>"
	defaultClose    = "</b>"
	defaultEllipsis = "..."
	defaultTokens   = 16
	maxTokens       = 64
)

// dispatch runs the renderer of the dialect being built. A nil renderer, or a
// dialect without one, fails the build with ErrUnsupported.
func dispatch(b *momo.Builder, what string, pg, sqlite, mysql func()) {
	switch b.Dialect().Name {
	case dialect.Postgres:
		pg()
	case dialect.SQLite:
		sqlite()
	case dialect.MySQL:
		if mysql != nil {
			mysql()
			return
		}
		b.Unsupported("full-text search (" + what + ")")
	default:
		b.Unsupported("full-text search (" + what + ")")
	}
}

// Search is the predicate that matches the rows whose searchable columns
// match q: ts @@ tsquery on PostgreSQL, a MATCH on the FTS5 table on SQLite,
// MATCH AGAINST in boolean mode on MySQL and MariaDB. The query text is a
// bound parameter. A query without a positive word
// renders as a false predicate and sends no MATCH.
func Search(t Target, q Query) momo.Predicate {
	return momo.PredicateFunc(func(b *momo.Builder) {
		if !t.check(b) {
			return
		}
		dispatch(b, "Search", func() { pgSearch(b, t, q) }, func() { sqliteSearch(b, t, q) }, func() { mysqlSearch(b, t, q) })
	})
}

// Score is the relevance of a row for q: higher is better on every dialect
// (ts_rank_cd on PostgreSQL, the negated bm25 on SQLite, the MATCH relevance
// on MySQL and MariaDB). Scores are
// comparable only within one dialect and one query; use them to order, never
// against a threshold. A query without a positive word scores 0.
func Score(t Target, q Query) Expr[float64] {
	return newPortableExpr[float64]("Score", func(b *momo.Builder) {
		if !t.check(b) {
			return
		}
		dispatch(b, "Score", func() { pgScore(b, t, q) }, func() { sqliteScore(b, t, q) }, func() { mysqlScore(b, t, q) })
	})
}

// Snippet is the excerpt of o.Column with the matched words marked
// (ts_headline on PostgreSQL, snippet() on SQLite). It is NULL for a query
// without a positive word. MySQL and MariaDB have no excerpt function and fail
// the build with ErrUnsupported.
func Snippet(t Target, q Query, o SnippetOpts) Expr[string] {
	return newPortableExpr[string]("Snippet", func(b *momo.Builder) {
		if !t.check(b) {
			return
		}
		if o.Column.ColumnName() == "" {
			b.SetErr(validationError("snippet needs the text column to excerpt (SnippetOpts.Column)"))
			return
		}
		if t.kind == kindDeclared && !slices.Contains(t.ft.Columns, o.Column.ColumnName()) {
			b.SetErr(validationError("snippet column %q is not a declared full-text column of table %s", o.Column.ColumnName(), t.tbl.Name))
			return
		}
		dispatch(b, "Snippet", func() { pgSnippet(b, t, q, o) }, func() { sqliteSnippet(b, t, q, o) }, nil)
	})
}

// ---------------------------------------------------------------- PostgreSQL

// pgCall is one validated PostgreSQL rendering.
type pgCall struct {
	t      Target
	q      Query
	pq     pgQuery
	fl     uint16
	config string // declared configuration; empty for an OverVector target
}

func pgPrepare(b *momo.Builder, t Target, q Query, extra ...momo.Expr) (pgCall, bool) {
	c := pgCall{t: t, q: q, pq: q.pg()}
	if t.kind == kindDeclared {
		cfg, err := t.declaredConfig(q)
		if err != nil {
			b.SetErr(err)
			return c, false
		}
		c.config = cfg
	}
	c.fl = sensFlags(b, append(t.exprs(), extra...)...)
	return c, true
}

// writeConfig writes the regconfig: a literal pinned by the declaration, or a
// bound parameter for a hand-managed vector.
func (c pgCall) writeConfig(b *momo.Builder, what string) {
	if c.t.kind == kindDeclared {
		b.WriteString("CAST('" + c.config + "' AS regconfig)")
		return
	}
	cfg := c.t.config
	if c.q.config != "" {
		cfg = c.q.config
	}
	writeRegconfig(b, what, cfg, c.fl)
}

// writeVector writes the tsvector operand.
func (c pgCall) writeVector(b *momo.Builder) {
	t := c.t
	switch {
	case t.kind == kindVector:
		b.ExprFor(c.fl, t.vec)
	case t.ft.Expression:
		b.WriteString("to_tsvector(")
		c.writeConfig(b, "to_tsvector")
		b.WriteString(", ")
		for i, col := range t.cols {
			if i > 0 {
				b.WriteString(" || ' ' || ")
			}
			b.WriteString("COALESCE(")
			b.ExprFor(c.fl, col)
			b.WriteString(", '')")
		}
		b.WriteString(")")
	default:
		b.QualifiedIdent(t.table, schemautil.TSVectorColumn(t.indexName))
	}
}

// writeQuery writes the tsquery call with the query text bound.
func (c pgCall) writeQuery(b *momo.Builder) {
	b.WriteString(c.pq.fn + "(")
	c.writeConfig(b, c.pq.fn)
	b.WriteString(", ")
	b.ArgFor(c.fl, c.pq.arg)
	b.WriteString(")")
}

func pgSearch(b *momo.Builder, t Target, q Query) {
	c, ok := pgPrepare(b, t, q)
	if !ok {
		return
	}
	if c.pq.empty {
		b.WriteString("1 = 0")
		return
	}
	c.writeVector(b)
	b.WriteString(" @@ ")
	c.writeQuery(b)
}

func pgScore(b *momo.Builder, t Target, q Query) {
	c, ok := pgPrepare(b, t, q)
	if !ok {
		return
	}
	if c.pq.empty {
		b.WriteString("0")
		return
	}
	b.WriteString("ts_rank_cd(")
	c.writeVector(b)
	b.WriteString(", ")
	c.writeQuery(b)
	b.WriteString(")")
}

// headlineOption writes name=value, double-quoting the value when it holds a
// character that ends an option or needs the quotes.
func headlineOption(name, value string) string {
	if value == "" || strings.ContainsAny(value, " \t\r\n,\"=\\") {
		value = `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return name + "=" + value
}

func pgSnippet(b *momo.Builder, t Target, q Query, o SnippetOpts) {
	c, ok := pgPrepare(b, t, q, o.Column)
	if !ok {
		return
	}
	if c.pq.empty {
		b.WriteString("NULL")
		return
	}
	open, closing := o.Open, o.Close
	if open == "" {
		open = defaultOpen
	}
	if closing == "" {
		closing = defaultClose
	}
	b.WriteString("ts_headline(")
	c.writeConfig(b, "ts_headline")
	b.WriteString(", ")
	b.ExprFor(c.fl, o.Column)
	b.WriteString(", ")
	c.writeQuery(b)
	b.WriteString(", ")
	b.ArgFor(c.fl, headlineOption("StartSel", open)+", "+headlineOption("StopSel", closing))
	b.WriteString(")")
}

// -------------------------------------------------------------------- SQLite

// sqliteCall is one validated SQLite rendering.
type sqliteCall struct {
	t     Target
	match string
	empty bool
	fts   string // the FTS5 table
	pk    string // the integer primary key column
	fl    uint16
}

func sqlitePrepare(b *momo.Builder, t Target, q Query, extra ...momo.Expr) (sqliteCall, bool) {
	c := sqliteCall{t: t}
	if t.kind != kindDeclared {
		b.Unsupported("full-text search (a tsvector column target)")
		return c, false
	}
	if len(t.tbl.PrimaryKey) != 1 {
		b.SetErr(validationError("table %s needs a single integer primary key for FTS5 search", t.tbl.Name))
		return c, false
	}
	c.pk = t.tbl.PrimaryKey[0]
	if col, ok := t.tbl.Column(c.pk); ok && !strings.Contains(strings.ToLower(col.Type), "int") {
		b.SetErr(validationError("table %s: primary key %s is %s; FTS5 search needs an integer key", t.tbl.Name, c.pk, col.Type))
		return c, false
	}
	c.fts = schemautil.FTS5Table(t.tbl.Name)
	c.match, c.empty = q.fts5()
	c.fl = sensFlags(b, append(t.exprs(), extra...)...)
	return c, true
}

func (c sqliteCall) writeMatch(b *momo.Builder) {
	b.Ident(c.fts)
	b.WriteString(" MATCH ")
	b.ArgFor(c.fl, c.match)
}

// writeCorrelation writes the join of the FTS5 row to the outer row.
func (c sqliteCall) writeCorrelation(b *momo.Builder) {
	b.Ident(c.fts)
	b.WriteString(".rowid = ")
	b.QualifiedIdent(c.t.table, c.pk)
}

func sqliteSearch(b *momo.Builder, t Target, q Query) {
	c, ok := sqlitePrepare(b, t, q)
	if !ok {
		return
	}
	if c.empty {
		b.WriteString("1 = 0")
		return
	}
	b.QualifiedIdent(t.table, c.pk)
	b.WriteString(" IN (SELECT rowid FROM ")
	b.Ident(c.fts)
	b.WriteString(" WHERE ")
	c.writeMatch(b)
	b.WriteString(")")
}

// bm25Weight maps a declared weight letter to its FTS5 column weight.
var bm25Weight = map[string]string{"A": "1", "B": "0.4", "C": "0.2", "D": "0.1"}

func sqliteScore(b *momo.Builder, t Target, q Query) {
	c, ok := sqlitePrepare(b, t, q)
	if !ok {
		return
	}
	if c.empty {
		b.WriteString("0")
		return
	}
	// A row the query does not match has no FTS5 row, so the subquery is NULL;
	// COALESCE gives it the score 0 that PostgreSQL gives it.
	b.WriteString("COALESCE((SELECT -bm25(")
	b.Ident(c.fts)
	for _, w := range t.ft.Weights {
		b.WriteString(", " + bm25Weight[w])
	}
	b.WriteString(") FROM ")
	b.Ident(c.fts)
	b.WriteString(" WHERE ")
	c.writeMatch(b)
	b.WriteString(" AND ")
	c.writeCorrelation(b)
	b.WriteString("), 0)")
}

func sqliteSnippet(b *momo.Builder, t Target, q Query, o SnippetOpts) {
	tokens := o.Tokens
	if tokens == 0 {
		tokens = defaultTokens
	}
	if tokens < 1 || tokens > maxTokens {
		b.SetErr(validationError("snippet tokens %d outside 1..%d", o.Tokens, maxTokens))
		return
	}
	c, ok := sqlitePrepare(b, t, q, o.Column)
	if !ok {
		return
	}
	if c.empty {
		b.WriteString("NULL")
		return
	}
	open, closing, ellipsis := o.Open, o.Close, o.Ellipsis
	if open == "" {
		open = defaultOpen
	}
	if closing == "" {
		closing = defaultClose
	}
	if ellipsis == "" {
		ellipsis = defaultEllipsis
	}
	b.WriteString("(SELECT snippet(")
	b.Ident(c.fts)
	b.WriteString(", " + strconv.Itoa(slices.Index(t.ft.Columns, o.Column.ColumnName())) + ", ")
	b.ArgFor(c.fl, open)
	b.WriteString(", ")
	b.ArgFor(c.fl, closing)
	b.WriteString(", ")
	b.ArgFor(c.fl, ellipsis)
	b.WriteString(", " + strconv.Itoa(tokens) + ") FROM ")
	b.Ident(c.fts)
	b.WriteString(" WHERE ")
	c.writeMatch(b)
	b.WriteString(" AND ")
	c.writeCorrelation(b)
	b.WriteString(")")
}
