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
// NullColumn, or an aliased column. Containment values and JSONHasKey keys are
// always bound parameters, and so are JSONText and JSONPath keys, except that a
// plain key on a non-sensitive column is inlined (see below). The expressions carry the
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
// renders json_extract and json_type. JSONContains is ErrUnsupported on SQLite
// by design: it has no containment operator, and json_each walks one level of
// members, which cannot reproduce the recursive subset rule of @> (nested
// objects, arrays inside arrays), so no partial emulation is offered. MySQL
// renders JSON_UNQUOTE(JSON_EXTRACT), JSON_CONTAINS and JSON_CONTAINS_PATH.
// SQLite runs against a real database through driver/sqlitedriver (cgo, not
// Windows); MySQL is not usable in this release and its SQL is golden-tested
// only. Extraction differs at the edges: SQLite casts json_extract to TEXT, so
// a JSON boolean reads as 1 or 0 instead of true or false, and MySQL returns
// the text null for a JSON null.
//
// Path elements. JSONText and JSONHasKey take one object key, always an
// object member even when it is numeric. In JSONPath, an element made only of
// digits addresses an array index on SQLite and MySQL, and an array index or an
// object key on PostgreSQL; any other element is an object key. A key may not
// be empty, and on SQLite and MySQL may not contain a double quote, a control
// character, or (SQLite) a backslash; MySQL escapes a backslash.
//
// A key or path made only of letters, digits and underscores is written into
// the SQL as a quoted literal in JSONText and JSONPath, so the same extraction
// in the select list and in GROUP BY is the same expression and grouping
// works. Any other key is a bound parameter, and PostgreSQL then rejects a
// grouped extraction (the parameters differ per clause); group on a SQL
// fragment with a literal key in that case. JSONHasKey and JSONContains always
// bind. Inlined keys make the statement text vary per key, so a key taken from
// user input grows the prepared-statement cache and pg_stat_statements; pass
// only keys from a fixed set.
//
// Masking. When the column operand is sensitive, every key, path and
// containment value is bound and recorded as sensitive, so hooks see
// RedactedArg; sensitivity beats inlining, so grouping by an extraction of a
// sensitive column fails like a non-plain key does.

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
		if quoteLimited && strings.IndexFunc(k, func(r rune) bool {
			return r == '"' || r < 0x20 || r == 0x7f || (r == '\\' && b.d.Name != dialect.MySQL)
		}) >= 0 {
			b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf("JSON key %q contains a character %s paths cannot express (double quote, control character or backslash)", k, b.d.Name)})
			return false
		}
	}
	return true
}

// jsonPathString renders keys as a SQLite or MySQL JSON path: $."a"[0]."b".
// With index set, an all-digit element is an array index; otherwise every
// element is a quoted object key. MySQL reads a backslash in a quoted label as
// an escape, so it is doubled.
func jsonPathString(d dialect.Name, keys []string, index bool) string {
	var sb strings.Builder
	sb.WriteByte('$')
	for _, k := range keys {
		if index && isDigits(k) {
			sb.WriteString("[" + k + "]")
			continue
		}
		if d == dialect.MySQL {
			k = strings.ReplaceAll(k, `\`, `\\`)
		}
		sb.WriteString(`."` + k + `"`)
	}
	return sb.String()
}

// plainKey reports whether every key is non-empty letters, digits and
// underscores, which is safe to write as a SQL literal.
func plainKey(keys []string) bool {
	for _, k := range keys {
		if k == "" {
			return false
		}
		for _, r := range k {
			if r != '_' && (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
				return false
			}
		}
	}
	return len(keys) > 0
}

// argOrLiteral writes lit as a quoted literal when keys are plain, otherwise
// binds v with flags.
func (e JSONTextExpr) argOrLiteral(b *Builder, flags uint16, lit string, v any) {
	if plainKey(e.keys) && flags&FlagSensitive == 0 {
		b.WriteString("'" + lit + "'")
		return
	}
	b.argFor(flags, v)
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
// JSONText(after, "via").Eq("api"). key is a bound parameter, or an inlined
// literal when it is plain and the column is not sensitive.
func JSONText[T any](c Expression[types.JSON[T]], key string) JSONTextExpr {
	return newJSONText(c, []string{key}, false)
}

// JSONPath is the text at the nested path keys of the JSON value in c:
// col #>> '{a,b}' on PostgreSQL. keys are one bound array parameter there, or an inlined literal when every
// element is plain and the column is not sensitive.
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
		fl := b.renderSens(e.src, flagsOf(e.src))
		if e.path {
			b.WriteString(" #>> ")
			e.argOrLiteral(b, fl, "{"+strings.Join(e.keys, ",")+"}", e.keys)
		} else {
			b.WriteString(" ->> ")
			e.argOrLiteral(b, fl, e.keys[0], e.keys[0])
		}
		b.WriteString(")")
	case dialect.SQLite:
		b.WriteString("CAST(json_extract(")
		fl := b.renderSens(e.src, flagsOf(e.src))
		b.WriteString(", ")
		path := jsonPathString(b.d.Name, e.keys, e.path)
		e.argOrLiteral(b, fl, path, path)
		b.WriteString(") AS TEXT)")
	case dialect.MySQL:
		b.WriteString("JSON_UNQUOTE(JSON_EXTRACT(")
		fl := b.renderSens(e.src, flagsOf(e.src))
		b.WriteString(", ")
		path := jsonPathString(b.d.Name, e.keys, e.path)
		e.argOrLiteral(b, fl, path, path)
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
		fl := b.renderSens(p.src, flagsOf(p.src))
		b.WriteString(" @> CAST(")
		b.argFor(fl, string(enc))
		b.WriteString(" AS jsonb))")
	case dialect.MySQL:
		b.WriteString("JSON_CONTAINS(")
		fl := b.renderSens(p.src, flagsOf(p.src))
		b.WriteString(", CAST(")
		b.argFor(fl, string(enc))
		b.WriteString(" AS JSON))")
	default:
		b.unsupported("JSONContains")
	}
}

type jsonHasKey struct {
	src Expr
	key string
}

// JSONHasKey is true when c has member key, even when its value is JSON null:
// col ? 'key' on PostgreSQL. PostgreSQL's operator also matches a string
// element of a top-level array and a top-level scalar string equal to key. A
// SQL NULL column yields NULL, so Not drops those rows too. key is always a
// bound parameter and is always an object key, never an array index.
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
		fl := b.renderSens(p.src, flagsOf(p.src))
		b.WriteString(" ? ")
		b.argFor(fl, p.key)
		b.WriteString(")")
	case dialect.SQLite:
		b.WriteString("(json_type(")
		fl := b.renderSens(p.src, flagsOf(p.src))
		b.WriteString(", ")
		b.argFor(fl, jsonPathString(b.d.Name, []string{p.key}, false))
		b.WriteString(") IS NOT NULL)")
	case dialect.MySQL:
		b.WriteString("JSON_CONTAINS_PATH(")
		fl := b.renderSens(p.src, flagsOf(p.src))
		b.WriteString(", 'one', ")
		b.argFor(fl, jsonPathString(b.d.Name, []string{p.key}, false))
		b.WriteString(")")
	default:
		b.unsupported("JSONHasKey")
	}
}
