// SPDX-License-Identifier: Apache-2.0

package types

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// JSON[T] stores any JSON-encodable T: jsonb on PostgreSQL, TEXT on SQLite.
// A nil map, slice or pointer encodes as the JSON literal null, an empty one
// as {} or []; Scan restores the same distinction.
//
// A scanned JSON literal null also sets a private marker, reported by IsNull,
// so that writing the value back stores null and not the encoding of the zero
// T (for a struct T that would be an object). On a nullable column, scan into
// *JSON[T]: SQL NULL gives a nil pointer and the JSON literal null gives a
// non-nil pointer whose IsNull is true. Scanning SQL NULL into a plain
// JSON[T] field stays an error. On SQLite and MySQL, where the column is
// text, any stored text that is null once white space is trimmed counts as the
// JSON literal null.
//
// The marker is a private field, so two JSON[T] values compare equal with ==
// only when it matches: a scanned null differs from JSON[T]{}. With go-cmp,
// pass an option or use an Equal method. JSON[T] has no MarshalJSON, so
// encoding/json renders any JSON[T] as {"V":...} and JSONNull[T]() as
// {"V":null}; the null state does not survive an encoding/json round trip.
type JSON[T any] struct {
	V T
	// null records that the last Scan read the JSON literal null. It counts
	// while V is the zero value of T or still encodes as null.
	null bool
}

// JSONNuller is implemented by JSON[T] and by no other type. It lets another
// package recognise the family without naming T; it has no method to call.
type JSONNuller interface {
	sql.Scanner
	IsNull() bool
	jsonFamily()
}

func (JSON[T]) jsonFamily() {}

// JSONNull returns a JSON[T] that is the JSON literal null: it writes null,
// not SQL NULL (a nil *JSON[T] writes SQL NULL) and IsNull reports true.
func JSONNull[T any]() JSON[T] { return JSON[T]{null: true} }

// Set assigns v and clears the JSON null state. After reading a JSON null,
// assign through Set or a fresh JSON[T]{V: v}: assigning j.V directly to the
// zero value of T (false, 0, "", an empty struct) leaves j a JSON null, because
// the zero value cannot be told apart from an untouched V.
func (j *JSON[T]) Set(v T) {
	j.V = v
	j.null = false
}

// IsNull reports whether j is the JSON literal null: it was scanned from null,
// or built by JSONNull, and it has not been assigned through Set since. It
// stays true while V is the zero value of T or encodes as null (a
// json.RawMessage holding null, or a T with its own UnmarshalJSON). Assigning
// V directly to a non-zero value ends it; assigning the zero value does not,
// so use Set. A value whose V merely encodes as null (a nil map, slice or
// pointer) and was not read from null is not reported; its Value is still
// null.
func (j JSON[T]) IsNull() bool {
	if !j.null {
		return false
	}
	if reflect.ValueOf(&j.V).Elem().IsZero() {
		return true
	}
	b, err := json.Marshal(j.V)
	return err == nil && string(b) == "null"
}

// SQLType returns the column type for d.
func (JSON[T]) SQLType(d dialect.Name) string {
	if d == dialect.Postgres {
		return "jsonb"
	}
	return "TEXT"
}

// Value encodes V as JSON text.
func (j JSON[T]) Value() (driver.Value, error) {
	if j.IsNull() {
		return "null", nil
	}
	b, err := json.Marshal(j.V)
	if err != nil {
		return nil, fmt.Errorf("types: encode JSON: %w", err)
	}
	return string(b), nil
}

// Scan decodes JSON text (string or []byte) into V. NULL is an error; the
// JSON literal null decodes to the zero value of T (or what T's own decoder
// makes of null) and makes IsNull true.
func (j *JSON[T]) Scan(src any) error {
	s, err := text("JSON", src)
	if err != nil {
		return err
	}
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return fmt.Errorf("types: decode JSON: %w", err)
	}
	j.V = v
	j.null = strings.TrimSpace(s) == "null"
	return nil
}
