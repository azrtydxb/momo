// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

// Full-text search (PostgreSQL only). Every builder here fails the build with
// ErrUnsupported on another dialect. The text search configuration (english,
// simple, ...) is a string that is bound as a parameter and cast to
// regconfig, so it cannot carry SQL. A tsvector operand is typically a
// Column[types.TSVector] kept current by a trigger, or ToTSVector over a text
// column.

// TSExpr is a full-text function call whose SQL result type corresponds to T.
// It is an Expression[T] with the comparison and ordering methods of any
// scalar expression (Asc and Desc order by it) and can be aliased with As.
type TSExpr[T any] struct {
	exprOps[T]
	what   string
	render func(b *Builder)
}

func newTS[T any](what string, render func(b *Builder)) TSExpr[T] {
	e := TSExpr[T]{what: what, render: render}
	e.exprOps = selfOps[T](e)
	return e
}

// As names the expression in a select list.
func (e TSExpr[T]) As(alias string) Aliased[T] { return As[T](e, alias) }

func (TSExpr[T]) resultType() T { var z T; return z }

// AppendSQL renders the call, or records a build error on a dialect without
// full-text search or for an invalid argument.
func (e TSExpr[T]) AppendSQL(b *Builder) {
	if !tsAvailable(b, e.what) || e.render == nil {
		return
	}
	e.render(b)
}

// tsAvailable records ErrUnsupported unless b renders for PostgreSQL.
func tsAvailable(b *Builder, what string) bool {
	if b.d.Name == dialect.Postgres {
		return true
	}
	b.SetErr(&Error{Kind: ErrUnsupported, Op: "build",
		Err: fmt.Errorf("full-text search (%s) not available on %s", what, b.d.Name)})
	return false
}

// writeRegconfig binds config and casts it to regconfig. An empty config is a
// validation error: PostgreSQL would reject it at run time.
func writeRegconfig(b *Builder, what, config string) {
	if config == "" {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build",
			Err: fmt.Errorf("%s: text search configuration must not be empty", what)})
		return
	}
	b.WriteString("CAST(")
	b.Arg(config)
	b.WriteString(" AS regconfig)")
}

// ToTSVector is to_tsvector(config, doc): doc parsed into a tsvector with the
// text search configuration config.
func ToTSVector(config string, doc Expression[string]) TSExpr[types.TSVector] {
	return newTS[types.TSVector]("to_tsvector", func(b *Builder) {
		b.WriteString("to_tsvector(")
		writeRegconfig(b, "to_tsvector", config)
		b.WriteString(", ")
		doc.AppendSQL(b)
		b.WriteString(")")
	})
}

// tsQuery builds fn(config, query) with query bound as a parameter.
func tsQuery(fn, config, query string) TSExpr[types.TSQuery] {
	return newTS[types.TSQuery](fn, func(b *Builder) {
		b.WriteString(fn + "(")
		writeRegconfig(b, fn, config)
		b.WriteString(", ")
		b.Arg(query)
		b.WriteString(")")
	})
}

// WebsearchToTSQuery is websearch_to_tsquery(config, query): the query in the
// forgiving web-search syntax (quoted phrases, or, a leading minus). It never
// fails on user input, which makes it the right choice for a search box.
func WebsearchToTSQuery(config, query string) TSExpr[types.TSQuery] {
	return tsQuery("websearch_to_tsquery", config, query)
}

// PlainToTSQuery is plainto_tsquery(config, query): the words of query ANDed
// together, punctuation ignored.
func PlainToTSQuery(config, query string) TSExpr[types.TSQuery] {
	return tsQuery("plainto_tsquery", config, query)
}

// PhraseToTSQuery is phraseto_tsquery(config, query): the words of query as
// one phrase.
func PhraseToTSQuery(config, query string) TSExpr[types.TSQuery] {
	return tsQuery("phraseto_tsquery", config, query)
}

// ToTSQuery is to_tsquery(config, query): query must already be in tsquery
// syntax (& | ! <-> and prefix:*); a malformed query is a database error.
func ToTSQuery(config, query string) TSExpr[types.TSQuery] {
	return tsQuery("to_tsquery", config, query)
}

// tsMatch is the @@ predicate.
type tsMatch struct {
	vec Expression[types.TSVector]
	q   Expression[types.TSQuery]
}

func (m tsMatch) AppendSQL(b *Builder) {
	if !tsAvailable(b, "@@") {
		return
	}
	m.vec.AppendSQL(b)
	b.WriteString(" @@ ")
	m.q.AppendSQL(b)
}

func (tsMatch) resultType() bool { return false }

// TSMatch is vec @@ q: true when the document matches the query.
func TSMatch(vec Expression[types.TSVector], q Expression[types.TSQuery]) Predicate {
	return tsMatch{vec, q}
}

// maxTSNormalization is the sum of all ts_rank normalization flags.
const maxTSNormalization = 63

func tsRank(fn string, vec Expression[types.TSVector], q Expression[types.TSQuery], normalization []int) TSExpr[float64] {
	return newTS[float64](fn, func(b *Builder) {
		if len(normalization) > 1 {
			b.SetErr(&Error{Kind: ErrValidation, Op: "build",
				Err: errors.New(fn + ": at most one normalization value")})
			return
		}
		if len(normalization) == 1 && (normalization[0] < 0 || normalization[0] > maxTSNormalization) {
			b.SetErr(&Error{Kind: ErrValidation, Op: "build",
				Err: fmt.Errorf("%s: normalization %d outside 0..%d", fn, normalization[0], maxTSNormalization)})
			return
		}
		b.WriteString(fn + "(")
		vec.AppendSQL(b)
		b.WriteString(", ")
		q.AppendSQL(b)
		if len(normalization) == 1 {
			b.WriteString(", ")
			intLit(normalization[0]).AppendSQL(b)
		}
		b.WriteString(")")
	})
}

// TSRank is ts_rank(vec, q [, normalization]): a relevance score from how
// often the query terms occur. normalization is the optional bit mask of
// PostgreSQL's ts_rank flags (0 to 63), rendered as a literal.
func TSRank(vec Expression[types.TSVector], q Expression[types.TSQuery], normalization ...int) TSExpr[float64] {
	return tsRank("ts_rank", vec, q, normalization)
}

// TSRankCD is ts_rank_cd(vec, q [, normalization]): the cover-density score,
// which also rewards query terms that sit close together.
func TSRankCD(vec Expression[types.TSVector], q Expression[types.TSQuery], normalization ...int) TSExpr[float64] {
	return tsRank("ts_rank_cd", vec, q, normalization)
}

// TSHeadline is ts_headline(config, doc, q [, options]): doc with the query
// matches marked up. options is PostgreSQL's comma-separated option string
// (for example StartSel=<b>, StopSel=</b>, MaxFragments=2); it is bound as a
// parameter, and an empty string leaves the server defaults. doc is the
// original text, not the tsvector.
func TSHeadline(config string, doc Expression[string], q Expression[types.TSQuery], options string) TSExpr[string] {
	return newTS[string]("ts_headline", func(b *Builder) {
		b.WriteString("ts_headline(")
		writeRegconfig(b, "ts_headline", config)
		b.WriteString(", ")
		doc.AppendSQL(b)
		b.WriteString(", ")
		q.AppendSQL(b)
		if options != "" {
			b.WriteString(", ")
			b.Arg(options)
		}
		b.WriteString(")")
	})
}
