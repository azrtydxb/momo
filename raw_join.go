// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"slices"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// joinSeparators is the allowlist of separators JoinSQL accepts, keyed by
// the upper-cased separator with surrounding whitespace removed. The value
// is the canonical text written between parts, and whether each part (and
// the whole join) is parenthesised: operators are, so a part holding a
// lower-precedence operator cannot regroup the result.
var joinSeparators = map[string]struct {
	text   string
	parens bool
}{
	",":   {", ", false},
	"":    {" ", false},
	"AND": {" AND ", true},
	"OR":  {" OR ", true},
	"||":  {" || ", true},
	"+":   {" + ", true},
}

// joinExpr is the result of JoinSQL.
type joinExpr struct {
	sep   string
	parts []Expr
}

func (j joinExpr) AppendSQL(b *Builder) {
	s, ok := joinSeparators[strings.ToUpper(strings.Trim(j.sep, " \t\r\n"))]
	switch {
	case !ok:
		b.SetErr(rawErr("JoinSQL separator %q is not one of , AND OR || + or a space", j.sep))
		return
	case len(j.parts) == 0 && (s.text == " AND " || s.text == " OR "):
		// Match And and Or: the empty conjunction is true, the empty
		// disjunction false.
		if s.text == " AND " {
			b.WriteString("1 = 1")
		} else {
			b.WriteString("1 = 0")
		}
		return
	case len(j.parts) == 0:
		b.SetErr(rawErr("JoinSQL with separator %q needs at least one part", j.sep))
		return
	case s.text == " || " && b.d.Name == dialect.MySQL:
		// || is logical OR on MySQL; concatenate with CONCAT instead.
		b.WriteString("CONCAT(")
		for i, p := range j.parts {
			if p == nil {
				b.SetErr(rawErr("JoinSQL part %d is nil", i))
				return
			}
			if i > 0 {
				b.WriteString(", ")
			}
			b.writeByte('(')
			p.AppendSQL(b)
			b.writeByte(')')
		}
		b.writeByte(')')
		return
	}
	if s.parens {
		b.writeByte('(')
	}
	for i, p := range j.parts {
		if p == nil {
			b.SetErr(rawErr("JoinSQL part %d is nil", i))
			return
		}
		if i > 0 {
			b.WriteString(s.text)
		}
		if s.parens {
			b.writeByte('(')
		}
		p.AppendSQL(b)
		if s.parens {
			b.writeByte(')')
		}
	}
	if s.parens {
		b.writeByte(')')
	}
}

func (joinExpr) resultType() bool { return false }
func (joinExpr) isRaw() bool      { return true }

// JoinSQL composes raw SQL fragments and their arguments into one
// expression, the equivalent of Prisma.join: each part is typically
// SQL("status = ?", s), and the result is usable wherever SQL is. Because
// every part keeps its own arguments and placeholders are numbered as the
// whole statement renders, fragments can be collected in a slice and
// combined without hand-counting binds:
//
//	var conds []momo.Expr
//	if status != "" { conds = append(conds, momo.SQL("status = ?", status)) }
//	if owner != 0 { conds = append(conds, momo.SQL("owner_id = ?", owner)) }
//	where := momo.JoinSQL(" AND ", conds...)
//
// sep must be a fixed literal and is checked against an allowlist, ignoring
// surrounding spaces and letter case: a comma, AND, OR, ||, + or a plain
// space (sep empty or only spaces). Any other separator fails the build with
// ErrValidation, so the separator can never carry caller-controlled SQL.
// For AND, OR, || and + every part and the whole join are parenthesised,
// which keeps operator precedence intact; a comma or a space writes the
// parts as they are. On MySQL, where || is logical OR, a "||" join renders
// CONCAT of the parenthesised parts. A join of no parts renders 1 = 1 for
// AND and 1 = 0 for OR, like And and Or; with any other separator, or with
// a nil part, the build fails with ErrValidation. Use JoinTypedSQL when the
// join must carry a Go type.
func JoinSQL(sep string, parts ...Expr) Predicate {
	return joinExpr{sep: sep, parts: slices.Clone(parts)}
}

// JoinTypedSQL is JoinSQL with a declared result type, for a join used in a
// projection or a typed comparison, for example a "+" sum of columns. Momo
// cannot verify that the declaration is truthful.
func JoinTypedSQL[T any](sep string, parts ...Expr) Expression[T] {
	return typed[T]{joinExpr{sep: sep, parts: slices.Clone(parts)}}
}
