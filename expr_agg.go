// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

// ------------------------------------------------- ordered aggregates

type aggKind uint8

const (
	aggGeneric aggKind = iota
	aggString
	aggArray
	aggJSON
)

// aggSpec is the dialect-independent description of an aggregate call. It is
// kept apart from the generic AggExpr so a further clause (FILTER) is added
// in one place.
type aggSpec struct {
	kind     aggKind
	name     string // aggGeneric: the function name
	args     []any
	sep      string // aggString: the separator
	orderBy  []Order
	within   []Order
	filter   Predicate // FILTER (WHERE ...)
	filtered bool      // Filter was called, possibly with a nil predicate
	ordered  bool      // an ordered-set aggregate: WithinGroup is required
	fraction float64   // rendered as the only argument when ordered
}

// AggExpr is an aggregate call whose SQL result type corresponds to T. It is
// an Expression[T], can be aliased with As, and takes an ORDER BY inside the
// call with OrderBy or a WITHIN GROUP clause with WithinGroup. It is
// immutable: every method returns a new value.
type AggExpr[T any] struct {
	exprOps[T]
	spec aggSpec
}

func newAgg[T any](s aggSpec) AggExpr[T] {
	a := AggExpr[T]{spec: s}
	a.exprOps = selfOps[T](a)
	return a
}

// Aggregate is a typed call of the aggregate function name, rendered
// name(args). name is validated like Func, and plain (non-Expr) arguments
// are bound as parameters exactly as Func binds them, without masking. Chain OrderBy for
// name(args ORDER BY ...). Momo cannot verify that T is what the function
// returns.
func Aggregate[T any](name string, args ...any) AggExpr[T] {
	return newAgg[T](aggSpec{kind: aggGeneric, name: name, args: slices.Clone(args)})
}

// OrderBy adds the ORDER BY that goes inside the call: agg(args ORDER BY o).
// PostgreSQL supports it on every aggregate, SQLite (3.44 and newer) too;
// MySQL only on GROUP_CONCAT (StringAgg), elsewhere it fails the build with
// ErrUnsupported.
func (a AggExpr[T]) OrderBy(o ...Order) AggExpr[T] {
	a.spec.orderBy = append(slices.Clip(a.spec.orderBy), o...)
	return newAgg[T](a.spec)
}

// WithinGroup sets the WITHIN GROUP (ORDER BY ...) of an ordered-set
// aggregate. Only PostgreSQL has them; other dialects fail the build with
// ErrUnsupported. It is allowed only on PercentileCont and PercentileDisc;
// any other aggregate fails the build with ErrValidation (use OrderBy for the
// order inside the call).
func (a AggExpr[T]) WithinGroup(o ...Order) AggExpr[T] {
	a.spec.within = append(slices.Clip(a.spec.within), o...)
	return newAgg[T](a.spec)
}

// Filter restricts the rows the aggregate sees: agg(...) FILTER (WHERE p),
// rendered after any WITHIN GROUP. It can be combined with OrderBy in either
// order: StringAgg(...).OrderBy(...).Filter(p). PostgreSQL and SQLite support
// it; MySQL fails the build with ErrUnsupported.
func (a AggExpr[T]) Filter(p Predicate) AggExpr[T] {
	a.spec.filter, a.spec.filtered = p, true
	return newAgg[T](a.spec)
}

// As names the call in a select list.
func (a AggExpr[T]) As(alias string) Aliased[T] { return As[T](a, alias) }

func (AggExpr[T]) resultType() T { var z T; return z }

func (a AggExpr[T]) AppendSQL(b *Builder) { a.spec.appendSQL(b) }

func aggErr(b *Builder, err error) {
	b.SetErr(newError(ErrValidation, "build", "", err))
}

func appendOrders(b *Builder, orders []Order) {
	for i, o := range orders {
		if i > 0 {
			b.WriteString(", ")
		}
		o.appendSQL(b)
	}
}

