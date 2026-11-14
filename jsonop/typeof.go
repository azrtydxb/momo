// SPDX-License-Identifier: Apache-2.0

package jsonop

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

// TypeExpr is the JSON type name of a JSON value, an Expression[string] with
// the comparison and ordering methods of any scalar expression. Build it with
// TypeOf.
type TypeExpr struct {
	momo.Ops[string]
	src momo.Expr
}

// TypeOf is the type of the JSON value in c as one of the six names of
// PostgreSQL's jsonb_typeof: "object", "array", "string", "number", "boolean"
// and "null". Compare it with the string methods, for example
// TypeOf(setting).Eq("number").
//
// The result is SQL NULL for a SQL NULL column, and the text "null" for the
// JSON literal null, the same as jsonb_typeof. The other dialects report their
// own names, so Momo maps them to the six above:
//
//   - PostgreSQL: jsonb_typeof(c).
//   - SQLite: json_type(c), with integer and real mapped to "number", text to
//     "string" and true and false to "boolean".
//   - MySQL and MariaDB: JSON_TYPE(c), with INTEGER, UNSIGNED INTEGER, DOUBLE
//     and DECIMAL mapped to "number" and the rest lower-cased. A type that a
//     stored JSON document cannot have (DATE, OPAQUE and so on) gives NULL.
//
// c is a jsonb column on PostgreSQL; on SQLite, MySQL and MariaDB the value
// must be valid JSON text. A number is "number" whether it is whole or not, so
// TypeOf cannot tell 1 from 1.5.
func TypeOf[T any](c momo.Expression[types.JSON[T]]) TypeExpr {
	e := TypeExpr{src: c}
	e.Ops = momo.NewOps[string](e)
	return e
}

// Flags returns the sensitive flag of the column operand plus FlagNullable.
func (e TypeExpr) Flags() uint16 { return flags(e.src) }

// As names the expression in a select list.
func (e TypeExpr) As(alias string) momo.Aliased[string] { return momo.As[string](e, alias) }

// AppendSQL renders the type name. Other dialects than PostgreSQL, SQLite and
// MySQL fail the build with ErrUnsupported.
func (e TypeExpr) AppendSQL(b *momo.Builder) {
	src := func() { b.RenderSensitive(e.src, momo.FlagsOf(e.src)) }
	switch b.Dialect().Name {
	case dialect.Postgres:
		b.WriteString("jsonb_typeof(")
		src()
		b.WriteString(")")
	case dialect.SQLite:
		b.WriteString("CASE json_type(")
		src()
		b.WriteString(") WHEN 'object' THEN 'object' WHEN 'array' THEN 'array'" +
			" WHEN 'text' THEN 'string' WHEN 'integer' THEN 'number' WHEN 'real' THEN 'number'" +
			" WHEN 'true' THEN 'boolean' WHEN 'false' THEN 'boolean' WHEN 'null' THEN 'null' END")
	case dialect.MySQL:
		b.WriteString("CASE JSON_TYPE(")
		src()
		b.WriteString(") WHEN 'OBJECT' THEN 'object' WHEN 'ARRAY' THEN 'array'" +
			" WHEN 'STRING' THEN 'string' WHEN 'INTEGER' THEN 'number' WHEN 'UNSIGNED INTEGER' THEN 'number'" +
			" WHEN 'DOUBLE' THEN 'number' WHEN 'DECIMAL' THEN 'number'" +
			" WHEN 'BOOLEAN' THEN 'boolean' WHEN 'NULL' THEN 'null' END")
	default:
		b.Unsupported("JSON type name (TypeOf)")
	}
}
