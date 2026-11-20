// SPDX-License-Identifier: Apache-2.0

// Package fts holds the PostgreSQL full-text search expressions: to_tsvector,
// the tsquery builders, the @@ match, ts_rank, ts_rank_cd and ts_headline.
// Every builder fails the build with momo.ErrUnsupported on another dialect.
//
// The text search configuration (english, simple, ...) is a string that is
// bound as a parameter and cast to regconfig, so it cannot carry SQL. A
// tsvector operand is typically a momo.Column[types.TSVector] kept current by
// a trigger, or TSVector over a text column. The operand types types.TSVector
// and types.TSQuery live in package types.
//
// The package builds on the exported extension API of the root package
// (Ops, PredicateFunc, Builder.ArgFor, Builder.ExprFor, Builder.RenderSensitive
// and Builder.Unsupported).
package fts

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

// Expr is a full-text function call whose SQL result type corresponds to T.
// It is a momo.Expression[T] with the comparison and ordering methods of any
// scalar expression (Asc and Desc order by it) and can be aliased with As.
type Expr[T any] struct {
	momo.Ops[T]
	what   string
	render func(b *momo.Builder)
	// portable marks an expression that renders on more than PostgreSQL; its
	// render function decides per dialect and records ErrUnsupported itself.
	portable bool
}

func newExpr[T any](what string, render func(b *momo.Builder)) Expr[T] {
	e := Expr[T]{what: what, render: render}
	e.Ops = momo.NewOps[T](e)
	return e
}

func newPortableExpr[T any](what string, render func(b *momo.Builder)) Expr[T] {
	e := Expr[T]{what: what, render: render, portable: true}
	e.Ops = momo.NewOps[T](e)
	return e
}

// As names the expression in a select list.
func (e Expr[T]) As(alias string) momo.Aliased[T] { return momo.As[T](e, alias) }

// AppendSQL renders the call, or records a build error on a dialect without
// full-text search or for an invalid argument.
func (e Expr[T]) AppendSQL(b *momo.Builder) {
	if e.render == nil || (!e.portable && !available(b, e.what)) {
		return
	}
	e.render(b)
}

// available records ErrUnsupported unless b renders for PostgreSQL.
func available(b *momo.Builder, what string) bool {
	if b.Dialect().Name == dialect.Postgres {
		return true
	}
	b.Unsupported("full-text search (" + what + ")")
	return false
}

// sensFlags reports momo.FlagSensitive when rendering any of es reads a
// sensitive column. The operands render into a scratch builder, so the answer
// is known before the first argument of the call is bound (a configuration
// precedes the operands in the SQL text). Arguments bound by the call are then
// masked in hook events, as for a comparison against a sensitive column.
func sensFlags(b *momo.Builder, es ...momo.Expr) uint16 {
	probe := momo.NewBuilder(b.Dialect())
	var fl uint16
	for _, e := range es {
		fl = probe.RenderSensitive(e, fl)
	}
	return fl & momo.FlagSensitive
}

// writeRegconfig binds config and casts it to regconfig. An empty config is a
// validation error: PostgreSQL would reject it at run time. fl carries
// FlagSensitive to mask the bound value.
func writeRegconfig(b *momo.Builder, what, config string, fl uint16) {
	if config == "" {
		b.SetErr(&momo.Error{Kind: momo.ErrValidation, Op: "build",
			Err: fmt.Errorf("%s: text search configuration must not be empty", what)})
		return
	}
	b.WriteString("CAST(")
	b.ArgFor(fl, config)
	b.WriteString(" AS regconfig)")
}

// ToVector is TSVector.
//
// Deprecated: use TSVector. ToVector is removed in the next release.
func ToVector(config string, doc momo.Expression[string]) Expr[types.TSVector] {
	return TSVector(config, doc)
}

// TSVector is to_tsvector(config, doc): doc parsed into a tsvector with the
// text search configuration config. PostgreSQL only.
func TSVector(config string, doc momo.Expression[string]) Expr[types.TSVector] {
	return newExpr[types.TSVector]("to_tsvector", func(b *momo.Builder) {
		fl := sensFlags(b, doc)
		b.WriteString("to_tsvector(")
		writeRegconfig(b, "to_tsvector", config, fl)
		b.WriteString(", ")
		b.ExprFor(fl, doc)
		b.WriteString(")")
	})
}

// query builds fn(config, query) with query bound as a parameter.
func query(fn, config, text string) Expr[types.TSQuery] {
	return newExpr[types.TSQuery](fn, func(b *momo.Builder) {
		b.WriteString(fn + "(")
		writeRegconfig(b, fn, config, 0)
		b.WriteString(", ")
		b.Arg(text)
		b.WriteString(")")
	})
}

// WebsearchToQuery is TSWebsearchQuery.
//
// Deprecated: use TSWebsearchQuery. WebsearchToQuery is removed in the next release.
func WebsearchToQuery(config, text string) Expr[types.TSQuery] {
	return TSWebsearchQuery(config, text)
}

// TSWebsearchQuery is websearch_to_tsquery(config, query): the query in the
// forgiving web-search syntax (quoted phrases, or, a leading minus). It never
// fails on user input, which makes it the right choice for a search box.
// PostgreSQL only.
func TSWebsearchQuery(config, text string) Expr[types.TSQuery] {
	return query("websearch_to_tsquery", config, text)
}

// PlainToQuery is TSPlainQuery.
//
// Deprecated: use TSPlainQuery. PlainToQuery is removed in the next release.
func PlainToQuery(config, text string) Expr[types.TSQuery] {
	return TSPlainQuery(config, text)
}

