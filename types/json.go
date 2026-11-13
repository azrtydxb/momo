// SPDX-License-Identifier: Apache-2.0

package types

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"reflect"

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
type JSON[T any] struct {
	V T
	// null records that the last Scan read the JSON literal null. It only
	// counts while V is still the zero value of T.
	null bool
}

// JSONNull returns a JSON[T] that is the JSON literal null: it writes null,
// not SQL NULL (a nil *JSON[T] writes SQL NULL) and IsNull reports true.
func JSONNull[T any]() JSON[T] { return JSON[T]{null: true} }

// IsNull reports whether j is the JSON literal null: it was scanned from null,
// or built by JSONNull, and V has not been changed since. A value whose V
// merely encodes as null (a nil map, slice or pointer) is not reported; its
// Value is still null.
func (j JSON[T]) IsNull() bool { return j.null && reflect.ValueOf(&j.V).Elem().IsZero() }

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
// JSON literal null decodes to the zero value of T and makes IsNull true.
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
	j.null = bytes.Equal(bytes.TrimSpace([]byte(s)), []byte("null"))
	return nil
}
