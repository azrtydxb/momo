// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"fmt"
	"reflect"
	"strings"
)

// convertUUIDSliceArgs turns a slice of UUID values into a PostgreSQL array
// literal string, so it binds to "= ANY($1::uuid[])" (or a uuid[] column)
// under the default pgx.QueryExecModeExec. In that mode pgx sends no parameter
// types and encodes each value from its Go type; a []uuid.UUID or
// []types.UUID has no pgx type of its own, so without this step the call fails
// with "failed to encode args". An element counts as a UUID when it is a
// [16]byte-based array type with a String method, which both
// github.com/google/uuid and momo's types.UUID are; the String form is the
// canonical lowercase text. A nil slice stays NULL and an empty one becomes
// "{}". It returns args itself when nothing needed converting.
func convertUUIDSliceArgs(args []any) []any {
	var out []any
	for i, a := range args {
		s, ok := uuidArrayLiteral(a)
		if !ok {
			continue
		}
		if out == nil {
			out = append([]any(nil), args...)
		}
		out[i] = s
	}
	if out == nil {
		return args
	}
	return out
}

var stringerType = reflect.TypeOf((*fmt.Stringer)(nil)).Elem()

// uuidArrayLiteral reports whether a is a slice of UUID values and, if so,
// returns its array literal. A nil slice yields (nil, true).
func uuidArrayLiteral(a any) (any, bool) {
	if a == nil {
		return nil, false
	}
	v := reflect.ValueOf(a)
	if v.Kind() != reflect.Slice {
		return nil, false
	}
	et := v.Type().Elem()
	if et.Kind() != reflect.Array || et.Len() != 16 || et.Elem().Kind() != reflect.Uint8 || !et.Implements(stringerType) {
		return nil, false
	}
	if v.IsNil() {
		return nil, true
	}
	var b strings.Builder
	b.WriteByte('{')
	for i := range v.Len() {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(v.Index(i).Interface().(fmt.Stringer).String())
	}
	b.WriteByte('}')
	return b.String(), true
}
