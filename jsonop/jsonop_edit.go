// SPDX-License-Identifier: Apache-2.0

package jsonop

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

// Expr is a JSON value computed from other JSON expressions: the result of
// DeleteKey, Concat or Set. It is an Expression[types.JSON[T]], so it can be
// compared, selected, assigned in an update, or used again as the column
// operand of Text, Path, Root, Contains or HasKey. It carries the sensitive
// flag of its operands, so a value computed from a sensitive column stays
// sensitive.
type Expr[T any] struct {
	momo.Ops[types.JSON[T]]
	fl     uint16
	render func(b *momo.Builder)
}

func newExpr[T any](fl uint16, render func(b *momo.Builder)) Expr[T] {
	e := Expr[T]{fl: fl, render: render}
	e.Ops = momo.NewOps[types.JSON[T]](e)
	return e
}

// Flags returns the sensitive and nullable flags of the operands.
func (e Expr[T]) Flags() uint16 { return e.fl }

// AppendSQL renders the expression.
func (e Expr[T]) AppendSQL(b *momo.Builder) { e.render(b) }

// opFlags merges the sensitive and nullable flags of the operands.
func opFlags(es ...momo.Expr) uint16 {
	var fl uint16
	for _, e := range es {
		fl |= momo.FlagsOf(e) & (momo.FlagSensitive | momo.FlagNullable)
	}
	return fl
}

// DeleteKey is c with the top-level member key removed: col - 'key' on
// PostgreSQL, json_remove on SQLite and JSON_REMOVE on MySQL. A missing key
// leaves c unchanged. Use it to keep a secret out of a snapshot without
// listing the other members: DeleteKey(snapshot, "password_hash") still
// carries a column added later, and still drops the secret. key is always a
// bound parameter (CAST($n AS text) on PostgreSQL, so the operator resolves)
// and always an object key, never an array index. On PostgreSQL, a JSON array
// operand removes a string element equal to key instead. On PostgreSQL the
// operand must be jsonb: the - operator does not exist for the json type. The
// result is typed as T, so a column type that does not match what the
// statement returns is only caught when the row is scanned.
func DeleteKey[T any](c momo.Expression[types.JSON[T]], key string) Expr[T] {
	return newExpr[T](opFlags(c), func(b *momo.Builder) {
		name := b.Dialect().Name
		if !keysValid(b, []string{key}, name != dialect.Postgres) {
			return
		}
		switch name {
		case dialect.Postgres:
			b.WriteString("(")
			fl := b.RenderSensitive(c, momo.FlagsOf(c))
			b.WriteString(" - CAST(")
			b.ArgFor(fl, key)
			b.WriteString(" AS text))")
		case dialect.SQLite:
			b.WriteString("json_remove(")
			fl := b.RenderSensitive(c, momo.FlagsOf(c))
			b.WriteString(", ")
			b.ArgFor(fl, pathString(name, []string{key}, false))
			b.WriteString(")")
		case dialect.MySQL:
			b.WriteString("JSON_REMOVE(")
			fl := b.RenderSensitive(c, momo.FlagsOf(c))
			b.WriteString(", ")
			b.ArgFor(fl, pathString(name, []string{key}, false))
			b.WriteString(")")
		default:
			b.Unsupported("JSON key delete (DeleteKey)")
		}
	})
}

// Concat is the shallow merge of two JSON values: a || b on PostgreSQL. The
// members of b win over equal keys of a, nested objects are replaced and not
// merged, and a JSON null member of b stays in the result. The result has the
// type of a, so Concat(release, jsonb_build_object(...)) reads back as the
// release type; b may be any JSON expression, such as a momo.Func call. SQLite
// and MySQL have no operator with these rules (json_patch and
// JSON_MERGE_PATCH merge recursively and drop null members, JSON_MERGE_PRESERVE
// builds arrays from equal keys), so Concat fails with ErrUnsupported there
// instead of rendering a different result. The operands must be jsonb, since
// the || operator on json does not exist. The result is typed as T without
// checking: a mismatch with the column or struct it is scanned into is only
// caught at scan time.
func Concat[T, U any](a momo.Expression[types.JSON[T]], b momo.Expression[types.JSON[U]]) Expr[T] {
	return newExpr[T](opFlags(a, b), func(bl *momo.Builder) {
		if bl.Dialect().Name != dialect.Postgres {
			bl.Unsupported("JSON concatenation (Concat)")
			return
		}
		bl.WriteString("(")
		fl := bl.RenderSensitive(a, momo.FlagsOf(a))
		bl.WriteString(" || ")
		bl.ExprFor(fl, b)
		bl.WriteString(")")
	})
}

// Set is target with the value at path replaced: jsonb_set(target, path,
// value) on PostgreSQL. path is a text array expression, so it can be built in
// SQL with momo.ArrayOf, for example a key computed from a bound parameter;
// value is a JSON expression, such as to_jsonb(...) through momo.Func. A
// missing last element of the path is created; a missing earlier element
// leaves target unchanged, as in PostgreSQL. A SQL NULL value, path or target
// yields NULL. SQLite and MySQL take a constant path string and cannot
// express a path computed in SQL, so Set fails with ErrUnsupported there. The
// target must be jsonb, since jsonb_set does not take json. A SQL NULL value
// makes the whole result NULL, not a member set to null: wrap an optional
// value in COALESCE (or pass to_jsonb of a default). The result is typed as T
// without checking, so a mismatch is only caught at scan time.
func Set[T, U any](target momo.Expression[types.JSON[T]], path momo.Expression[[]string], value momo.Expression[types.JSON[U]]) Expr[T] {
	return newExpr[T](opFlags(target, path, value), func(b *momo.Builder) {
		if b.Dialect().Name != dialect.Postgres {
			b.Unsupported("JSON set with a computed path (Set)")
			return
		}
		b.WriteString("jsonb_set(")
		fl := b.RenderSensitive(target, momo.FlagsOf(target))
		b.WriteString(", ")
		b.ExprFor(fl, path)
		b.WriteString(", ")
		b.ExprFor(fl, value)
		b.WriteString(")")
	})
}
