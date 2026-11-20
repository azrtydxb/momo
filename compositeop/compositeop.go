// SPDX-License-Identifier: Apache-2.0

// Package compositeop holds the comparison of a column of a types.Composite
// with a whole value: equality and inequality of the record. Each binds its
// operand as one parameter cast to the composite type, because the server reads
// the parameter of a plain comparison as an anonymous record and refuses its
// text.
//
// Composite types exist on PostgreSQL alone, so every builder fails the build
// with momo.ErrUnsupported on another dialect.
//
// The package builds on the frozen extension API of the root package plus
// internal/exprkit, as rangeop does.
package compositeop

import (
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprkit"
	"github.com/azrtydxb/momo/types"
)

// compare returns the predicate left op $n::typeName, the type name being the
// composite type as the type tag of the column spells it, quoted part by part.
func compare(feature string, left momo.Expr, op string, arg any, typeName string) momo.Predicate {
	return exprkit.Predicate(func(b *momo.Builder) {
		if b.Dialect().Name != dialect.Postgres {
			exprkit.Unsupported(b, feature)
			return
		}
		fl := exprkit.RenderSensitive(b, left, exprkit.FlagsOf(left))
		b.WriteString(op)
		exprkit.ArgFor(b, fl, arg)
		b.WriteString("::")
		if schemaName, name, ok := strings.Cut(typeName, "."); ok {
			b.Ident(schemaName)
			b.WriteString(".")
			b.Ident(name)
			return
		}
		b.Ident(typeName)
	})
}

// Eq is true when the composite column col equals v. typeName is the composite
// type of the column, as its type tag spells it, and casts the bound value. The
// record is compared field by field, so every attribute type needs an equality
// operator.
func Eq[T any](col momo.Expression[types.Composite[T]], typeName string, v types.Composite[T]) momo.Predicate {
	return compare("composite equality (=)", col, " = ", v, typeName)
}

// Ne is true when the composite column col differs from v; see Eq.
func Ne[T any](col momo.Expression[types.Composite[T]], typeName string, v types.Composite[T]) momo.Predicate {
	return compare("composite inequality (<>)", col, " <> ", v, typeName)
}
