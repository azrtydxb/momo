// SPDX-License-Identifier: Apache-2.0

// Package agg holds the aggregates that the basic Agg of the root package
// does not cover: ORDER BY inside the call, WITHIN GROUP ordered-set
// aggregates, string, array and JSON array aggregation, and the JSON object
// builders that feed them. A Call is an Expression, so it composes with the
// core: it is aliased with As, compared in HAVING through the embedded Ops,
// and turned into a window function with Over.
//
// The package builds on the exported extension API of the root package (Ops,
// Order.AppendOrder, Builder.AppendFilter, WindowOf, ValidName and the Builder
// methods) and no unexported name.
package agg

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

type kind uint8

const (
	kindGeneric kind = iota
	kindString
	kindArray
	kindJSON
)

// spec is the dialect-independent description of an aggregate call. It is
// kept apart from the generic Call so a further clause is added in one place.
type spec struct {
	kind     kind
	name     string // kindGeneric: the function name
	args     []any
	sep      string // kindString: the separator
	orderBy  []momo.Order
	within   []momo.Order
	distinct bool           // DISTINCT inside the call
	filter   momo.Predicate // FILTER (WHERE ...)
	filtered bool           // Filter was called, possibly with a nil predicate
	ordered  bool           // an ordered-set aggregate: WithinGroup is required
	fraction float64        // rendered as the only argument when ordered
}

// Call is an aggregate call whose SQL result type corresponds to T. It is an
// Expression[T] with the comparison methods of momo.Ops, can be aliased with
// As, and takes an ORDER BY inside the call with OrderBy or a WITHIN GROUP
// clause with WithinGroup. It is immutable: every method returns a new value.
type Call[T any] struct {
	momo.Ops[T]
	spec spec
}

func newCall[T any](s spec) Call[T] {
	a := Call[T]{spec: s}
	a.Ops = momo.NewOps[T](a)
	return a
}

// Func is a typed call of the aggregate function name, rendered name(args).
// name is validated like momo.Func, and plain (non-Expr) arguments are bound
// as parameters exactly as momo.Func binds them, without masking. Chain
// OrderBy for name(args ORDER BY ...). Momo cannot verify that T is what the
// function returns.
func Func[T any](name string, args ...any) Call[T] {
	return newCall[T](spec{kind: kindGeneric, name: name, args: slices.Clone(args)})
}

// OrderBy adds the ORDER BY that goes inside the call: agg(args ORDER BY o).
// PostgreSQL supports it on every aggregate, SQLite (3.44 and newer) too;
// MySQL only on GROUP_CONCAT (String), elsewhere it fails the build with
// ErrUnsupported.
func (a Call[T]) OrderBy(o ...momo.Order) Call[T] {
	a.spec.orderBy = append(slices.Clip(a.spec.orderBy), o...)
	return newCall[T](a.spec)
}

// Distinct makes the aggregate see each distinct value of its argument once:
// array_agg(DISTINCT e), count(DISTINCT e), string_agg(DISTINCT e, sep),
// jsonb_agg(DISTINCT e). It takes exactly one argument (none or several fail
// the build with ErrValidation) and cannot be used with Over (ErrValidation).
// It combines with OrderBy and Filter; on PostgreSQL every OrderBy expression
// must be the argument itself, or the build fails with ErrValidation, as
// PostgreSQL requires. Support differs: PostgreSQL
// takes it on every aggregate except the ordered-set ones (PercentileCont,
// PercentileDisc), where it fails the build with ErrValidation. SQLite takes
// it on single-argument aggregates (not String, whose separator is a second
// argument) and MySQL on every aggregate except JSONB; the combinations that
// a dialect lacks fail the build with ErrUnsupported.
func (a Call[T]) Distinct() Call[T] {
	a.spec.distinct = true
	return newCall[T](a.spec)
}

// WithinGroup sets the WITHIN GROUP (ORDER BY ...) of an ordered-set
// aggregate. Only PostgreSQL has them; other dialects fail the build with
// ErrUnsupported. It is allowed only on PercentileCont and PercentileDisc;
// any other aggregate fails the build with ErrValidation (use OrderBy for the
// order inside the call).
func (a Call[T]) WithinGroup(o ...momo.Order) Call[T] {
	a.spec.within = append(slices.Clip(a.spec.within), o...)
	return newCall[T](a.spec)
}

