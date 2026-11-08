// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/azrtydxb/momo/dialect"
)

// DistinctOn keeps the first row of each group of rows that agree on es:
// SELECT DISTINCT ON (es). Which row is first is decided by OrderBy, so
// DistinctOn requires an OrderBy whose leading items are exactly es (in any
// order) followed by the tie-breakers that pick the row; otherwise the build
// fails with ErrValidation, the same rule PostgreSQL enforces. DistinctOn
// replaces Distinct. PostgreSQL only; other dialects fail the build with
// ErrUnsupported.
func (sq SelectQuery[T]) DistinctOn(es ...Expr) SelectQuery[T] {
	if len(es) == 0 {
		sq.s.err = &Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: errors.New("DistinctOn needs at least one expression")}
		return sq
	}
	for _, e := range es {
		rejectAliased(&sq.s.err, "DISTINCT ON", e)
	}
	sq.s.distinctOn = slices.Clone(es)
	return sq
}

// renderDistinctOn writes DISTINCT ON (...) after SELECT, validating the
// ORDER BY prefix rule.
func (sq SelectQuery[T]) renderDistinctOn(b *Builder) {
	s := &sq.s
	if b.d.Name != dialect.Postgres {
		b.unsupported("DISTINCT ON")
		return
	}
	// A repeated expression adds nothing: keep the first of each.
	var exprs []Expr
	var seen []renderedExpr
	for _, e := range s.distinctOn {
		r := renderExpr(b.d, e)
		if !slices.ContainsFunc(seen, r.equal) {
			seen = append(seen, r)
			exprs = append(exprs, e)
		}
	}
	n := len(exprs)
	fail := func(msg string, args ...any) {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: fmt.Errorf(msg, args...)})
	}
	if len(s.order) < n {
		fail("DistinctOn needs an OrderBy whose first %d items are the DISTINCT ON expressions; the row kept per group is otherwise arbitrary", n)
		return
	}
	// Each DISTINCT ON expression must be matched by a distinct leading
	// ORDER BY item.
	lead := make([]renderedExpr, n)
	for i := range lead {
		lead[i] = renderExpr(b.d, s.order[i].Expr)
	}
	used := make([]bool, n)
	for _, e := range exprs {
		want := renderExpr(b.d, e)
		found := false
		for i, l := range lead {
			if !used[i] && l.equal(want) {
				used[i], found = true, true
				break
			}
		}
		if !found {
			fail("DistinctOn expression %s is not among the first %d OrderBy items; PostgreSQL requires the leading ORDER BY items to match the DISTINCT ON expressions", want.sql, n)
			return
		}
	}
	b.WriteString("DISTINCT ON (")
	appendList(b, exprs)
	b.WriteString(") ")
}

// renderedExpr is an expression rendered on its own, for comparing two
// expressions by their SQL text and arguments.
type renderedExpr struct {
	sql  string
	args []any
}

func renderExpr(d *dialect.Dialect, e Expr) renderedExpr {
	sb := &Builder{d: d}
	// The real render checks qualifiers; a scratch scope keeps this one from
	// reporting them.
	sb.pushScope(nil)
	e.AppendSQL(sb)
	return renderedExpr{sql: string(sb.buf), args: sb.args}
}

func (r renderedExpr) equal(o renderedExpr) bool {
	return r.sql == o.sql && reflect.DeepEqual(r.args, o.args)
}
