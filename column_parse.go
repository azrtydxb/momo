// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

// ParseEq parses raw with the column's Go type and returns the equality
// predicate on it. A value that does not parse is an *Error with Kind
// ErrValidation; the error does not echo raw, which may be attacker
// controlled.
func (c Column[T]) ParseEq(raw string) (Predicate, error) { return c.ParseCmp("eq", raw) }

// ParseCmp parses raw with the column's Go type and returns the comparison
// predicate for op, one of eq, ne, lt, le, gt and ge. An unknown op or a value
// that does not parse is an *Error with Kind ErrValidation. It is meant for a
// filter value that arrives as text from outside; the column comes from the
// generated ColumnByName allowlist.
func (c Column[T]) ParseCmp(op, raw string) (Predicate, error) {
	switch op {
	case "eq", "ne", "lt", "le", "gt", "ge":
	default:
		return nil, &Error{Kind: ErrValidation, Op: "parse", Column: c.name, Err: errors.New("unknown comparison operator")}
	}
	v, err := parseText[T](raw)
	if err != nil {
		return nil, &Error{Kind: ErrValidation, Op: "parse", Column: c.name, Err: err}
	}
	switch op {
	case "eq":
		return c.Eq(v), nil
	case "ne":
		return c.Ne(v), nil
	case "lt":
		return c.Lt(v), nil
	case "le":
		return c.Le(v), nil
	case "gt":
		return c.Gt(v), nil
	}
	return c.Ge(v), nil
}

// ParseEq is Column.ParseEq.
func (c ReadOnlyColumn[T]) ParseEq(raw string) (Predicate, error) { return c.col.ParseEq(raw) }

// ParseCmp is Column.ParseCmp.
func (c ReadOnlyColumn[T]) ParseCmp(op, raw string) (Predicate, error) {
	return c.col.ParseCmp(op, raw)
}

// ParseEq is Column.ParseEq.
func (c NullReadOnlyColumn[T]) ParseEq(raw string) (Predicate, error) { return c.col.ParseEq(raw) }

// ParseCmp is Column.ParseCmp.
func (c NullReadOnlyColumn[T]) ParseCmp(op, raw string) (Predicate, error) {
	return c.col.ParseCmp(op, raw)
}

// ParseEq is Column.ParseEq.
func (c CreateOnlyColumn[T]) ParseEq(raw string) (Predicate, error) { return c.col.ParseEq(raw) }

// ParseCmp is Column.ParseCmp.
func (c CreateOnlyColumn[T]) ParseCmp(op, raw string) (Predicate, error) {
	return c.col.ParseCmp(op, raw)
}

// ParseEq is Column.ParseEq.
func (c ImmutableColumn[T]) ParseEq(raw string) (Predicate, error) { return c.col.ParseEq(raw) }

// ParseCmp is Column.ParseCmp.
func (c ImmutableColumn[T]) ParseCmp(op, raw string) (Predicate, error) {
	return c.col.ParseCmp(op, raw)
}

var errVectorFilter = errors.New("a vector column cannot be filtered by a text value")

// ParseEq always fails: a vector has no text form to filter by.
func (c vectorBase[V]) ParseEq(string) (Predicate, error) {
	return nil, &Error{Kind: ErrValidation, Op: "parse", Column: c.col.name, Err: errVectorFilter}
}

// ParseCmp always fails: a vector has no text form to filter by.
func (c vectorBase[V]) ParseCmp(string, string) (Predicate, error) {
	return nil, &Error{Kind: ErrValidation, Op: "parse", Column: c.col.name, Err: errVectorFilter}
}

// parseText parses raw into T: integers, floats, bools and strings by their
// kind (so named types work), and any type whose pointer implements
// encoding.TextUnmarshaler, which covers time.Time (RFC 3339).
func parseText[T any](raw string) (T, error) {
	var v T
	if tu, ok := any(&v).(encoding.TextUnmarshaler); ok {
		if err := tu.UnmarshalText([]byte(raw)); err != nil {
			return v, fmt.Errorf("value is not a valid %s", reflect.TypeFor[T]())
		}
		return v, nil
	}
	rv := reflect.ValueOf(&v).Elem()
	t := rv.Type()
	bad := fmt.Errorf("value is not a valid %s", t)
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, t.Bits())
		if err != nil {
			return v, bad
		}
		rv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, t.Bits())
		if err != nil {
			return v, bad
		}
		rv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw, t.Bits())
		if err != nil {
			return v, bad
		}
		rv.SetFloat(f)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return v, bad
		}
		rv.SetBool(b)
	case reflect.String:
		rv.SetString(raw)
	default:
		return v, fmt.Errorf("type %s cannot be parsed from text", t)
	}
	return v, nil
}
