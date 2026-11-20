// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

// castExpr renders inner::type, the type being a possibly schema-qualified
// name whose parts are quoted.
type castExpr struct {
	inner Expr
	typ   string
}

func (c castExpr) AppendSQL(b *Builder) {
	if b.Dialect().Name != dialect.Postgres {
		b.SetErr(&Error{Kind: ErrUnsupported, Op: "build", Err: errors.New("composite types exist on PostgreSQL only")})
		return
	}
	c.inner.AppendSQL(b)
	b.WriteString("::")
	if schemaName, name, ok := strings.Cut(c.typ, "."); ok {
		b.Ident(schemaName)
		b.WriteString(".")
		b.Ident(name)
		return
	}
	b.Ident(c.typ)
}

// CompositeEq is true when the composite column col equals v. typeName is the
// composite type of the column, as its type tag spells it, and casts the bound
// value: without it PostgreSQL reads the parameter of a comparison as an
// anonymous record and refuses its text. The record is compared field by field,
// so every attribute type needs an equality operator. On a dialect other than
// PostgreSQL the build fails with ErrUnsupported.
func CompositeEq[T any](col Column[types.Composite[T]], typeName string, v types.Composite[T]) Predicate {
	return binOp{col, " = ", castExpr{argExpr{v: v, flags: col.Flags()}, typeName}}
}

// CompositeNe is true when the composite column col differs from v; see
// CompositeEq.
func CompositeNe[T any](col Column[types.Composite[T]], typeName string, v types.Composite[T]) Predicate {
	return binOp{col, " <> ", castExpr{argExpr{v: v, flags: col.Flags()}, typeName}}
}
