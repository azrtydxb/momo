// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"database/sql"
	"reflect"
)

// jsonNuller is a column type that can hold the JSON literal null apart from
// SQL NULL (types.JSON).
type jsonNuller interface {
	sql.Scanner
	IsNull() bool
}

var jsonNullerType = reflect.TypeFor[jsonNuller]()

// ptrScanner scans into a **T for a T that is a jsonNuller. pgx's own plan
// for a pointer to a pointer on json and jsonb sets the pointer to nil for the
// JSON literal null, as it does for SQL NULL. Going through sql.Scanner keeps
// the two apart: SQL NULL gives a nil pointer, the JSON literal null a
// non-nil pointer to a value that reports IsNull.
type ptrScanner struct{ dst reflect.Value } // dst is the *T, settable

func (p ptrScanner) Scan(src any) error {
	if src == nil {
		p.dst.SetZero()
		return nil
	}
	v := reflect.New(p.dst.Type().Elem())
	if err := v.Interface().(sql.Scanner).Scan(src); err != nil {
		return err
	}
	p.dst.Set(v)
	return nil
}

// wrapJSONNull replaces every destination of type **T, where *T is a
// jsonNuller, by a ptrScanner. It copies dest only when it replaces one.
func wrapJSONNull(dest []any) []any {
	var out []any
	for i, d := range dest {
		rv := reflect.ValueOf(d)
		if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Pointer || !rv.Elem().Type().Implements(jsonNullerType) {
			continue
		}
		if out == nil {
			out = append([]any(nil), dest...)
		}
		out[i] = ptrScanner{dst: rv.Elem()}
	}
	if out == nil {
		return dest
	}
	return out
}
