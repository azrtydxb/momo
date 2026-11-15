// SPDX-License-Identifier: Apache-2.0

package fts

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// MySQLMode is the search modifier of MATCH ... AGAINST on MySQL and MariaDB.
// It selects a fixed piece of SQL; no input ever becomes part of it.
type MySQLMode uint8

const (
	// NaturalLanguage is IN NATURAL LANGUAGE MODE: the text is read as words,
	// and a word present in half the rows or more counts for nothing.
	NaturalLanguage MySQLMode = iota
	// Boolean is IN BOOLEAN MODE: the text carries the operators + - > < ( ) ~ *
	// and "phrases". Search and Score use it, with a text they build.
	Boolean
	// NaturalLanguageWithExpansion is IN NATURAL LANGUAGE MODE WITH QUERY
	// EXPANSION: a second pass adds the words of the best matches.
	NaturalLanguageWithExpansion
)

// modifier is the SQL text of the mode, or false for a value that is no mode.
func (m MySQLMode) modifier() (string, bool) {
	switch m {
	case NaturalLanguage:
		return "IN NATURAL LANGUAGE MODE", true
	case Boolean:
		return "IN BOOLEAN MODE", true
	case NaturalLanguageWithExpansion:
		return "IN NATURAL LANGUAGE MODE WITH QUERY EXPANSION", true
	}
	return "", false
}

// writeMySQLMatch writes MATCH(c1, c2) AGAINST (? <mode>) with the text bound.
// The columns must be exactly the columns of one FULLTEXT index, in its order;
// a server without such an index answers with an error that becomes
// ErrValidation.
func writeMySQLMatch(b *momo.Builder, cols []momo.Column[string], text string, mode MySQLMode, fl uint16) {
	mod, _ := mode.modifier()
	b.WriteString("MATCH(")
	for i, c := range cols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.ExprFor(fl, c)
	}
	b.WriteString(") AGAINST (")
	b.ArgFor(fl, text)
	b.WriteString(" " + mod + ")")
}

// mysqlMatchCheck records the build error of a MySQL-only call and reports
// whether it may render.
func mysqlMatchCheck(b *momo.Builder, what string, cols []momo.Column[string], mode MySQLMode) bool {
	if b.Dialect().Name != dialect.MySQL {
		b.Unsupported("full-text search (" + what + ")")
		return false
	}
	if len(cols) == 0 {
		b.SetErr(validationError("%s needs at least one column", what))
		return false
	}
	if _, ok := mode.modifier(); !ok {
		b.SetErr(validationError("%s: unknown search mode %d", what, mode))
		return false
	}
	return true
}

func columnExprs(cols []momo.Column[string]) []momo.Expr {
	es := make([]momo.Expr, len(cols))
	for i, c := range cols {
		es[i] = c
	}
	return es
}

// MySQLMatch is MATCH(cols) AGAINST (q mode) > 0 on MySQL and MariaDB: true
// when the row matches. q is passed to the server as given and bound as a
// parameter, so in Boolean mode the caller owns the operators in it; use
// Search for text typed by a user. cols must be exactly the columns of one
// FULLTEXT index, in its order, else the server refuses and the build result is
// ErrValidation. Other dialects fail the build with ErrUnsupported.
func MySQLMatch(cols []momo.Column[string], q string, mode MySQLMode) momo.Predicate {
	return momo.PredicateFunc(func(b *momo.Builder) {
		if !mysqlMatchCheck(b, "MATCH AGAINST", cols, mode) {
			return
		}
		writeMySQLMatch(b, cols, q, mode, sensFlags(b, columnExprs(cols)...))
		b.WriteString(" > 0")
	})
}

// MySQLRelevance is MATCH(cols) AGAINST (q mode) as a value: the relevance of
// the row, 0 for a row that does not match. Relevance is comparable only within
// one query on one server. The rules of MySQLMatch apply.
func MySQLRelevance(cols []momo.Column[string], q string, mode MySQLMode) Expr[float64] {
	return newPortableExpr[float64]("MySQLRelevance", func(b *momo.Builder) {
		if !mysqlMatchCheck(b, "MATCH AGAINST", cols, mode) {
			return
		}
		writeMySQLMatch(b, cols, q, mode, sensFlags(b, columnExprs(cols)...))
	})
}

// mysqlSearch renders Search on MySQL and MariaDB: a boolean-mode MATCH over
// the declared columns, written bare so that a non-zero relevance is true.
func mysqlSearch(b *momo.Builder, t Target, q Query) {
	text, empty, fl, ok := mysqlPrepare(b, t, q)
	if !ok {
		return
	}
	if empty {
		b.WriteString("1 = 0")
		return
	}
	writeMySQLMatch(b, t.cols, text, Boolean, fl)
}

// mysqlScore renders Score on MySQL and MariaDB: the MATCH value, which is 0
// for a row the query does not match.
func mysqlScore(b *momo.Builder, t Target, q Query) {
	text, empty, fl, ok := mysqlPrepare(b, t, q)
	if !ok {
		return
	}
	if empty {
		b.WriteString("0")
		return
	}
	writeMySQLMatch(b, t.cols, text, Boolean, fl)
}

func mysqlPrepare(b *momo.Builder, t Target, q Query) (text string, empty bool, fl uint16, ok bool) {
	if t.kind != kindDeclared {
		b.Unsupported("full-text search (a tsvector column target)")
		return "", false, 0, false
	}
	text, empty = q.mysql()
	return text, empty, sensFlags(b, t.exprs()...), true
}