// TSPlainQuery is plainto_tsquery(config, query): the words of query ANDed
// together, punctuation ignored. PostgreSQL only.
func TSPlainQuery(config, text string) Expr[types.TSQuery] {
	return query("plainto_tsquery", config, text)
}

// PhraseToQuery is TSPhraseQuery.
//
// Deprecated: use TSPhraseQuery. PhraseToQuery is removed in the next release.
func PhraseToQuery(config, text string) Expr[types.TSQuery] {
	return TSPhraseQuery(config, text)
}

// TSPhraseQuery is phraseto_tsquery(config, query): the words of query as one
// phrase. PostgreSQL only.
func TSPhraseQuery(config, text string) Expr[types.TSQuery] {
	return query("phraseto_tsquery", config, text)
}

// ToQuery is TSQuery.
//
// Deprecated: use TSQuery. ToQuery is removed in the next release.
func ToQuery(config, text string) Expr[types.TSQuery] {
	return TSQuery(config, text)
}

// TSQuery is to_tsquery(config, query): query must already be in tsquery
// syntax (& | ! <-> and prefix:*); a malformed query is a database error.
// PostgreSQL only.
func TSQuery(config, text string) Expr[types.TSQuery] {
	return query("to_tsquery", config, text)
}

// Match is TSMatch.
//
// Deprecated: use TSMatch. Match is removed in the next release.
func Match(vec momo.Expression[types.TSVector], q momo.Expression[types.TSQuery]) momo.Predicate {
	return TSMatch(vec, q)
}

// TSMatch is vec @@ q: true when the document matches the query. It is
// PostgreSQL only; other dialects fail the build with ErrUnsupported.
func TSMatch(vec momo.Expression[types.TSVector], q momo.Expression[types.TSQuery]) momo.Predicate {
	return momo.PredicateFunc(func(b *momo.Builder) {
		if !available(b, "@@") {
			return
		}
		fl := sensFlags(b, vec, q)
		b.ExprFor(fl, vec)
		b.WriteString(" @@ ")
		b.ExprFor(fl, q)
	})
}

// maxNormalization is the sum of all ts_rank normalization flags.
const maxNormalization = 63

func rank(fn string, vec momo.Expression[types.TSVector], q momo.Expression[types.TSQuery], normalization []int) Expr[float64] {
	return newExpr[float64](fn, func(b *momo.Builder) {
		if len(normalization) > 1 {
			b.SetErr(&momo.Error{Kind: momo.ErrValidation, Op: "build",
				Err: errors.New(fn + ": at most one normalization value")})
			return
		}
		if len(normalization) == 1 && (normalization[0] < 0 || normalization[0] > maxNormalization) {
			b.SetErr(&momo.Error{Kind: momo.ErrValidation, Op: "build",
				Err: fmt.Errorf("%s: normalization %d outside 0..%d", fn, normalization[0], maxNormalization)})
			return
		}
		fl := sensFlags(b, vec, q)
		b.WriteString(fn + "(")
		b.ExprFor(fl, vec)
		b.WriteString(", ")
		b.ExprFor(fl, q)
		if len(normalization) == 1 {
			b.WriteString(", ")
			b.WriteString(strconv.Itoa(normalization[0]))
		}
		b.WriteString(")")
	})
}

// Rank is TSRank.
//
// Deprecated: use TSRank. Rank is removed in the next release.
func Rank(vec momo.Expression[types.TSVector], q momo.Expression[types.TSQuery], normalization ...int) Expr[float64] {
	return TSRank(vec, q, normalization...)
}

// TSRank is ts_rank(vec, q [, normalization]): a relevance score from how often
// the query terms occur. normalization is the optional bit mask of
// PostgreSQL's ts_rank flags (0 to 63), rendered as a literal. PostgreSQL only.
func TSRank(vec momo.Expression[types.TSVector], q momo.Expression[types.TSQuery], normalization ...int) Expr[float64] {
	return rank("ts_rank", vec, q, normalization)
}

// RankCD is TSRankCD.
//
// Deprecated: use TSRankCD. RankCD is removed in the next release.
func RankCD(vec momo.Expression[types.TSVector], q momo.Expression[types.TSQuery], normalization ...int) Expr[float64] {
	return TSRankCD(vec, q, normalization...)
}

// TSRankCD is ts_rank_cd(vec, q [, normalization]): the cover-density score,
// which also rewards query terms that sit close together. PostgreSQL only.
func TSRankCD(vec momo.Expression[types.TSVector], q momo.Expression[types.TSQuery], normalization ...int) Expr[float64] {
	return rank("ts_rank_cd", vec, q, normalization)
}

// Headline is TSHeadline.
//
// Deprecated: use TSHeadline. Headline is removed in the next release.
func Headline(config string, doc momo.Expression[string], q momo.Expression[types.TSQuery], options string) Expr[string] {
	return TSHeadline(config, doc, q, options)
}

// TSHeadline is ts_headline(config, doc, q [, options]): doc with the query
// matches marked up. options is PostgreSQL's comma-separated option string
// (for example StartSel=<b>, StopSel=</b>, MaxFragments=2); it is bound as a
// parameter, and an empty string leaves the server defaults. doc is the
// original text, not the tsvector. PostgreSQL only.
func TSHeadline(config string, doc momo.Expression[string], q momo.Expression[types.TSQuery], options string) Expr[string] {
	return newExpr[string]("ts_headline", func(b *momo.Builder) {
		fl := sensFlags(b, doc, q)
		b.WriteString("ts_headline(")
		writeRegconfig(b, "ts_headline", config, fl)
		b.WriteString(", ")
		b.ExprFor(fl, doc)
		b.WriteString(", ")
		b.ExprFor(fl, q)
		if options != "" {
			b.WriteString(", ")
			b.ArgFor(fl, options)
		}
		b.WriteString(")")
	})
}
