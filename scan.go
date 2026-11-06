// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"
)

// scanErr builds the ErrValidation error ScanValue returns.
func scanErr(err error) error {
	return &Error{Kind: ErrValidation, Op: "scan", Err: err}
}

// ScanValue assigns the driver value src to the field behind dst. The type
// switch is over concrete field types, so the join path needs no reflection.
//
// Supported destinations are *string, *int64, *int, *int32, *int16, *bool,
// *float64, *float32, *[]byte and *time.Time, the same types behind a second
// pointer (**T), and any sql.Scanner. A nil src sets a **T or *[]byte to nil
// and is passed to a sql.Scanner; a nil src into any other destination, an
// unconvertible src and an unsupported destination type fail with an error
// matching ErrValidation (Op "scan").
func ScanValue(dst any, src any) error {
	if src == nil {
		return scanNull(dst)
	}
	switch d := dst.(type) {
	case *string:
		return scanVal(d, src, asString)
	case **string:
		return scanPtr(d, src, asString)
	case *int64:
		return scanVal(d, src, asInt64)
	case **int64:
		return scanPtr(d, src, asInt64)
	case *int:
		return scanVal(d, src, asInt)
	case **int:
		return scanPtr(d, src, asInt)
	case *int32:
		return scanVal(d, src, asInt32)
	case **int32:
		return scanPtr(d, src, asInt32)
	case *int16:
		return scanVal(d, src, asInt16)
	case **int16:
		return scanPtr(d, src, asInt16)
	case *bool:
		return scanVal(d, src, asBool)
	case **bool:
		return scanPtr(d, src, asBool)
	case *float64:
		return scanVal(d, src, asFloat64)
	case **float64:
		return scanPtr(d, src, asFloat64)
	case *float32:
		return scanVal(d, src, asFloat32)
	case **float32:
		return scanPtr(d, src, asFloat32)
	case *[]byte:
		return scanVal(d, src, asBytes)
	case *time.Time:
		return scanVal(d, src, asTime)
	case **time.Time:
		return scanPtr(d, src, asTime)
	}
	if s, ok := dst.(sql.Scanner); ok {
		return s.Scan(src)
	}
	return scanErr(fmt.Errorf("unsupported destination type %T", dst))
}

// scanNull handles a NULL source.
func scanNull(dst any) error {
	switch d := dst.(type) {
	case **string:
		*d = nil
	case **int64:
		*d = nil
	case **int:
		*d = nil
	case **int32:
		*d = nil
	case **int16:
		*d = nil
	case **bool:
		*d = nil
	case **float64:
		*d = nil
	case **float32:
		*d = nil
	case **time.Time:
		*d = nil
	case *[]byte:
		*d = nil
	default:
		if s, ok := dst.(sql.Scanner); ok {
			return s.Scan(nil)
		}
		// Named slice, map and interface kinds hold NULL as their zero value.
		if v := reflect.ValueOf(dst); v.Kind() == reflect.Pointer && !v.IsNil() {
			switch v.Elem().Kind() {
			case reflect.Slice, reflect.Map, reflect.Interface, reflect.Pointer:
				v.Elem().SetZero()
				return nil
			}
		}
		return scanErr(errors.New("NULL into a non-nullable field"))
	}
	return nil
}

// scanVal converts src into *d; *d is left alone on error.
func scanVal[T any](d *T, src any, conv func(any) (T, error)) error {
	v, err := conv(src)
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// scanPtr converts src and points *d at a fresh copy; *d is left alone on error.
func scanPtr[T any](d **T, src any, conv func(any) (T, error)) error {
	v, err := conv(src)
	if err != nil {
		return err
	}
	*d = &v
	return nil
}

func asString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case []byte:
		return string(x), nil
	}
	return "", scanErr(fmt.Errorf("cannot convert %T to string", v))
}

func asInt64(v any) (int64, error) {
	switch x := v.(type) {
	case int64:
		return x, nil
	case int:
		return int64(x), nil
	case int32:
		return int64(x), nil
	case int16:
		return int64(x), nil
	case int8:
		return int64(x), nil
	case uint8:
		return int64(x), nil
	case uint16:
		return int64(x), nil
	case uint32:
		return int64(x), nil
	}
	return 0, scanErr(fmt.Errorf("cannot convert %T to an integer", v))
}

// asIntN converts v to an integer type bounded by [lo, hi]; an out-of-range
// value is a validation error rather than a silent wrap.
func asIntN[T int | int32 | int16](v any, lo, hi int64) (T, error) {
	n, err := asInt64(v)
	if err != nil {
		return 0, err
	}
	if n < lo || n > hi {
		return 0, scanErr(fmt.Errorf("integer %d out of range", n))
	}
	return T(n), nil // #nosec G115 -- range checked above
}

func asInt(v any) (int, error)     { return asIntN[int](v, math.MinInt, math.MaxInt) }
func asInt32(v any) (int32, error) { return asIntN[int32](v, math.MinInt32, math.MaxInt32) }
func asInt16(v any) (int16, error) { return asIntN[int16](v, math.MinInt16, math.MaxInt16) }

func asBool(v any) (bool, error) {
	if x, ok := v.(bool); ok {
		return x, nil
	}
	return false, scanErr(fmt.Errorf("cannot convert %T to bool", v))
}

func asFloat64(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	}
	return 0, scanErr(fmt.Errorf("cannot convert %T to float", v))
}

func asFloat32(v any) (float32, error) { f, err := asFloat64(v); return float32(f), err }

func asBytes(v any) ([]byte, error) {
	switch x := v.(type) {
	case []byte:
		return append([]byte(nil), x...), nil
	case string:
		return []byte(x), nil
	}
	return nil, scanErr(fmt.Errorf("cannot convert %T to []byte", v))
}

func asTime(v any) (time.Time, error) {
	if x, ok := v.(time.Time); ok {
		return x, nil
	}
	return time.Time{}, scanErr(fmt.Errorf("cannot convert %T to time.Time", v))
}
