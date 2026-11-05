// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// Allowlist maps external names to typed column descriptors. Unknown names are
// an error: a name that arrives from outside (a sort key, a filter field) is
// only ever turned into SQL through the descriptor it resolves to, whose
// identifier is quoted by the builder, never by splicing the string.
type Allowlist struct{ m map[string]ColumnRef }

// NewAllowlist builds an allowlist keyed by ColumnName(). A later column with
// the same name replaces an earlier one.
func NewAllowlist(cols ...ColumnRef) Allowlist {
	m := make(map[string]ColumnRef, len(cols))
	for _, c := range cols {
		m[c.ColumnName()] = c
	}
	return Allowlist{m: m}
}

// Column returns the descriptor for name, or a *Error with Kind ErrValidation.
// The error does not echo name, which may be attacker controlled.
func (a Allowlist) Column(name string) (ColumnRef, error) {
	if c, ok := a.m[name]; ok {
		return c, nil
	}
	return nil, &Error{Kind: ErrValidation, Op: "allowlist", Err: errors.New("unknown column name")}
}

// EscapeLike escapes %, _ and the escape character \ so the result matches
// literally inside a LIKE pattern that is rendered with ESCAPE '\'.
func EscapeLike(s string) string {
	if !strings.ContainsAny(s, `%_\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '%', '_', '\\':
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// ContainsLiteral matches rows whose string expression contains s literally.
// s is escaped with EscapeLike, wrapped in % and bound as a parameter; the
// statement carries an explicit ESCAPE clause rendered for the dialect (MySQL
// string literals treat backslash as an escape, so it is doubled there; this
// assumes the default sql_mode, without NO_BACKSLASH_ESCAPES).
func ContainsLiteral(e Expression[string], s string) Predicate {
	return containsLiteral{e, "%" + EscapeLike(s) + "%"}
}

type containsLiteral struct {
	e Expr
	p string
}

func (x containsLiteral) AppendSQL(b *Builder) {
	x.e.AppendSQL(b)
	b.WriteString(" LIKE ")
	b.Arg(x.p)
	if b.d.Name == dialect.MySQL {
		b.WriteString(` ESCAPE '\\'`)
	} else {
		b.WriteString(` ESCAPE '\'`)
	}
}
func (containsLiteral) resultType() bool { return false }
