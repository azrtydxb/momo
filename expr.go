// SPDX-License-Identifier: Apache-2.0

package momo

// Expr is any SQL expression fragment. Implementations must write
// identifiers through Builder.Ident and values through Builder.Arg.
type Expr interface {
	AppendSQL(b *Builder)
}

// Expression is an Expr whose SQL result type corresponds to the Go type
// T. The type parameter lets the compiler reject comparisons between
// incompatible operands, e.g. an int64 column against a string.
type Expression[T any] interface {
	Expr
	resultType() T
}

// Predicate is a boolean expression usable in WHERE, ON and HAVING.
type Predicate = Expression[bool]

// ColumnRef is an untyped reference to a column of a table source. It is
// used where only the identity of the column matters (ON CONFLICT targets,
// Omit lists, GROUP BY).
type ColumnRef interface {
	Expr
	ColumnName() string
}

// ---------------------------------------------------------------- columns

// Column is a typed column descriptor generated for each model field. T is
// the column's Go value type with nullability removed (a *string field has
// a Column[string]-compatible NullColumn[string]).
//
// Column values are small, immutable and safe to share between goroutines.
type Column[T any] struct {
	table string // table name or alias used as qualifier
	name  string
	flags uint16
}

// NewColumn is used by generated code.
func NewColumn[T any](table, name string, flags uint16) Column[T] {
	return Column[T]{table: table, name: name, flags: flags}
}

// As returns the same column qualified by a table alias.
func (c Column[T]) As(alias string) Column[T] { c.table = alias; return c }

func (c Column[T]) AppendSQL(b *Builder) { b.QualifiedIdent(c.table, c.name) }
func (c Column[T]) resultType() T        { var z T; return z }

// ColumnName returns the unquoted column name.
func (c Column[T]) ColumnName() string { return c.name }

// Table returns the qualifier (table name or alias).
func (c Column[T]) Table() string { return c.table }

// Flags returns the generated column flags.
func (c Column[T]) Flags() uint16 { return c.flags }

func (c Column[T]) Eq(v T) Predicate  { return binOp{c, " = ", argExpr{v}} }
func (c Column[T]) Ne(v T) Predicate  { return binOp{c, " <> ", argExpr{v}} }
func (c Column[T]) Lt(v T) Predicate  { return binOp{c, " < ", argExpr{v}} }
func (c Column[T]) Le(v T) Predicate  { return binOp{c, " <= ", argExpr{v}} }
func (c Column[T]) Gt(v T) Predicate  { return binOp{c, " > ", argExpr{v}} }
func (c Column[T]) Ge(v T) Predicate  { return binOp{c, " >= ", argExpr{v}} }
func (c Column[T]) IsNull() Predicate { return postfix{c, " IS NULL"} }

// IsNotNull is meaningful on nullable columns and on columns from the
// outer side of a join.
func (c Column[T]) IsNotNull() Predicate { return postfix{c, " IS NOT NULL"} }

func (c Column[T]) EqExpr(e Expression[T]) Predicate { return binOp{c, " = ", e} }
func (c Column[T]) NeExpr(e Expression[T]) Predicate { return binOp{c, " <> ", e} }
func (c Column[T]) LtExpr(e Expression[T]) Predicate { return binOp{c, " < ", e} }
func (c Column[T]) LeExpr(e Expression[T]) Predicate { return binOp{c, " <= ", e} }
func (c Column[T]) GtExpr(e Expression[T]) Predicate { return binOp{c, " > ", e} }
func (c Column[T]) GeExpr(e Expression[T]) Predicate { return binOp{c, " >= ", e} }

// IsDistinctFrom is a NULL-safe inequality.
func (c Column[T]) IsDistinctFrom(v T) Predicate { return distinctFrom{c, argExpr{v}, false} }

// IsNotDistinctFrom is a NULL-safe equality.
func (c Column[T]) IsNotDistinctFrom(v T) Predicate { return distinctFrom{c, argExpr{v}, true} }

// Between is inclusive on both ends.
func (c Column[T]) Between(lo, hi T) Predicate { return between{c, lo, hi} }

// In matches any of vs. An empty list renders a predicate that is always
// false (1 = 0) rather than invalid SQL.
func (c Column[T]) In(vs ...T) Predicate { return inList[T]{c, vs, false} }

// NotIn is the negation of In; an empty list is always true.
func (c Column[T]) NotIn(vs ...T) Predicate { return inList[T]{c, vs, true} }