// Filter restricts the rows the aggregate sees: agg(...) FILTER (WHERE p),
// rendered after any WITHIN GROUP. It can be combined with OrderBy in either
// order: String(...).OrderBy(...).Filter(p). PostgreSQL and SQLite render the
// FILTER clause. MySQL and MariaDB have none, so the predicate moves into the
// arguments, agg(CASE WHEN p THEN x END), which is exact for the aggregates
// that skip NULLs: String, and Func over COUNT, SUM, AVG, MIN, MAX and the
// STD and VAR family. p is rendered once per argument, so a bound value in p
// appears that many times, in order. The aggregates that keep NULLs (JSONB,
// and any other Func) fail the build with ErrUnsupported there, because the
// rewrite would add a NULL element per filtered-out row.
func (a Call[T]) Filter(p momo.Predicate) Call[T] {
	a.spec.filter, a.spec.filtered = p, true
	return newCall[T](a.spec)
}

// As names the call in a select list.
func (a Call[T]) As(alias string) momo.Aliased[T] { return momo.As[T](a, alias) }

// AppendSQL renders the call.
func (a Call[T]) AppendSQL(b *momo.Builder) { a.spec.appendSQL(b) }

func fail(b *momo.Builder, err error) {
	b.SetErr(&momo.Error{Kind: momo.ErrValidation, Op: "build", Err: err})
}

func appendOrders(b *momo.Builder, orders []momo.Order) {
	for i, o := range orders {
		if i > 0 {
			b.WriteString(", ")
		}
		o.AppendOrder(b)
	}
}

func (s spec) appendArgs(b *momo.Builder) {
	emulate := s.emulateFilter(b)
	for i, a := range s.args {
		if i > 0 {
			b.WriteString(", ")
		}
		if emulate {
			b.WriteString("CASE WHEN ")
			s.filter.AppendSQL(b)
			b.WriteString(" THEN ")
		}
		if e, ok := a.(momo.Expr); ok {
			e.AppendSQL(b)
		} else {
			b.Arg(a)
		}
		if emulate {
			b.WriteString(" END")
		}
	}
}

// emulateFilter reports whether the FILTER predicate is rewritten into the
// arguments of the call, which is how MySQL and MariaDB, having no FILTER
// clause, get the same result: agg(CASE WHEN p THEN x END).
func (s spec) emulateFilter(b *momo.Builder) bool {
	return s.filtered && s.filter != nil && b.Dialect().Name == dialect.MySQL
}

// filterSkipsNull names the generic aggregates that skip NULL inputs, for
// which agg(CASE WHEN p THEN x END) equals agg(x) FILTER (WHERE p).
var filterSkipsNull = map[string]bool{
	"count": true, "sum": true, "avg": true, "min": true, "max": true,
	"std": true, "stddev": true, "stddev_pop": true, "stddev_samp": true,
	"variance": true, "var_pop": true, "var_samp": true,
}

// checkEmulatedFilter refuses the aggregates for which the CASE rewrite would
// change the result, because they keep NULL inputs: JSON_ARRAYAGG would gain a
// null element for every filtered-out row. A generic aggregate is refused
// unless it is known to skip NULLs. It records the build error and reports
// false when the filter cannot be emulated.
func (s spec) checkEmulatedFilter(b *momo.Builder) bool {
	switch {
	case s.ordered || len(s.within) > 0 || s.kind == kindArray:
		return true // appendCall reports these as unsupported on MySQL
	case s.kind == kindJSON:
		b.Unsupported("aggregate FILTER on a JSON array aggregate (JSON_ARRAYAGG keeps NULLs, so the CASE WHEN rewrite would add a null element per filtered-out row; filter in a subquery or WHERE instead)")
		return false
	case s.kind == kindGeneric && !filterSkipsNull[strings.ToLower(s.name)]:
		b.Unsupported("aggregate FILTER on " + s.name + " (only COUNT, SUM, AVG, MIN, MAX and the STD and VAR family are known to skip NULLs, which the CASE WHEN rewrite needs; filter in a subquery or WHERE instead)")
		return false
	case len(s.args) == 0:
		b.Unsupported("aggregate FILTER on a call with no argument")
		return false
	}
	return true
}

func (s spec) appendSQL(b *momo.Builder) {
	if s.emulateFilter(b) && !s.checkEmulatedFilter(b) {
		return
	}
	s.appendCall(b)
	if s.filtered && !s.emulateFilter(b) {
		b.AppendFilter(s.filter)
	}
}

