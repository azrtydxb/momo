// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"slices"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// dmlSubquery marks an INSERT, UPDATE or DELETE builder used as the body of a
// common table expression. Only PostgreSQL accepts such a body.
type dmlSubquery interface {
	isDML()
	// hasReturning reports whether the statement has a RETURNING list, without
	// which a CTE body exposes no columns.
	hasReturning() bool
}

// addCTE returns ctes with one more entry, copy-on-write.
func addCTE(ctes []cte, name string, q Subquery, cols []string, recursive bool) []cte {
	return append(slices.Clip(ctes), cte{name: name, cols: slices.Clone(cols), q: q, recursive: recursive})
}

// cteNames lists the names the CTEs expose to the statement's scope.
func cteNames(ctes []cte) []string {
	names := make([]string, len(ctes))
	for i, c := range ctes {
		names[i] = c.name
	}
	return names
}

// writeWithClause writes "WITH [RECURSIVE] name [(cols)] AS (body), ... ".
func writeWithClause(b *Builder, ctes []cte) {
	if len(ctes) == 0 {
		return
	}
	b.WriteString("WITH ")
	for _, c := range ctes {
		if c.recursive {
			if !b.d.RecursiveCTE {
				b.unsupported("WITH RECURSIVE")
			}
			b.WriteString("RECURSIVE ")
			break
		}
	}
	for i, c := range ctes {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(c.name)
		if len(c.cols) > 0 {
			b.WriteString(" (")
			for j, col := range c.cols {
				if j > 0 {
					b.WriteString(", ")
				}
				b.Ident(col)
			}
			b.writeByte(')')
		}
		b.WriteString(" AS ")
		if c.q == nil {
			b.SetErr(newError(ErrValidation, "build", "", errors.New("WITH "+c.name+": nil query")))
			continue
		}
		if dml, ok := c.q.(dmlSubquery); ok {
			if b.d.Name != dialect.Postgres {
				b.unsupported("a data-modifying statement (INSERT, UPDATE, DELETE) in a WITH clause")
			} else if !dml.hasReturning() {
				b.SetErr(newError(ErrValidation, "build", "", errors.New("WITH "+c.name+": a data-modifying body needs Returning")))
				continue
			}
		}
		c.q.AppendSQL(b)
	}
	b.writeByte(' ')
}

// referencesTable reports whether the predicates constrain the table exposed
// as qualifier. A statement with a second source (UPDATE ... FROM, DELETE ...
// USING) whose WHERE never constrains its own target joins the target to the
// source with no condition and so touches every target row; the
// unfiltered-mutation guard uses this to refuse it. The predicates are ANDed,
// so one that constrains the target is enough.
//
// Inside a predicate an And (or a JoinSQL with AND) is satisfied by any of its
// terms, but an Or (or a JoinSQL with OR) only when every branch is, because a
// branch off the target can be true on its own. A Not swaps the two rules (De
// Morgan): NOT (a AND b) is NOT a OR NOT b, so every term of a negated And must
// constrain the target and any term of a negated Or is enough; a double
// negation is the original again. A JoinSQL with any other separator, and raw
// SQL fragments, record no column references and do not count.
//
// This checks that the WHERE references the target, not that it restricts it:
// a constant-true predicate such as t.id = t.id passes.
func referencesTable(d *dialect.Dialect, ps []Predicate, qualifier string) bool {
	for _, p := range ps {
		if constrainsTable(d, p, qualifier, false) {
			return true
		}
	}
	return false
}

// constrainsTable reports whether e, when true (or, with negated, when false),
// is limited to rows of the target table. See referencesTable.
func constrainsTable(d *dialect.Dialect, e Expr, qualifier string, negated bool) bool {
	switch j := e.(type) {
	case nil:
		return false
	case not:
		return constrainsTable(d, j.p, qualifier, !negated)
	case junction:
		return combineConstrains(d, j.parts, qualifier, negated, j.op == " OR ")
	case joinExpr:
		switch strings.ToUpper(strings.Trim(j.sep, " \t\r\n")) {
		case "AND":
			return combineConstrains(d, j.parts, qualifier, negated, false)
		case "OR":
			return combineConstrains(d, j.parts, qualifier, negated, true)
		}
		return false
	}
	b := NewBuilder(d)
	b.pushScope(nil)
	e.AppendSQL(b)
	_, ok := b.scopes[0].refs[qualifier]
	return ok
}

// combineConstrains applies the And and Or rules: an And needs any part and an
// Or needs every part, and negation swaps them. No parts never constrain.
func combineConstrains[E Expr](d *dialect.Dialect, parts []E, qualifier string, negated, or bool) bool {
	if len(parts) == 0 {
		return false
	}
	all := or != negated // an Or, or a negated And, needs every part
	for _, part := range parts {
		c := constrainsTable(d, part, qualifier, negated)
		if all && !c {
			return false
		}
		if !all && c {
			return true
		}
	}
	return all
}
