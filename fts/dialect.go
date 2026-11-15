// SPDX-License-Identifier: Apache-2.0

package fts

import (
	"fmt"
	"math"
	"strconv"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// FTS5Match is the SQLite predicate "table" MATCH q over an FTS5 table, with q
// in FTS5 query syntax bound as a parameter. Other dialects fail the build
// with ErrUnsupported. Use Search for user input: it translates and escapes
// the text, FTS5Match passes it on as given.
func FTS5Match(table string, q string) momo.Predicate {
	return momo.PredicateFunc(func(b *momo.Builder) {
		if b.Dialect().Name != dialect.SQLite {
			b.Unsupported("full-text search (FTS5 MATCH)")
			return
		}
		b.Ident(table)
		b.WriteString(" MATCH ")
		b.Arg(q)
	})
}

// BM25 is bm25("table", weights...) over an FTS5 table, the relevance of the
// row matched in the same query; lower (more negative) is better. weights are
// per-column weights in column order. Other dialects fail the build with
// ErrUnsupported. Score negates bm25 so that higher is better.
func BM25(table string, weights ...float64) Expr[float64] {
	return newPortableExpr[float64]("bm25", func(b *momo.Builder) {
		if b.Dialect().Name != dialect.SQLite {
			b.Unsupported("full-text search (bm25)")
			return
		}
		for _, w := range weights {
			if math.IsNaN(w) || math.IsInf(w, 0) {
				b.SetErr(&momo.Error{Kind: momo.ErrValidation, Op: "build",
					Err: fmt.Errorf("bm25: weight %v is not a finite number", w)})
				return
			}
		}
		b.WriteString("bm25(")
		b.Ident(table)
		for _, w := range weights {
			b.WriteString(", " + strconv.FormatFloat(w, 'g', -1, 64))
		}
		b.WriteString(")")
	})
}