func (s spec) appendCall(b *momo.Builder) {
	name := s.name
	dn := b.Dialect().Name
	switch {
	case len(s.orderBy) > 0 && len(s.within) > 0:
		fail(b, errors.New("an aggregate takes OrderBy or WithinGroup, not both"))
		return
	case s.ordered && len(s.within) == 0:
		fail(b, fmt.Errorf("%s is an ordered-set aggregate: add WithinGroup(order)", s.name))
		return
	case s.ordered && (math.IsNaN(s.fraction) || s.fraction < 0 || s.fraction > 1):
		fail(b, fmt.Errorf("%s fraction %v is outside 0 to 1", s.name, s.fraction))
		return
	case len(s.within) > 0 && !s.ordered:
		fail(b, errors.New("WithinGroup applies only to ordered-set aggregates (PercentileCont, PercentileDisc); use OrderBy for the order inside the call"))
		return
	case len(s.within) > 0 && dn != dialect.Postgres:
		b.Unsupported("WITHIN GROUP")
		return
	}
	if s.distinct && !s.checkDistinct(b) {
		return
	}
	switch s.kind {
	case kindString:
		s.appendString(b)
		return
	case kindArray:
		if dn != dialect.Postgres {
			b.Unsupported("array_agg")
			return
		}
		name = "array_agg"
	case kindJSON:
		if s.distinct && dn == dialect.MySQL {
			b.Unsupported("JSON_ARRAYAGG(DISTINCT ...)")
			return
		}
		name = map[dialect.Name]string{dialect.Postgres: "jsonb_agg", dialect.SQLite: "json_group_array", dialect.MySQL: "JSON_ARRAYAGG"}[dn]
	default:
		for _, part := range strings.Split(name, ".") {
			if err := momo.ValidName(part); err != nil {
				fail(b, fmt.Errorf("aggregate name %q: %w", name, err))
				return
			}
		}
	}
	if len(s.orderBy) > 0 && dn == dialect.MySQL {
		b.Unsupported("aggregate ORDER BY")
		return
	}
	b.WriteString(name)
	_ = b.WriteByte('(')
	if s.distinct {
		b.WriteString("DISTINCT ")
	}
	if s.ordered {
		b.WriteString(strconv.FormatFloat(s.fraction, 'g', -1, 64))
	} else {
		s.appendArgs(b)
	}
	if len(s.orderBy) > 0 {
		b.WriteString(" ORDER BY ")
		appendOrders(b, s.orderBy)
	}
	_ = b.WriteByte(')')
	if len(s.within) > 0 {
		b.WriteString(" WITHIN GROUP (ORDER BY ")
		appendOrders(b, s.within)
		_ = b.WriteByte(')')
	}
}

// checkDistinct validates a DISTINCT call: not on an ordered-set aggregate,
// exactly one argument, and on PostgreSQL every ORDER BY expression among the
// arguments. It records the build error and reports false when invalid.
func (s spec) checkDistinct(b *momo.Builder) bool {
	switch {
	case s.ordered:
		fail(b, errors.New("Distinct does not apply to ordered-set aggregates (PercentileCont, PercentileDisc)"))
		return false
	case len(s.args) != 1:
		fail(b, fmt.Errorf("Distinct needs exactly one argument, got %d", len(s.args)))
		return false
	case b.Dialect().Name != dialect.Postgres || len(s.orderBy) == 0:
		return true
	}
	arg := renderOne(b, s.args[0])
	for _, o := range s.orderBy {
		if renderOne(b, o.Expr) != arg {
			fail(b, errors.New("with Distinct, PostgreSQL requires every OrderBy expression to appear in the argument list of the aggregate (for an aggregate over a column, order by that same column)"))
			return false
		}
	}
	return true
}

// renderOne renders v (an expression, or a value bound as a parameter) on its
// own, to compare two expressions by their SQL.
func renderOne(b *momo.Builder, v any) string {
	t := momo.NewBuilder(b.Dialect())
	if e, ok := v.(momo.Expr); ok {
		e.AppendSQL(t)
	} else {
		t.Arg(v)
	}
	return t.String()
}