func (s aggSpec) appendArgs(b *Builder) {
	for i, a := range s.args {
		if i > 0 {
			b.WriteString(", ")
		}
		if e, ok := a.(Expr); ok {
			e.AppendSQL(b)
		} else {
			b.Arg(a)
		}
	}
}

func (s aggSpec) appendSQL(b *Builder) {
	s.appendCall(b)
	if s.filtered {
		appendFilter(b, s.filter)
	}
}

// appendFilter renders " FILTER (WHERE p)". A nil predicate fails the build
// with ErrValidation instead of silently leaving the aggregate unfiltered;
// MySQL has no FILTER clause. AggExpr.Filter is its only caller.
func appendFilter(b *Builder, p Predicate) {
	if p == nil {
		aggErr(b, errors.New("Filter needs a predicate"))
		return
	}
	if b.d.Name == dialect.MySQL {
		b.unsupported("aggregate FILTER")
		return
	}
	b.WriteString(" FILTER (WHERE ")
	p.AppendSQL(b)
	b.writeByte(')')
}

func (s aggSpec) appendCall(b *Builder) {
	name := s.name
	switch {
	case len(s.orderBy) > 0 && len(s.within) > 0:
		aggErr(b, errors.New("an aggregate takes OrderBy or WithinGroup, not both"))
		return
	case s.ordered && len(s.within) == 0:
		aggErr(b, fmt.Errorf("%s is an ordered-set aggregate: add WithinGroup(order)", s.name))
		return
	case s.ordered && (math.IsNaN(s.fraction) || s.fraction < 0 || s.fraction > 1):
		aggErr(b, fmt.Errorf("%s fraction %v is outside 0 to 1", s.name, s.fraction))
		return
	case len(s.within) > 0 && !s.ordered:
		aggErr(b, errors.New("WithinGroup applies only to ordered-set aggregates (PercentileCont, PercentileDisc); use OrderBy for the order inside the call"))
		return
	case len(s.within) > 0 && b.d.Name != dialect.Postgres:
		b.unsupported("WITHIN GROUP")
		return
	}
	switch s.kind {
	case aggString:
		s.appendStringAgg(b)
		return
	case aggArray:
		if b.d.Name != dialect.Postgres {
			b.unsupported("array_agg")
			return
		}
		name = "array_agg"
	case aggJSON:
		name = map[dialect.Name]string{dialect.Postgres: "jsonb_agg", dialect.SQLite: "json_group_array", dialect.MySQL: "JSON_ARRAYAGG"}[b.d.Name]
	default:
		for _, part := range strings.Split(name, ".") {
			if err := validName(part); err != nil {
				aggErr(b, fmt.Errorf("aggregate name %q: %w", name, err))
				return
			}
		}
	}
	if len(s.orderBy) > 0 && b.d.Name == dialect.MySQL {
		b.unsupported("aggregate ORDER BY")
		return
	}
	b.WriteString(name)
	b.writeByte('(')
	if s.ordered {
		b.WriteString(strconv.FormatFloat(s.fraction, 'g', -1, 64))
	} else {
		s.appendArgs(b)
	}
	if len(s.orderBy) > 0 {
		b.WriteString(" ORDER BY ")
		appendOrders(b, s.orderBy)
	}
	b.writeByte(')')
	if len(s.within) > 0 {
		b.WriteString(" WITHIN GROUP (ORDER BY ")
		appendOrders(b, s.within)
		b.writeByte(')')
	}
}

func (s aggSpec) appendStringAgg(b *Builder) {
	sep, ok := sqlStringLiteral(b, "separator", s.sep)
	if !ok {
		return
	}
	if b.d.Name == dialect.MySQL {
		b.WriteString("GROUP_CONCAT(")
		s.appendArgs(b)
		if len(s.orderBy) > 0 {
			b.WriteString(" ORDER BY ")
			appendOrders(b, s.orderBy)
		}
		b.WriteString(" SEPARATOR " + sep + ")")
		return
	}
	if b.d.Name == dialect.SQLite {
		b.WriteString("group_concat(")
	} else {
		b.WriteString("string_agg(")
	}
	s.appendArgs(b)
	b.WriteString(", " + sep)
	if len(s.orderBy) > 0 {
		b.WriteString(" ORDER BY ")
		appendOrders(b, s.orderBy)
	}
	b.writeByte(')')
}

