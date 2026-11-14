// SPDX-License-Identifier: Apache-2.0

// Package jsonop holds the JSON operators over types.JSON[T] columns (jsonb on
// PostgreSQL): text extraction, nested paths, containment and key tests. The
// package builds on the exported extension API of the root package (Ops,
// PredicateFunc, Builder.ArgFor, Builder.RenderSensitive, Builder.Unsupported
// and FlagsOf).
//
// The column operand is any Expression of types.JSON[T]: a Column, a
// NullColumn, or an aliased column. Containment values and HasKey keys are
// always bound parameters, and so are Text and Path keys, except that a plain
// key on a non-sensitive column is inlined (see below). The extraction carries
// the column's FlagSensitive, so a sensitive JSON column stays sensitive
// through extraction (see TextExpr.Flags).
//
// SQL NULL versus JSON null. A nullable JSON column has two distinct empty
// values: SQL NULL (no value; col IS NULL) and the JSON literal null (a value
// that encodes as "null", for example a nil map or pointer in types.JSON).
// Text and Path yield SQL NULL for both of them, for a missing key and for a
// key whose value is JSON null, so extracted text cannot tell the cases
// apart; test the column itself with IsNull for SQL NULL, or compare it with a
// types.JSON whose V is nil for JSON null. HasKey is true for a key whose
// value is JSON null and false for a missing key.
//
// Dialects. PostgreSQL uses the jsonb operators ->>, #>>, @> and ?. SQLite
// renders json_extract and json_type. Contains is ErrUnsupported on SQLite by
// design: it has no containment operator, and json_each walks one level of
// members, which cannot reproduce the recursive subset rule of @> (nested
// objects, arrays inside arrays), so no partial emulation is offered. MySQL
// renders JSON_UNQUOTE(JSON_EXTRACT), JSON_CONTAINS and JSON_CONTAINS_PATH.
// SQLite runs against a real database through driver/sqlitedriver (cgo, not
// Windows) and driver/sqlitepure. MySQL and MariaDB run against MySQL 8.4,
// MySQL 9.7 and MariaDB 11.8 in the integration tests (testopen.EachMySQL),
// except Concat and Set, which are ErrUnsupported there. Extraction differs at
// the edges: SQLite casts json_extract to TEXT, so a JSON boolean reads as 1
// or 0 instead of true or false, and MySQL returns the text null for a JSON
// null. MariaDB has no JSON type for CAST, so Contains passes the candidate as
// JSON text there.
//
// Type names. TypeOf gives the JSON type of a value as one of the six names of
// PostgreSQL's jsonb_typeof (object, array, string, number, boolean, null) on
// every dialect; SQLite's json_type and MySQL's JSON_TYPE names are mapped. It
// is the one function here with integration tests on MySQL 8.4, MySQL 9.7 and
// MariaDB 11.8.
//
// Path elements. Text and HasKey take one object key, always an object member
// even when it is numeric. In Path, an element made only of digits addresses
// an array index on SQLite and MySQL, and an array index or an object key on
// PostgreSQL; any other element is an object key. A key may not be empty, and
// on SQLite and MySQL may not contain a double quote, a control character, or
// (SQLite) a backslash; MySQL escapes a backslash.
//
// A key or path made only of letters, digits and underscores is written into
// the SQL as a quoted literal in Text and Path, so the same extraction in the
// select list and in GROUP BY is the same expression and grouping works. Any
// other key is a bound parameter, and PostgreSQL then rejects a grouped
// extraction (the parameters differ per clause); group on a SQL fragment with
// a literal key in that case. HasKey and Contains always bind. Inlined keys
// make the statement text vary per key, so a key taken from user input grows
// the prepared-statement cache and pg_stat_statements; pass only keys from a
// fixed set.
//
// Masking. When the column operand is sensitive, every key, path and
// containment value is bound and recorded as sensitive, so hooks see
// RedactedArg; sensitivity beats inlining, so grouping by an extraction of a
// sensitive column fails like a non-plain key does.
package jsonop

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

// flags keeps the sensitive flag of the column operand and marks the result
// nullable, since an extraction is NULL for a missing key.
func flags(src momo.Expr) uint16 {
	return momo.FlagNullable | momo.FlagsOf(src)&momo.FlagSensitive
}

func validation(err error) error {
	return &momo.Error{Kind: momo.ErrValidation, Op: "build", Err: err}
}