// InQuery matches the single-column result of a subquery.
func (c Column[T]) InQuery(q Subquery) Predicate { return inQuery{c, q, false} }

// NotInQuery is the negation of InQuery. Beware SQL's NOT IN semantics when
// the subquery can yield NULL.
func (c Column[T]) NotInQuery(q Subquery) Predicate { return inQuery{c, q, true} }

func (c Column[T]) Asc() Order  { return Order{Expr: c} }
func (c Column[T]) Desc() Order { return Order{Expr: c, Desc: true} }

// Set assigns a value (including the zero value) in INSERT or UPDATE.
func (c Column[T]) Set(v T) Assignment { return Assignment{col: c.name, kind: setValue, val: v} }

// SetExpr assigns an expression.
func (c Column[T]) SetExpr(e Expression[T]) Assignment {
	return Assignment{col: c.name, kind: setExpr, expr: e}
}

// SetDefault assigns the column's database default. PostgreSQL supports it
// in INSERT and UPDATE; SQLite only by omitting the column from an INSERT,
// so an UPDATE with SetDefault fails with ErrUnsupported there.
func (c Column[T]) SetDefault() Assignment { return Assignment{col: c.name, kind: setDefault} }

// SetExcluded assigns the value proposed for insertion in an upsert
// (ON CONFLICT ... DO UPDATE SET col = excluded.col).
func (c Column[T]) SetExcluded() Assignment { return Assignment{col: c.name, kind: setExcluded} }

// NullColumn is the descriptor for a nullable column. It adds SetNull.
type NullColumn[T any] struct{ Column[T] }

// NewNullColumn is used by generated code.
func NewNullColumn[T any](table, name string, flags uint16) NullColumn[T] {
	return NullColumn[T]{Column[T]{table: table, name: name, flags: flags | FlagNullable}}
}

// As returns the same column qualified by a table alias.
func (c NullColumn[T]) As(alias string) NullColumn[T] { c.table = alias; return c }

// SetNull assigns SQL NULL.
func (c NullColumn[T]) SetNull() Assignment { return Assignment{col: c.name, kind: setNull} }

// ---------------------------------------------------------- assignments

type setKind uint8

const (
	setValue setKind = iota
	setExpr
	setNull
	setDefault
	setExcluded
)

// Assignment is one explicit column write. Constructing one is the only way
// to include a column in a typed INSERT or UPDATE, which is how Momo keeps
// omitted, zero, NULL and DEFAULT distinct.
type Assignment struct {
	col  string
	kind setKind
	val  any
	expr Expr
}

// ColumnName returns the assigned column.
func (a Assignment) ColumnName() string { return a.col }

func (a Assignment) appendValue(b *Builder, insert bool) {
	switch a.kind {
	case setValue:
		b.Arg(a.val)
	case setExpr:
		a.expr.AppendSQL(b)
	case setNull:
		b.WriteString("NULL")
	case setDefault:
		if (insert && !b.d.DefaultInValues) || (!insert && !b.d.DefaultInUpdate) {
			b.unsupported("SET column = DEFAULT")
			return
		}
		b.WriteString("DEFAULT")
	case setExcluded:
		b.WriteString("excluded.")
		b.Ident(a.col)
	}
}

// ------------------------------------------------------------- ordering

// Nulls placement for ORDER BY.
type Nulls uint8

const (
	NullsDefault Nulls = iota
	NullsFirst
	NullsLast
)

// Order is one ORDER BY item.
type Order struct {
	Expr  Expr
	Desc  bool
	Nulls Nulls
}

// NullsFirst returns o with explicit NULLS FIRST.
func (o Order) NullsFirst() Order { o.Nulls = NullsFirst; return o }

// NullsLast returns o with explicit NULLS LAST.
func (o Order) NullsLast() Order { o.Nulls = NullsLast; return o }

func (o Order) appendSQL(b *Builder) {
	o.Expr.AppendSQL(b)
	if o.Desc {
		b.WriteString(" DESC")
	} else {
		b.WriteString(" ASC")
	}
	switch o.Nulls {
	case NullsFirst, NullsLast:
		if !b.d.NullsOrdering {
			b.unsupported("NULLS FIRST/LAST")
			return
		}
		if o.Nulls == NullsFirst {
			b.WriteString(" NULLS FIRST")
		} else {
			b.WriteString(" NULLS LAST")
		}
	}
}

// Asc orders by an arbitrary expression ascending.
func Asc(e Expr) Order { return Order{Expr: e} }

// Desc orders by an arbitrary expression descending.
func Desc(e Expr) Order { return Order{Expr: e, Desc: true} }

