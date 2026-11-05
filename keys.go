// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"database/sql/driver"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// keyString encodes a relation key tuple as a comparable string. Each
// component is type-tagged so 1 and "1" differ, strings and bytes carry their
// length so a component containing the 0x1f separator cannot be confused with
// two components, and values are normalized so that a key built from a struct
// field (int, uint, named string or named integer type) matches the same key
// read back from the driver (int64, []byte, string, driver.Valuer output).
// Integers share one tag: an unsigned value that fits in int64 encodes like
// the signed value, larger ones keep a distinct tag.
func keyString(vals []any) string {
	var sb strings.Builder
	for i, v := range vals {
		if i > 0 {
			sb.WriteByte(0x1f)
		}
		writeKeyPart(&sb, v)
	}
	return sb.String()
}

func writeKeyString(sb *strings.Builder, s string) {
	sb.WriteString("s")
	sb.WriteString(strconv.Itoa(len(s)))
	sb.WriteByte(':')
	sb.WriteString(s)
}

func writeKeyInt(sb *strings.Builder, n int64) {
	sb.WriteString("i:")
	sb.WriteString(strconv.FormatInt(n, 10))
}

func writeKeyUint(sb *strings.Builder, n uint64) {
	if n <= math.MaxInt64 {
		writeKeyInt(sb, int64(n))
		return
	}
	sb.WriteString("u:")
	sb.WriteString(strconv.FormatUint(n, 10))
}

func writeKeyPart(sb *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		sb.WriteString("n:")
		return
	case string:
		writeKeyString(sb, x)
		return
	case []byte:
		writeKeyString(sb, string(x))
		return
	case time.Time:
		sb.WriteString("t:")
		sb.WriteString(x.UTC().Format(time.RFC3339Nano))
		return
	case driver.Valuer:
		vv, err := x.Value()
		if err != nil {
			sb.WriteString("e:")
			sb.WriteString(err.Error())
			return
		}
		writeKeyPart(sb, vv)
		return
	}
	// Builtin and named types reduce to their kind, so type UserID string and
	// type OrderID int64 match the plain values the driver returns.
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String:
		writeKeyString(sb, rv.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		writeKeyInt(sb, rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		writeKeyUint(sb, rv.Uint())
	case reflect.Bool:
		sb.WriteString("b:")
		sb.WriteString(strconv.FormatBool(rv.Bool()))
	case reflect.Float32, reflect.Float64:
		sb.WriteString("f:")
		sb.WriteString(strconv.FormatFloat(rv.Float(), 'g', -1, 64))
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			writeKeyString(sb, string(rv.Bytes()))
			return
		}
		fmt.Fprintf(sb, "x:%T:%v", v, v)
	case reflect.Pointer:
		if rv.IsNil() {
			sb.WriteString("n:")
			return
		}
		writeKeyPart(sb, rv.Elem().Interface())
	default:
		fmt.Fprintf(sb, "x:%T:%v", v, v)
	}
}