// keysValid records ErrValidation and returns false for an empty path, an
// empty key, or (when quoteLimited) a key holding a double quote.
func keysValid(b *momo.Builder, keys []string, quoteLimited bool) bool {
	if len(keys) == 0 {
		b.SetErr(validation(errors.New("JSON path has no elements")))
		return false
	}
	name := b.Dialect().Name
	for _, k := range keys {
		if k == "" {
			b.SetErr(validation(errors.New("JSON key is empty")))
			return false
		}
		if quoteLimited && strings.IndexFunc(k, func(r rune) bool {
			return r == '"' || r < 0x20 || r == 0x7f || (r == '\\' && name != dialect.MySQL)
		}) >= 0 {
			b.SetErr(validation(fmt.Errorf("JSON key %q contains a character %s paths cannot express (double quote, control character or backslash)", k, name)))
			return false
		}
	}
	return true
}

// pathString renders keys as a SQLite or MySQL JSON path: $."a"[0]."b". With
// index set, an all-digit element is an array index; otherwise every element
// is a quoted object key. MySQL reads a backslash in a quoted label as an
// escape, so it is doubled.
func pathString(d dialect.Name, keys []string, index bool) string {
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

// argOrLiteral writes lit as a quoted literal when keys are plain and the
// column is not sensitive, otherwise binds v with fl.
func argOrLiteral(b *momo.Builder, keys []string, fl uint16, lit string, v any) {
	if plainKey(keys) && fl&momo.FlagSensitive == 0 {
		b.WriteString("'" + lit + "'")
		return
	}
	b.ArgFor(fl, v)
}

// TextExpr is the text of a JSON member, an Expression[string] with the
// comparison and ordering methods of any scalar expression (Eq, In, Asc, Desc,
// IsNull and so on; pass it to Like for pattern matching). It is NULL when the
// member is missing or JSON null. Build it with Text or Path.
type TextExpr struct {
	momo.Ops[string]
	src  momo.Expr
	keys []string
	path bool
	root bool
}

func newText(src momo.Expr, keys []string, path bool) TextExpr {
	e := TextExpr{src: src, keys: keys, path: path}
	e.Ops = momo.NewOps[string](e)
	return e
}

// Text is the text of member key of the JSON object in c: col ->> 'key' on
// PostgreSQL. Compare it with the string methods, for example
// Text(after, "via").Eq("api"). key is a bound parameter, or an inlined
// literal when it is plain and the column is not sensitive.
func Text[T any](c momo.Expression[types.JSON[T]], key string) TextExpr {
	return newText(c, []string{key}, false)
}

// Path is the text at the nested path keys of the JSON value in c:
// col #>> '{a,b}' on PostgreSQL. keys are one bound array parameter there, or
// an inlined literal when every element is plain and the column is not
// sensitive.
func Path[T any](c momo.Expression[types.JSON[T]], keys ...string) TextExpr {
	return newText(c, append([]string(nil), keys...), true)
}

// Root is the text of the JSON value stored at the root of c, for a column
// that holds a scalar (a number, string or boolean) instead of an object:
// col #>> '{}' on PostgreSQL, json_extract(col, '$') on SQLite and
// JSON_UNQUOTE(JSON_EXTRACT(col, '$')) on MySQL. Cast it for a numeric read,
// for example sqlfn.Cast[int](Root(setting), "int"). It is NULL for SQL NULL and for JSON
// null. An object or array at the root reads as its JSON text. The path
// written into the SQL holds no caller data, so it is always an inlined
// literal, and the extraction groups like a plain key does.
func Root[T any](c momo.Expression[types.JSON[T]]) TextExpr {
	e := TextExpr{src: c, path: true, root: true}
	e.Ops = momo.NewOps[string](e)
	return e
}

// Flags returns the sensitive flag of the column operand plus FlagNullable.
func (e TextExpr) Flags() uint16 { return flags(e.src) }

// AppendSQL renders the extraction. Other dialects than PostgreSQL, SQLite and
// MySQL fail the build with ErrUnsupported.
func (e TextExpr) AppendSQL(b *momo.Builder) {
	name := b.Dialect().Name
	if e.root {
		e.appendRoot(b)
		return
	}
	if !keysValid(b, e.keys, name != dialect.Postgres) {
		return
	}
	switch name {
	case dialect.Postgres:
		b.WriteString("(")
		fl := b.RenderSensitive(e.src, momo.FlagsOf(e.src))
		if e.path {
			b.WriteString(" #>> ")
			argOrLiteral(b, e.keys, fl, "{"+strings.Join(e.keys, ",")+"}", e.keys)
		} else {
			b.WriteString(" ->> ")
			argOrLiteral(b, e.keys, fl, e.keys[0], e.keys[0])
		}
		b.WriteString(")")
	case dialect.SQLite:
		b.WriteString("CAST(json_extract(")
		fl := b.RenderSensitive(e.src, momo.FlagsOf(e.src))
		b.WriteString(", ")
		path := pathString(name, e.keys, e.path)
		argOrLiteral(b, e.keys, fl, path, path)
		b.WriteString(") AS TEXT)")
	case dialect.MySQL:
		b.WriteString("JSON_UNQUOTE(JSON_EXTRACT(")
		fl := b.RenderSensitive(e.src, momo.FlagsOf(e.src))
		b.WriteString(", ")
		path := pathString(name, e.keys, e.path)
		argOrLiteral(b, e.keys, fl, path, path)
		b.WriteString("))")
	default:
		b.Unsupported("JSON text extraction")
	}
}

// appendRoot renders the root extraction of Root.
func (e TextExpr) appendRoot(b *momo.Builder) {
	switch b.Dialect().Name {
	case dialect.Postgres:
		b.WriteString("(")
		b.RenderSensitive(e.src, momo.FlagsOf(e.src))
		b.WriteString(" #>> '{}')")
	case dialect.SQLite:
		b.WriteString("CAST(json_extract(")
		b.RenderSensitive(e.src, momo.FlagsOf(e.src))
		b.WriteString(", '$') AS TEXT)")
	case dialect.MySQL:
		b.WriteString("JSON_UNQUOTE(JSON_EXTRACT(")
		b.RenderSensitive(e.src, momo.FlagsOf(e.src))
		b.WriteString(", '$'))")
	default:
		b.Unsupported("JSON root extraction")
	}
}

// Contains is true when the JSON value in c contains v: col @> v on
// PostgreSQL. v is encoded with encoding/json and bound as one parameter, so
// Contains(tags, []string{"a"}) matches a document whose array holds "a" and
// Contains(c, map[string]any{"via": "api"}) matches an object with that
// member. MySQL uses JSON_CONTAINS; SQLite has no containment operator and
// fails with ErrUnsupported, as does any other dialect.
func Contains[T any](c momo.Expression[types.JSON[T]], v any) momo.Predicate {
	return momo.PredicateFunc(func(b *momo.Builder) {
		name := b.Dialect().Name
		if name == dialect.SQLite {
			b.Unsupported("JSON containment (Contains)")
			return
		}
		enc, err := json.Marshal(v)
		if err != nil {
			b.SetErr(validation(fmt.Errorf("encode JSON containment value: %w", err)))
			return
		}
		switch name {
		case dialect.Postgres:
			b.WriteString("(")
			fl := b.RenderSensitive(c, momo.FlagsOf(c))
			b.WriteString(" @> CAST(")
			b.ArgFor(fl, string(enc))
			b.WriteString(" AS jsonb))")
		case dialect.MySQL:
			b.WriteString("JSON_CONTAINS(")
			fl := b.RenderSensitive(c, momo.FlagsOf(c))
			if b.Dialect().Flavor == dialect.FlavorMariaDB {
				// MariaDB has no JSON target in CAST (a syntax error), and its
				// JSON_CONTAINS takes the candidate as JSON text.
				b.WriteString(", ")
				b.ArgFor(fl, string(enc))
				b.WriteString(")")
				break
			}
			b.WriteString(", CAST(")
			b.ArgFor(fl, string(enc))
			b.WriteString(" AS JSON))")
		default:
			b.Unsupported("JSON containment (Contains)")
		}
	})
}

// HasKey is true when c has member key, even when its value is JSON null:
// col ? 'key' on PostgreSQL. PostgreSQL's operator also matches a string
// element of a top-level array and a top-level scalar string equal to key. A
// SQL NULL column yields NULL, so Not drops those rows too. key is always a
// bound parameter and is always an object key, never an array index.
func HasKey[T any](c momo.Expression[types.JSON[T]], key string) momo.Predicate {
	return momo.PredicateFunc(func(b *momo.Builder) {
		name := b.Dialect().Name
		if !keysValid(b, []string{key}, name != dialect.Postgres) {
			return
		}
		switch name {
		case dialect.Postgres:
			b.WriteString("(")
			fl := b.RenderSensitive(c, momo.FlagsOf(c))
			b.WriteString(" ? ")
			b.ArgFor(fl, key)
			b.WriteString(")")
		case dialect.SQLite:
			b.WriteString("(json_type(")
			fl := b.RenderSensitive(c, momo.FlagsOf(c))
			b.WriteString(", ")
			b.ArgFor(fl, pathString(name, []string{key}, false))
			b.WriteString(") IS NOT NULL)")
		case dialect.MySQL:
			b.WriteString("JSON_CONTAINS_PATH(")
			fl := b.RenderSensitive(c, momo.FlagsOf(c))
			b.WriteString(", 'one', ")
			b.ArgFor(fl, pathString(name, []string{key}, false))
			b.WriteString(")")
		default:
			b.Unsupported("JSON key test (HasKey)")
		}
	})
}