// ---------------------------------------------------------- expressions

type argExpr struct{ v any }

func (a argExpr) AppendSQL(b *Builder) { b.Arg(a.v) }

// Value wraps a Go value as a typed bound parameter.
func Value[T any](v T) Expression[T] { return typed[T]{argExpr{v}} }

type typed[T any] struct{ Expr }

func (typed[T]) resultType() T { var z T; return z }

type binOp struct {
	l  Expr
	op string
	r  Expr
}

func (o binOp) AppendSQL(b *Builder) {
	o.l.AppendSQL(b)
	b.WriteString(o.op)
	o.r.AppendSQL(b)
}
func (binOp) resultType() bool { return false }

type postfix struct {
	e  Expr
	op string
}

func (p postfix) AppendSQL(b *Builder) { p.e.AppendSQL(b); b.WriteString(p.op) }
func (postfix) resultType() bool       { return false }

type distinctFrom struct {
	l, r Expr
	not  bool
}

func (d distinctFrom) AppendSQL(b *Builder) {
	d.l.AppendSQL(b)
	if b.d.Name == "sqlite" { // SQLite spells it IS / IS NOT
		if d.not {
			b.WriteString(" IS ")
		} else {
			b.WriteString(" IS NOT ")
		}
	} else if d.not {
		b.WriteString(" IS NOT DISTINCT FROM ")
	} else {
		b.WriteString(" IS DISTINCT FROM ")
	}
	d.r.AppendSQL(b)
}
func (distinctFrom) resultType() bool { return false }

type between struct {
	e      Expr
	lo, hi any
}

func (x between) AppendSQL(b *Builder) {
	x.e.AppendSQL(b)
	b.WriteString(" BETWEEN ")
	b.Arg(x.lo)
	b.WriteString(" AND ")
	b.Arg(x.hi)
}
func (between) resultType() bool { return false }

type inList[T any] struct {
	e   Expr
	vs  []T
	not bool
}

func (x inList[T]) AppendSQL(b *Builder) {
	if len(x.vs) == 0 {
		if x.not {
			b.WriteString("1 = 1")
		} else {
			b.WriteString("1 = 0")
		}
		return
	}
	x.e.AppendSQL(b)
	if x.not {
		b.WriteString(" NOT IN (")
	} else {
		b.WriteString(" IN (")
	}
	for i, v := range x.vs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Arg(v)
	}
	b.WriteByte(')')
}
func (inList[T]) resultType() bool { return false }

type inQuery struct {
	e   Expr
	q   Subquery
	not bool
}

func (x inQuery) AppendSQL(b *Builder) {
	x.e.AppendSQL(b)
	if x.not {
		b.WriteString(" NOT IN ")
	} else {
		b.WriteString(" IN ")
	}
	x.q.AppendSQL(b)
}
func (inQuery) resultType() bool { return false }

type junction struct {
	op    string
	parts []Predicate
}

func (j junction) AppendSQL(b *Builder) {
	if len(j.parts) == 0 {
		if j.op == " AND " {
			b.WriteString("1 = 1")
		} else {
			b.WriteString("1 = 0")
		}
		return
	}
	if len(j.parts) == 1 {
		j.parts[0].AppendSQL(b)
		return
	}
	b.WriteByte('(')
	for i, p := range j.parts {
		if i > 0 {
			b.WriteString(j.op)
		}
		p.AppendSQL(b)
	}
	b.WriteByte(')')
}
func (junction) resultType() bool { return false }

// And joins predicates with AND; an empty And is true.
func And(ps ...Predicate) Predicate { return junction{" AND ", ps} }

// Or joins predicates with OR; an empty Or is false.
func Or(ps ...Predicate) Predicate { return junction{" OR ", ps} }

type not struct{ p Predicate }

func (n not) AppendSQL(b *Builder) { b.WriteString("NOT ("); n.p.AppendSQL(b); b.WriteByte(')') }
func (not) resultType() bool       { return false }

// Not negates a predicate.
func Not(p Predicate) Predicate { return not{p} }

type exists struct {
	q   Subquery
	not bool
}

func (e exists) AppendSQL(b *Builder) {
	if e.not {
		b.WriteString("NOT ")
	}
	b.WriteString("EXISTS ")
	e.q.AppendSQL(b)
}
func (exists) resultType() bool { return false }

// Exists is true when the subquery yields any row.
func Exists(q Subquery) Predicate { return exists{q, false} }

// NotExists is true when the subquery yields no row.
func NotExists(q Subquery) Predicate { return exists{q, true} }

