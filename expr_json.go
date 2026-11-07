// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

// JSON operators over types.JSON[T] columns (jsonb on PostgreSQL).
//
// The column operand is any Expression of types.JSON[T]: a Column, a
// NullColumn, or an aliased column. Every key, path element and containment
// value is a bound parameter, never SQL text. The expressions carry the
// column's FlagSensitive, so a sensitive JSON column stays sensitive through
// extraction (see their Flags methods).
//
// SQL NULL versus JSON null. A nullable JSON column has two distinct empty
// values: SQL NULL (no value; col IS NULL) and the JSON literal null (a value
// that encodes as "null", for example a nil map or pointer in types.JSON).
// JSONText and JSONPath yield SQL NULL for both of them, for a missing key and
// for a key whose value is JSON null, so extracted text cannot tell the cases
// apart; test the column itself with IsNull for SQL NULL, or compare it with a
// types.JSON whose V is nil for JSON null. JSONHasKey is true for a key whose
// value is JSON null and false for a missing key.
//
// Dialects. PostgreSQL uses the jsonb operators ->>, #>>, @> and ?. SQLite
// renders json_extract and json_type (JSONContains is ErrUnsupported). MySQL
// renders JSON_UNQUOTE(JSON_EXTRACT), JSON_CONTAINS and JSON_CONTAINS_PATH.
// SQLite and MySQL are not usable in this release; their SQL is golden-tested
// only. Extraction differs at the edges: SQLite casts json_extract to TEXT, so
// a JSON boolean reads as 1 or 0 instead of true or false, and MySQL returns
// the text null for a JSON null.
//
// Path elements. JSONText takes one object key. In JSONPath, an element made
// only of digits addresses an array index on SQLite and MySQL, and an array
// index or an object key on PostgreSQL; any other element is an object key.
// A key may not be empty, and on SQLite and MySQL may not contain a double
// quote.
//
// Because the key is a bound parameter, PostgreSQL sees ($1) in the select
// list and ($2) in GROUP BY as different expressions and rejects a grouped
// extraction; group on a SQL fragment with a literal key instead.

// jsonFlags keeps the sensitive flag of the column operand and marks the
// result nullable, since an extraction is NULL for a missing key.
func jsonFlags(src Expr) uint16 {
	f := FlagNullable
	if s, ok := src.(interface{ Flags() uint16 }); ok {
		f |= s.Flags() & FlagSensitive
	}
	return f
}

// jsonKeysValid records ErrValidation and returns false for an empty path, an
// empty key, or (when quoteLimited) a key holding a double quote.
func jsonKeysValid(b *Builder, keys []string, quoteLimited bool) bool {
	if len(keys) == 0 {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("JSON path has no elements")})
		return false
	}
	for _, k := range keys {
		if k == "" {
			b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("JSON key is empty")})
			return false
		}
		if quoteLimited && strings.ContainsRune(k, '"') {
			b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf("JSON key %q contains a double quote, which %s paths cannot express", k, b.d.Name)})
			return false
		}
	}
	return true
}