func (s spec) appendString(b *momo.Builder) {
	sep, ok := stringLiteral(b, "separator", s.sep)
	if !ok {
		return
	}
	switch b.Dialect().Name {
	case dialect.MySQL:
		b.WriteString("GROUP_CONCAT(")
		if s.distinct {
			b.WriteString("DISTINCT ")
		}
		s.appendArgs(b)
		if len(s.orderBy) > 0 {
			b.WriteString(" ORDER BY ")
			appendOrders(b, s.orderBy)
		}
		b.WriteString(" SEPARATOR " + sep + ")")
		return
	case dialect.SQLite:
		if s.distinct {
			b.Unsupported("group_concat(DISTINCT ..., separator)")
			return
		}
		b.WriteString("group_concat(")
	default:
		b.WriteString("string_agg(")
		if s.distinct {
			b.WriteString("DISTINCT ")
		}
	}
	s.appendArgs(b)
	b.WriteString(", " + sep)
	if len(s.orderBy) > 0 {
		b.WriteString(" ORDER BY ")
		appendOrders(b, s.orderBy)
	}
	_ = b.WriteByte(')')
}

// stringLiteral quotes s as a SQL string literal. It is a literal and not a
// parameter because MySQL's SEPARATOR and PostgreSQL's jsonb_build_object keys
// cannot take an untyped parameter. A backslash or NUL byte fails the build
// with ErrValidation: their meaning differs between dialects.
func stringLiteral(b *momo.Builder, what, s string) (string, bool) {
	if strings.ContainsAny(s, "\\\x00") {
		fail(b, fmt.Errorf("%s %q must not contain a backslash or NUL", what, s))
		return "", false
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'", true
}

// String concatenates the non-NULL values of e, separated by sep, in the
// order given by OrderBy: string_agg on PostgreSQL, group_concat on SQLite
// and GROUP_CONCAT ... SEPARATOR on MySQL (which truncates at
// group_concat_max_len). The separator is rendered as a literal and must not
// contain a backslash. Without a matching row, and for a group whose values
// are all NULL, the result is NULL: wrap it in sqlfn.CoalesceExpr with a
// literal (for example an empty string) when the group may be empty.
func String(e momo.Expression[string], sep string) Call[string] {
	return newCall[string](spec{kind: kindString, args: []any{e}, sep: sep})
}

// Array collects the values of e into an array, in the order given by
// OrderBy: array_agg(e ORDER BY ...). PostgreSQL only; other dialects fail
// the build with ErrUnsupported. Without a matching row the result is NULL.
// array_agg keeps NULL elements, so for a nullable column use a pointer
// element type (Array[*string]); a non-pointer T fails to scan a NULL.
func Array[T any](e momo.Expression[T]) Call[[]T] {
	return newCall[[]T](spec{kind: kindArray, args: []any{e}})
}

// JSONB collects the values of e into a JSON array: jsonb_agg on PostgreSQL,
// json_group_array on SQLite, JSON_ARRAYAGG on MySQL (which has no OrderBy).
// T is the element type the array decodes into, for example
// JSONB[Item](BuildObject[Item](...)). Without a matching row the result is
// NULL on PostgreSQL and MySQL and '[]' on SQLite; wrap it in
// sqlfn.CoalesceExpr with EmptyJSONArray to get an empty array everywhere.
func JSONB[T any](e momo.Expr) Call[types.JSON[[]T]] {
	return newCall[types.JSON[[]T]](spec{kind: kindJSON, args: []any{e}})
}

// PercentileCont is percentile_cont(fraction) WITHIN GROUP (ORDER BY ...):
// the interpolated value at fraction (0 to 1) of the ordered input. The
// ordering comes from WithinGroup, which is required. PostgreSQL only. The
// result is NULL for an empty group: wrap it in sqlfn.CoalesceExpr when the
// group may be empty.
func PercentileCont(fraction float64) Call[float64] {
	return percentile[float64]("percentile_cont", fraction)
}

// PercentileDisc is percentile_disc(fraction) WITHIN GROUP (ORDER BY ...):
// the first input value whose position reaches fraction (0 to 1). T is the
// type of the ordered expression. The ordering comes from WithinGroup, which
// is required. PostgreSQL only.
func PercentileDisc[T any](fraction float64) Call[T] {
	return percentile[T]("percentile_disc", fraction)
}

func percentile[T any](name string, fraction float64) Call[T] {
	return newCall[T](spec{kind: kindGeneric, name: name, ordered: true, fraction: fraction})
}
