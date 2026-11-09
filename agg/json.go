// SPDX-License-Identifier: Apache-2.0

package agg

import (
	"fmt"
	"slices"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

// Pair is one key and value of BuildObject.
type Pair struct {
	Key   string
	Value momo.Expr
}

// Field pairs a JSON object key with the expression that supplies its value.
func Field(key string, v momo.Expr) Pair { return Pair{key, v} }

// ObjectExpr is a JSON object built from key and value pairs.
type ObjectExpr[T any] struct {
	momo.Ops[types.JSON[T]]
	pairs []Pair
}

// BuildObject builds a JSON object from key and value pairs:
// jsonb_build_object on PostgreSQL, json_object on SQLite, JSON_OBJECT on
// MySQL. T is the type the object decodes into, usually a struct. Keys are
// rendered as literals and must not contain a backslash.
func BuildObject[T any](pairs ...Pair) ObjectExpr[T] {
	o := ObjectExpr[T]{pairs: slices.Clone(pairs)}
	o.Ops = momo.NewOps[types.JSON[T]](o)
	return o
}

// As names the expression in a select list.
func (o ObjectExpr[T]) As(alias string) momo.Aliased[types.JSON[T]] {
	return momo.As[types.JSON[T]](o, alias)
}

// AppendSQL renders the object.
func (o ObjectExpr[T]) AppendSQL(b *momo.Builder) {
	b.WriteString(map[dialect.Name]string{dialect.Postgres: "jsonb_build_object(", dialect.SQLite: "json_object(", dialect.MySQL: "JSON_OBJECT("}[b.Dialect().Name])
	for i, p := range o.pairs {
		if i > 0 {
			b.WriteString(", ")
		}
		if p.Value == nil {
			fail(b, fmt.Errorf("JSON field %q has no value expression", p.Key))
			return
		}
		lit, ok := stringLiteral(b, "JSON key", p.Key)
		if !ok {
			return
		}
		b.WriteString(lit + ", ")
		p.Value.AppendSQL(b)
	}
	_ = b.WriteByte(')')
}

// JSONExpr is a value converted to JSON.
type JSONExpr[T any] struct {
	momo.Ops[types.JSON[T]]
	e momo.Expr
}

// ToJSONB converts a value to JSON: to_jsonb(e) on PostgreSQL (a table alias
// converts the whole row) and json_quote(e) on SQLite. MySQL fails the build
// with ErrUnsupported.
func ToJSONB[T any](e momo.Expr) JSONExpr[T] {
	t := JSONExpr[T]{e: e}
	t.Ops = momo.NewOps[types.JSON[T]](t)
	return t
}

// As names the expression in a select list.
func (t JSONExpr[T]) As(alias string) momo.Aliased[types.JSON[T]] {
	return momo.As[types.JSON[T]](t, alias)
}

// AppendSQL renders the conversion.
func (t JSONExpr[T]) AppendSQL(b *momo.Builder) {
	switch b.Dialect().Name {
	case dialect.Postgres:
		b.WriteString("to_jsonb(")
	case dialect.SQLite:
		b.WriteString("json_quote(")
	default:
		b.Unsupported("to_jsonb")
		return
	}
	t.e.AppendSQL(b)
	_ = b.WriteByte(')')
}

// EmptyJSONArray is the empty JSON array literal, the fallback of
// sqlfn.CoalesceExpr over an aggregate that saw no rows:
// sqlfn.CoalesceExpr(JSONB[T](x), EmptyJSONArray[T]()).
func EmptyJSONArray[T any]() momo.Expression[types.JSON[[]T]] {
	return momo.Typed[types.JSON[[]T]](emptyJSONArray{})
}

type emptyJSONArray struct{}

func (emptyJSONArray) AppendSQL(b *momo.Builder) {
	switch b.Dialect().Name {
	case dialect.Postgres:
		b.WriteString("'[]'::jsonb")
	case dialect.MySQL:
		b.WriteString("JSON_ARRAY()")
	default:
		b.WriteString("'[]'")
	}
}
