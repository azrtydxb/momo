// SPDX-License-Identifier: Apache-2.0

package fts

import (
	"strings"

	"github.com/azrtydxb/momo/internal/ftsquery"
)

// Query is the user's search text plus how to read it. The text is always a
// bound parameter, never SQL. Build one with Web, Plain or Raw.
type Query struct {
	text   string
	syntax ftsquery.Syntax
	config string
}

// Web reads text like a search box: "quoted phrases", -exclusions, OR between
// words and a trailing * for a prefix. Punctuation inside a word splits it
// (well-known reads as the phrase "well known"), so no input can raise a syntax
// error in the database. A bare AND, NOT or NEAR is dropped too. A query
// without a positive word matches nothing.
//
// On PostgreSQL a query without a prefix term is bound as given to
// websearch_to_tsquery, which reads it by its own rules: a lowercase or is an
// OR too, and a -x binds to its own OR group. SQLite and a PostgreSQL query
// with a prefix term use the shared parser instead, which reads only an
// uppercase OR and applies every exclusion to the whole query.
func Web(text string) Query { return Query{text: text, syntax: ftsquery.Web} }

// Plain reads text as words that must all match; punctuation is ignored.
func Plain(text string) Query { return Query{text: text, syntax: ftsquery.Plain} }

// Raw passes text to the database in its own query syntax: tsquery text on
// PostgreSQL, an FTS5 query on SQLite. The caller owns its correctness, and a
// malformed text is a database error. The text is still a bound parameter.
func Raw(text string) Query { return Query{text: text, syntax: ftsquery.Raw} }

// Config sets the PostgreSQL text search configuration for this query. On a
// target built with Over it must equal the declared configuration; a
// different name is an ErrValidation build error, because the index could not
// serve the query. On OverVector it replaces the configuration given there.
// SQLite and MySQL fix the configuration in the index and ignore it.
func (q Query) Config(name string) Query {
	q.config = name
	return q
}

// pgQuery is the PostgreSQL tsquery call for a Query.
type pgQuery struct {
	fn    string
	arg   string
	empty bool
}

// pg decides the tsquery function and its argument.
func (q Query) pg() pgQuery {
	switch q.syntax {
	case ftsquery.Raw:
		return pgQuery{fn: "to_tsquery", arg: q.text, empty: strings.TrimSpace(q.text) == ""}
	case ftsquery.Plain:
		return pgQuery{fn: "plainto_tsquery", arg: q.text, empty: ftsquery.Parse(q.text, ftsquery.Plain).Empty()}
	}
	p := ftsquery.Parse(q.text, ftsquery.Web)
	switch {
	case p.Empty():
		return pgQuery{empty: true}
	case p.HasPrefix():
		return pgQuery{fn: "to_tsquery", arg: p.PostgresTSQuery()}
	}
	return pgQuery{fn: "websearch_to_tsquery", arg: q.text}
}

// mysql returns the boolean-mode string and whether the query is empty. A Raw
// query is passed on as given: the caller writes boolean-mode syntax.
func (q Query) mysql() (string, bool) {
	if q.syntax == ftsquery.Raw {
		return q.text, strings.TrimSpace(q.text) == ""
	}
	p := ftsquery.Parse(q.text, q.syntax)
	return p.MySQLBoolean(), p.Empty()
}

// fts5 returns the FTS5 MATCH text and whether the query is empty.
func (q Query) fts5() (string, bool) {
	switch q.syntax {
	case ftsquery.Raw:
		return q.text, strings.TrimSpace(q.text) == ""
	case ftsquery.Plain:
		p := ftsquery.Parse(q.text, ftsquery.Plain)
		return p.FTS5(), p.Empty()
	}
	p := ftsquery.Parse(q.text, ftsquery.Web)
	return p.FTS5(), p.Empty()
}