// jsonPathString renders keys as a SQLite or MySQL JSON path: $."a"[0]."b".
func jsonPathString(keys []string) string {
	var sb strings.Builder
	sb.WriteByte('$')
	for _, k := range keys {
		if isDigits(k) {
			sb.WriteString("[" + k + "]")
			continue
		}
		sb.WriteString(`."` + k + `"`)
	}
	return sb.String()
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// JSONTextExpr is the text of a JSON member, an Expression[string] with the
// comparison and ordering methods of any scalar expression (Eq, In, Asc, Desc,
// IsNull and so on; pass it to Like for pattern matching). It is NULL when the
// member is missing or JSON null. Build it with JSONText or JSONPath.
type JSONTextExpr struct {
	exprOps[string]
	src  Expr
	keys []string
	path bool
}

func newJSONText(src Expr, keys []string, path bool) JSONTextExpr {
	e := JSONTextExpr{src: src, keys: keys, path: path}
	e.exprOps = selfOps[string](e)
	return e
}

// JSONText is the text of member key of the JSON object in c: col ->> 'key'
// on PostgreSQL. Compare it with the string methods, for example
// JSONText(after, "via").Eq("api"). key is bound as a parameter.
func JSONText[T any](c Expression[types.JSON[T]], key string) JSONTextExpr {
	return newJSONText(c, []string{key}, false)
}

// JSONPath is the text at the nested path keys of the JSON value in c:
// col #>> '{a,b}' on PostgreSQL. keys are bound as a single parameter there.
func JSONPath[T any](c Expression[types.JSON[T]], keys ...string) JSONTextExpr {
	return newJSONText(c, append([]string(nil), keys...), true)
}

// Flags returns the sensitive flag of the column operand plus FlagNullable.
func (e JSONTextExpr) Flags() uint16 { return jsonFlags(e.src) }

func (JSONTextExpr) resultType() string { return "" }

func (e JSONTextExpr) AppendSQL(b *Builder) {
	if !jsonKeysValid(b, e.keys, b.d.Name != dialect.Postgres) {
		return
	}
	switch b.d.Name {
	case dialect.Postgres:
		b.WriteString("(")
		e.src.AppendSQL(b)
		if e.path {
			b.WriteString(" #>> ")
			b.Arg(e.keys)
		} else {
			b.WriteString(" ->> ")
			b.Arg(e.keys[0])
		}
		b.WriteString(")")
	case dialect.SQLite:
		b.WriteString("CAST(json_extract(")
		e.src.AppendSQL(b)
		b.WriteString(", ")
		b.Arg(jsonPathString(e.keys))
		b.WriteString(") AS TEXT)")
	case dialect.MySQL:
		b.WriteString("JSON_UNQUOTE(JSON_EXTRACT(")
		e.src.AppendSQL(b)
		b.WriteString(", ")
		b.Arg(jsonPathString(e.keys))
		b.WriteString("))")
	default:
		b.unsupported("JSON text extraction")
	}
}

type jsonContains struct {
	src Expr
	val any
}

// JSONContains is true when the JSON value in c contains v: col @> v on
// PostgreSQL. v is encoded with encoding/json and bound as one parameter, so
// JSONContains(tags, []string{"a"}) matches a document whose array holds "a"
// and JSONContains(c, map[string]any{"via": "api"}) matches an object with
// that member. MySQL uses JSON_CONTAINS; SQLite has no containment operator
// and fails with ErrUnsupported.
func JSONContains[T any](c Expression[types.JSON[T]], v any) Predicate {
	return jsonContains{c, v}
}

// Flags returns the sensitive flag of the column operand.
func (p jsonContains) Flags() uint16 { return jsonFlags(p.src) &^ FlagNullable }

func (jsonContains) resultType() bool { return false }

func (p jsonContains) AppendSQL(b *Builder) {
	if b.d.Name == dialect.SQLite {
		b.unsupported("JSONContains")
		return
	}
	enc, err := json.Marshal(p.val)
	if err != nil {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf("encode JSON containment value: %w", err)})
		return
	}
	switch b.d.Name {
	case dialect.Postgres:
		b.WriteString("(")
		p.src.AppendSQL(b)
		b.WriteString(" @> CAST(")
		b.Arg(string(enc))
		b.WriteString(" AS jsonb))")
	case dialect.MySQL:
		b.WriteString("JSON_CONTAINS(")
		p.src.AppendSQL(b)
		b.WriteString(", CAST(")
		b.Arg(string(enc))
		b.WriteString(" AS JSON))")
	default:
		b.unsupported("JSONContains")
	}
}

type jsonHasKey struct {
	src Expr
	key string
}

// JSONHasKey is true when the top-level object in c has member key, even when
// its value is JSON null: col ? 'key' on PostgreSQL (which also matches a
// string element of a top-level array). key is bound as a parameter.
func JSONHasKey[T any](c Expression[types.JSON[T]], key string) Predicate {
	return jsonHasKey{c, key}
}

// Flags returns the sensitive flag of the column operand.
func (p jsonHasKey) Flags() uint16 { return jsonFlags(p.src) &^ FlagNullable }

func (jsonHasKey) resultType() bool { return false }

func (p jsonHasKey) AppendSQL(b *Builder) {
	if !jsonKeysValid(b, []string{p.key}, b.d.Name != dialect.Postgres) {
		return
	}
	switch b.d.Name {
	case dialect.Postgres:
		b.WriteString("(")
		p.src.AppendSQL(b)
		b.WriteString(" ? ")
		b.Arg(p.key)
		b.WriteString(")")
	case dialect.SQLite:
		b.WriteString("(json_type(")
		p.src.AppendSQL(b)
		b.WriteString(", ")
		b.Arg(jsonPathString([]string{p.key}))
		b.WriteString(") IS NOT NULL)")
	case dialect.MySQL:
		b.WriteString("JSON_CONTAINS_PATH(")
		p.src.AppendSQL(b)
		b.WriteString(", 'one', ")
		b.Arg(jsonPathString([]string{p.key}))
		b.WriteString(")")
	default:
		b.unsupported("JSONHasKey")
	}
}