// Like matches a string expression against a LIKE pattern. The pattern is
// bound as a parameter; escape wildcards in user input yourself.
func Like(e Expression[string], pattern string) Predicate {
	return binOp{e, " LIKE ", argExpr{pattern}}
}

// ILike is PostgreSQL's case-insensitive LIKE. On SQLite, whose LIKE is
// already ASCII case-insensitive by default, it is ErrUnsupported to avoid
// implying Unicode case folding SQLite does not perform.
func ILike(e Expression[string], pattern string) Predicate { return ilike{e, pattern} }

type ilike struct {
	e Expr
	p string
}

func (x ilike) AppendSQL(b *Builder) {
	if !b.d.ILike {
		b.unsupported("ILIKE")
		return
	}
	x.e.AppendSQL(b)
	b.WriteString(" ILIKE ")
	b.Arg(x.p)
}
func (ilike) resultType() bool { return false }

// ------------------------------------------------------------ functions

type fn struct {
	name string
	args []Expr
	star bool
}

func (f fn) AppendSQL(b *Builder) {
	b.WriteString(f.name)
	b.WriteByte('(')
	if f.star {
		b.WriteByte('*')
	}
	for i, a := range f.args {
		if i > 0 {
			b.WriteString(", ")
		}
		a.AppendSQL(b)
	}
	b.WriteByte(')')
}

type typedFn[T any] struct{ fn }

func (typedFn[T]) resultType() T { var z T; return z }

// Count is COUNT(*).
func Count() Expression[int64] { return typedFn[int64]{fn{name: "COUNT", star: true}} }

// CountOf is COUNT(expr), which skips NULLs.
func CountOf(e Expr) Expression[int64] { return typedFn[int64]{fn{name: "COUNT", args: []Expr{e}}} }

// CountDistinct is COUNT(DISTINCT expr).
func CountDistinct(e Expr) Expression[int64] {
	return typedFn[int64]{fn{name: "COUNT", args: []Expr{SQL("DISTINCT ?", e)}}}
}

// Sum is SUM(expr). The result is NULL over zero rows; scan it into a
// nullable type or wrap it in Coalesce.
func Sum[T any](e Expression[T]) Expression[T] { return typedFn[T]{fn{name: "SUM", args: []Expr{e}}} }

// Avg is AVG(expr) as float64 (NULL over zero rows).
func Avg[T any](e Expression[T]) Expression[float64] {
	return typedFn[float64]{fn{name: "AVG", args: []Expr{e}}}
}

// Min is MIN(expr) (NULL over zero rows).
func Min[T any](e Expression[T]) Expression[T] { return typedFn[T]{fn{name: "MIN", args: []Expr{e}}} }

// Max is MAX(expr) (NULL over zero rows).
func Max[T any](e Expression[T]) Expression[T] { return typedFn[T]{fn{name: "MAX", args: []Expr{e}}} }

// Coalesce returns the first non-NULL argument.
func Coalesce[T any](e Expression[T], fallback T) Expression[T] {
	return typedFn[T]{fn{name: "COALESCE", args: []Expr{e, argExpr{fallback}}}}
}

// ------------------------------------------------------------- raw SQL

type rawExpr struct {
	sql  string
	args []any
}

func (r rawExpr) AppendSQL(b *Builder) { b.appendRaw(r.sql, r.args) }
func (rawExpr) resultType() bool       { return false }

// SQL is an explicit escape hatch: a raw SQL fragment usable anywhere an
// expression or predicate is accepted.
//
// Safety contract: sql is inserted verbatim and must be a trusted constant.
// Every '?' is replaced by a bound parameter for the next argument (an Expr
// argument is rendered inline instead). Write "??" for a literal '?'. Never
// build sql from user input; pass user input as arguments.
func SQL(sql string, args ...any) Predicate { return rawExpr{sql, args} }

// TypedSQL is SQL with a declared result type, for use in projections and
// typed comparisons. Momo cannot verify that the declaration is truthful.
func TypedSQL[T any](sql string, args ...any) Expression[T] { return typed[T]{rawExpr{sql, args}} }

// Aliased renders "expr AS alias" in a select list.
type Aliased struct {
	Expr  Expr
	Alias string
}

func (a Aliased) AppendSQL(b *Builder) {
	a.Expr.AppendSQL(b)
	b.WriteString(" AS ")
	b.Ident(a.Alias)
}

// As aliases an expression in a select list.
func As(e Expr, alias string) Aliased { return Aliased{e, alias} }