// sqlStringLiteral quotes s as a SQL string literal. It is a literal and not
// a parameter because MySQL's SEPARATOR and PostgreSQL's jsonb_build_object
// keys cannot take an untyped parameter. A backslash or NUL byte fails the
// build with ErrValidation: their meaning differs between dialects.
func sqlStringLiteral(b *Builder, what, s string) (string, bool) {
	if strings.ContainsAny(s, "\\\x00") {
		aggErr(b, fmt.Errorf("%s %q must not contain a backslash or NUL", what, s))
		return "", false
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'", true
}

// StringAgg concatenates the non-NULL values of e, separated by sep, in the
// order given by OrderBy: string_agg on PostgreSQL, group_concat on SQLite
// and GROUP_CONCAT ... SEPARATOR on MySQL (which truncates at
// group_concat_max_len). The separator is rendered as a literal and must not
// contain a backslash. Without a matching row, and for a group whose values
// are all NULL, the result is NULL: wrap it in CoalesceExpr with a literal
// (for example an empty string) when the group may be empty.
func StringAgg(e Expression[string], sep string) AggExpr[string] {
	return newAgg[string](aggSpec{kind: aggString, args: []any{e}, sep: sep})
}

// ArrayAgg collects the values of e into an array, in the order given by
// OrderBy: array_agg(e ORDER BY ...). PostgreSQL only; other dialects fail
// the build with ErrUnsupported. Without a matching row the result is NULL.
// array_agg keeps NULL elements, so for a nullable column use a pointer
// element type (ArrayAgg[*string]); a non-pointer T fails to scan a NULL.
func ArrayAgg[T any](e Expression[T]) AggExpr[[]T] {
	return newAgg[[]T](aggSpec{kind: aggArray, args: []any{e}})
}

// JSONBAgg collects the values of e into a JSON array: jsonb_agg on
// PostgreSQL, json_group_array on SQLite, JSON_ARRAYAGG on MySQL (which has
// no OrderBy). T is the element type the array decodes into, for example
// JSONBAgg[Item](JSONBBuildObject[Item](...)). Without a matching row the
// result is NULL on PostgreSQL and MySQL and '[]' on SQLite; wrap it in
// CoalesceExpr with EmptyJSONArray to get an empty array everywhere.
func JSONBAgg[T any](e Expr) AggExpr[types.JSON[[]T]] {
	return newAgg[types.JSON[[]T]](aggSpec{kind: aggJSON, args: []any{e}})
}

// PercentileCont is percentile_cont(fraction) WITHIN GROUP (ORDER BY ...):
// the interpolated value at fraction (0 to 1) of the ordered input. The
// ordering comes from WithinGroup, which is required. PostgreSQL only. The
// result is NULL for an empty group: wrap it in CoalesceExpr when the group
// may be empty.
func PercentileCont(fraction float64) AggExpr[float64] {
	return percentile[float64]("percentile_cont", fraction)
}

// PercentileDisc is percentile_disc(fraction) WITHIN GROUP (ORDER BY ...):
// the first input value whose position reaches fraction (0 to 1). T is the
// type of the ordered expression. The ordering comes from WithinGroup, which
// is required. PostgreSQL only.
func PercentileDisc[T any](fraction float64) AggExpr[T] {
	return percentile[T]("percentile_disc", fraction)
}

func percentile[T any](name string, fraction float64) AggExpr[T] {
	return newAgg[T](aggSpec{kind: aggGeneric, name: name, ordered: true, fraction: fraction})
}

// ------------------------------------------------------------- JSON values

// JSONPair is one key and value of JSONBBuildObject.
type JSONPair struct {
	Key   string
	Value Expr
}

// JSONField pairs a JSON object key with the expression that supplies its
// value.
func JSONField(key string, v Expr) JSONPair { return JSONPair{key, v} }

// JSONObjectExpr is a JSON object built from key and value pairs.
type JSONObjectExpr[T any] struct {
	exprOps[types.JSON[T]]
	pairs []JSONPair
}

// JSONBBuildObject builds a JSON object from key and value pairs:
// jsonb_build_object on PostgreSQL, json_object on SQLite, JSON_OBJECT on
// MySQL. T is the type the object decodes into, usually a struct. Keys are
// rendered as literals and must not contain a backslash.
func JSONBBuildObject[T any](pairs ...JSONPair) JSONObjectExpr[T] {
	o := JSONObjectExpr[T]{pairs: slices.Clone(pairs)}
	o.exprOps = selfOps[types.JSON[T]](o)
	return o
}

// As names the expression in a select list.
func (o JSONObjectExpr[T]) As(alias string) Aliased[types.JSON[T]] {
	return As[types.JSON[T]](o, alias)
}

func (JSONObjectExpr[T]) resultType() types.JSON[T] { var z types.JSON[T]; return z }

func (o JSONObjectExpr[T]) AppendSQL(b *Builder) {
	b.WriteString(map[dialect.Name]string{dialect.Postgres: "jsonb_build_object(", dialect.SQLite: "json_object(", dialect.MySQL: "JSON_OBJECT("}[b.d.Name])
	for i, p := range o.pairs {
		if i > 0 {
			b.WriteString(", ")
		}
		if p.Value == nil {
			aggErr(b, fmt.Errorf("JSON field %q has no value expression", p.Key))
			return
		}
		lit, ok := sqlStringLiteral(b, "JSON key", p.Key)
		if !ok {
			return
		}
		b.WriteString(lit + ", ")
		p.Value.AppendSQL(b)
	}
	b.writeByte(')')
}

// ToJSONExpr is a value converted to JSON.
type ToJSONExpr[T any] struct {
	exprOps[types.JSON[T]]
	e Expr
}

// ToJSONB converts a value to JSON: to_jsonb(e) on PostgreSQL (a table
// alias converts the whole row) and json_quote(e) on SQLite. MySQL fails the
// build with ErrUnsupported.
func ToJSONB[T any](e Expr) ToJSONExpr[T] {
	t := ToJSONExpr[T]{e: e}
	t.exprOps = selfOps[types.JSON[T]](t)
	return t
}

// As names the expression in a select list.
func (t ToJSONExpr[T]) As(alias string) Aliased[types.JSON[T]] {
	return As[types.JSON[T]](t, alias)
}

func (ToJSONExpr[T]) resultType() types.JSON[T] { var z types.JSON[T]; return z }

func (t ToJSONExpr[T]) AppendSQL(b *Builder) {
	switch b.d.Name {
	case dialect.Postgres:
		b.WriteString("to_jsonb(")
	case dialect.SQLite:
		b.WriteString("json_quote(")
	default:
		b.unsupported("to_jsonb")
		return
	}
	t.e.AppendSQL(b)
	b.writeByte(')')
}

// EmptyJSONArray is the empty JSON array literal, the fallback of
// CoalesceExpr over an aggregate that saw no rows:
// CoalesceExpr(JSONBAgg[T](x), EmptyJSONArray[T]()).
func EmptyJSONArray[T any]() Expression[types.JSON[[]T]] { return emptyJSONArray[T]{} }

type emptyJSONArray[T any] struct{}

func (emptyJSONArray[T]) resultType() types.JSON[[]T] { return types.JSON[[]T]{} }

func (emptyJSONArray[T]) AppendSQL(b *Builder) {
	switch b.d.Name {
	case dialect.Postgres:
		b.WriteString("'[]'::jsonb")
	case dialect.MySQL:
		b.WriteString("JSON_ARRAY()")
	default:
		b.WriteString("'[]'")
	}
}
