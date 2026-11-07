// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"reflect"
)

// Scope is a named, reusable query transformation, for example a tenant or
// visibility filter shared by many call sites. A Scope receives and returns a
// SelectQuery value, so it composes with every other builder method.
type Scope[T any] func(SelectQuery[T]) SelectQuery[T]

// Scope applies each scope to the query in order and returns the result. A nil
// scope is skipped.
func (sq SelectQuery[T]) Scope(s ...Scope[T]) SelectQuery[T] {
	for _, f := range s {
		if f != nil {
			sq = f(sq)
		}
	}
	return sq
}

// maxFilterDepth bounds how deep a Filter tree may nest, so a tree built from
// external input cannot exhaust the stack.
const maxFilterDepth = 32

// Filter is a runtime-built predicate tree, for filters that arrive from
// outside the program (a query string, an API body). Exactly one of And, Or,
// Not or Field must be set on each node. Field is an external name resolved
// through an Allowlist; it never becomes SQL text itself. Op is one of eq, ne,
// lt, le, gt, ge, in, null, notnull and like; Value is bound as a parameter
// (null and notnull ignore it, in takes a slice, like takes a string pattern
// already escaped with EscapeLike).
type Filter struct {
	And   []Filter
	Or    []Filter
	Not   *Filter
	Field string
	Op    string
	Value any
}

// Predicate builds the predicate. A field outside the allowlist, an unknown
// Op, a malformed node or a Value of the wrong shape returns an error matching
// ErrValidation. The errors never echo the offending name or operator, which
// may be attacker controlled.
func (f Filter) Predicate(al Allowlist) (Predicate, error) {
	return f.build(al, 0)
}

func filterErr(msg string) error {
	return &Error{Kind: ErrValidation, Op: "filter", Err: errors.New(msg)}
}

func (f Filter) build(al Allowlist, depth int) (Predicate, error) {
	if depth > maxFilterDepth {
		return nil, filterErr("filter nests too deeply")
	}
	forms := 0
	if len(f.And) > 0 {
		forms++
	}
	if len(f.Or) > 0 {
		forms++
	}
	if f.Not != nil {
		forms++
	}
	if f.Field != "" {
		forms++
	}
	if forms != 1 {
		return nil, filterErr("a filter node must set exactly one of And, Or, Not or Field")
	}
	switch {
	case len(f.And) > 0:
		ps, err := buildFilters(f.And, al, depth)
		if err != nil {
			return nil, err
		}
		return And(ps...), nil
	case len(f.Or) > 0:
		ps, err := buildFilters(f.Or, al, depth)
		if err != nil {
			return nil, err
		}
		return Or(ps...), nil
	case f.Not != nil:
		p, err := f.Not.build(al, depth+1)
		if err != nil {
			return nil, err
		}
		return Not(p), nil
	}
	col, err := al.Column(f.Field)
	if err != nil {
		return nil, err
	}
	return fieldPredicate(col, f.Op, f.Value)
}

func buildFilters(fs []Filter, al Allowlist, depth int) ([]Predicate, error) {
	ps := make([]Predicate, len(fs))
	for i, c := range fs {
		p, err := c.build(al, depth+1)
		if err != nil {
			return nil, err
		}
		ps[i] = p
	}
	return ps, nil
}

func fieldPredicate(col ColumnRef, op string, v any) (Predicate, error) {
	cmp := map[string]string{"eq": " = ", "ne": " <> ", "lt": " < ", "le": " <= ", "gt": " > ", "ge": " >= "}
	if sym, ok := cmp[op]; ok {
		if v == nil {
			return nil, filterErr("a comparison needs a non-nil value; use null or notnull")
		}
		return binOp{col, sym, argExpr{v}}, nil
	}
	switch op {
	case "null":
		return postfix{col, " IS NULL"}, nil
	case "notnull":
		return postfix{col, " IS NOT NULL"}, nil
	case "like":
		s, ok := v.(string)
		if !ok {
			return nil, filterErr("like needs a string pattern")
		}
		return like{e: col, p: s}, nil
	case "in":
		vs, err := filterList(v)
		if err != nil {
			return nil, err
		}
		return inList[any]{col, vs, false}, nil
	}
	return nil, filterErr("unknown filter operator")
}

// filterList converts the Value of an in node into a list of bound values.
func filterList(v any) ([]any, error) {
	switch t := v.(type) {
	case []any:
		return t, nil
	case []string:
		return toAnyList(t), nil
	case []int:
		return toAnyList(t), nil
	case []int64:
		return toAnyList(t), nil
	case []int32:
		return toAnyList(t), nil
	case []float64:
		return toAnyList(t), nil
	case []bool:
		return toAnyList(t), nil
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || rv.Kind() != reflect.Slice {
		return nil, filterErr("in needs a slice value")
	}
	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out, nil
}

func toAnyList[E any](s []E) []any {
	out := make([]any, len(s))
	for i, e := range s {
		out[i] = e
	}
	return out
}
