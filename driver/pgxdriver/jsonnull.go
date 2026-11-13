// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"database/sql"
	"reflect"
	"sync"

	"github.com/azrtydxb/momo/types"
)

var jsonFamilyType = reflect.TypeFor[types.JSONNuller]()

// jsonFamilyCache remembers, per destination element type *T, whether *T is a
// types.JSON[...], so a row of ordinary pointer columns costs one map lookup.
var jsonFamilyCache sync.Map // reflect.Type -> bool

func isJSONFamily(t reflect.Type) bool {
	if v, ok := jsonFamilyCache.Load(t); ok {
		return v.(bool)
	}
	ok := t.Implements(jsonFamilyType)
	jsonFamilyCache.Store(t, ok)
	return ok
}

// ptrScanner scans into a **T for a T that is a types.JSON. pgx's own plan for
// a pointer to a pointer on json and jsonb sets the pointer to nil for the
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

// wrapJSONNull replaces every destination of type **types.JSON[T] by a
// ptrScanner. It returns dest itself when nothing is replaced and otherwise a
// copy; it never changes the caller's slice. Other types, including user types
// with their own IsNull, keep pgx's own scan plan.
func wrapJSONNull(dest []any) []any {
	var out []any
	for i, d := range dest {
		rv := reflect.ValueOf(d)
		if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Pointer || !isJSONFamily(rv.Elem().Type()) {
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
