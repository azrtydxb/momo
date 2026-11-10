// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"slices"

	"github.com/azrtydxb/momo/dialect"
)

// dmlSubquery marks an INSERT, UPDATE or DELETE builder used as the body of a
// common table expression. Only PostgreSQL accepts such a body.
type dmlSubquery interface{ isDML() }

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
		if _, ok := c.q.(dmlSubquery); ok && b.d.Name != dialect.Postgres {
			b.unsupported("a data-modifying statement (INSERT, UPDATE, DELETE) in a WITH clause")
		}
		c.q.AppendSQL(b)
	}
	b.writeByte(' ')
}

// referencesTable reports whether any predicate refers to a column of the
// table exposed as qualifier. A statement with a second source (UPDATE ...
// FROM, DELETE ... USING) whose WHERE never mentions its own target joins the
// target to the source with no condition and so touches every target row; the
// unfiltered-mutation guard uses this to refuse it. Raw SQL fragments record
// no column references and do not count.
func referencesTable(d *dialect.Dialect, ps []Predicate, qualifier string) bool {
	for _, p := range ps {
		if p == nil {
			continue
		}
		b := NewBuilder(d)
		b.pushScope(nil)
		p.AppendSQL(b)
		if _, ok := b.scopes[0].refs[qualifier]; ok {
			return true
		}
	}
	return false
}
