// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"slices"
	"unicode"

	"github.com/azrtydxb/momo/dialect"
)

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
	// ParseEq parses raw with the column's Go type and returns the equality
	// predicate; a value that does not parse is an ErrValidation error.
	ParseEq(raw string) (Predicate, error)
	// ParseCmp is ParseEq for op eq, ne, lt, le, gt or ge.
	ParseCmp(op, raw string) (Predicate, error)
}

// ---------------------------------------------------------------- columns

// Column is a typed column descriptor generated for each model field. T is
// the column's Go value type with nullability removed (a *string field has
// a Column[string]-compatible NullColumn[string]).
//
// Column values are small, immutable and safe to share between goroutines.
// Build one with NewColumn, ColumnOf or the generated descriptors: the zero
// value Column[T]{} has no column reference, so using it in a comparison
// fails the build with ErrValidation.
type Column[T any] struct{ colCore[T] }

// colRef is the SQL reference of a column: its qualifier, name and flags.
type colRef struct {
	table string // table name or alias used as qualifier
	name  string
	flags uint16
}

func (r colRef) AppendSQL(b *Builder) {
	if r.flags&FlagSensitive != 0 {
		b.sensSeen++
	}
	b.QualifiedIdent(r.table, r.name)
}

// Flags returns the column flags, which decide argument masking.
func (r colRef) Flags() uint16 { return r.flags }

// ColumnName and Table let the reference satisfy ColumnRef wherever a
// comparison or ordering operand is inspected.
func (r colRef) ColumnName() string { return r.name }
func (r colRef) Table() string      { return r.table }

// colCore is the comparison, ordering and metadata set shared by Column and
// the permission wrappers (ReadOnlyColumn and friends). It embeds exprOps,
// which holds the one implementation of the comparison methods, bound to the
// column's colRef. It carries no write method.
type colCore[T any] struct{ exprOps[T] }

func newColCore[T any](table, name string, flags uint16) colCore[T] {
	return colCore[T]{selfOps[T](colRef{table: table, name: name, flags: flags})}
}

func (c colCore[T]) ref() colRef {
	r, _ := c.self.(colRef)
	return r
}

func (c colCore[T]) withAlias(alias string) colCore[T] {
	r := c.ref()
	return newColCore[T](alias, r.name, r.flags)
}

func (c colCore[T]) AppendSQL(b *Builder) { c.ref().AppendSQL(b) }

// Asc orders by the column ascending. The order item is the column itself,
// a ColumnRef, so keyset pagination recognizes a primary-key column.
func (c colCore[T]) Asc() Order { return Order{Expr: c} }

// Desc orders by the column descending.
func (c colCore[T]) Desc() Order { return Order{Expr: c, Desc: true} }

func (c colCore[T]) order() Order  { return Order{Expr: c} }
func (c colCore[T]) resultType() T { var z T; return z }

// AsExpr returns the column as an Expression[T], so a projection list reads
// explicitly: DocTitle.As("l").AsExpr().
func (c colCore[T]) AsExpr() Expression[T] { return c }

// ColumnName returns the unquoted column name.
func (c colCore[T]) ColumnName() string { return c.ref().name }

// Table returns the qualifier (table name or alias).
func (c colCore[T]) Table() string { return c.ref().table }

// Flags returns the generated column flags.
func (c colCore[T]) Flags() uint16 { return c.ref().flags }

// Excluded is the value proposed for insertion in an upsert, rendered as
// EXCLUDED.<col>. Use it on the right-hand side of a comparison in the
// DO UPDATE WHERE or ON CONFLICT predicates, for example
// until.LtExpr(until.Excluded()). It is only meaningful inside an ON CONFLICT
// clause; the database rejects it elsewhere.
func (c colCore[T]) Excluded() ExcludedExpr[T] {
	e := ExcludedExpr[T]{name: c.ref().name}
	e.exprOps = selfOps[T](e)
	return e
}

// NewColumn is used by generated code.
func NewColumn[T any](table, name string, flags uint16) Column[T] {
	return Column[T]{newColCore[T](table, name, flags)}
}

// As returns the same column qualified by a table alias.
func (c Column[T]) As(alias string) Column[T] { return Column[T]{c.withAlias(alias)} }

// Set assigns a value (including the zero value) in INSERT or UPDATE.
func (c Column[T]) Set(v T) Assignment {
	return Assignment{col: c.ColumnName(), kind: setValue, val: v, flags: c.Flags()}
}

// SetExpr assigns an expression.
func (c Column[T]) SetExpr(e Expression[T]) Assignment {
	return Assignment{col: c.ColumnName(), kind: setExpr, expr: e, flags: c.Flags()}
}

// SetDefault assigns the column's database default. PostgreSQL supports it
// in INSERT and UPDATE; SQLite only by omitting the column from an INSERT,
// so an UPDATE with SetDefault fails with ErrUnsupported there.
func (c Column[T]) SetDefault() Assignment { return Assignment{col: c.ColumnName(), kind: setDefault} }

// SetExcluded assigns the value proposed for insertion in an upsert
// (ON CONFLICT ... DO UPDATE SET col = excluded.col).
func (c Column[T]) SetExcluded() Assignment {
	return Assignment{col: c.ColumnName(), kind: setExcluded}
}

// ExcludedExpr is the EXCLUDED.<col> reference returned by Column.Excluded.
// Besides being an Expression[T], it carries value comparisons for CASE and
// DO UPDATE WHERE predicates, for example hostname.Excluded().Ne("").
type ExcludedExpr[T any] struct {
	exprOps[T]
	name string
}

// AppendSQL writes the EXCLUDED column reference to b.
func (e ExcludedExpr[T]) AppendSQL(b *Builder) {
	if b.conflictTarget {
		b.SetErr(&Error{Kind: ErrValidation, Op: "insert", Err: fmt.Errorf("Excluded() column %q is not valid in the ON CONFLICT target predicate; use it in the DO UPDATE Where", e.name)})
		return
	}
	b.writeExcluded(e.name)
}
func (ExcludedExpr[T]) resultType() T { var z T; return z }

// NullColumn is the descriptor for a nullable column. It adds SetNull.
type NullColumn[T any] struct{ Column[T] }

// NewNullColumn is used by generated code.
func NewNullColumn[T any](table, name string, flags uint16) NullColumn[T] {
	return NullColumn[T]{NewColumn[T](table, name, flags|FlagNullable)}
}

// As returns the same column qualified by a table alias.
func (c NullColumn[T]) As(alias string) NullColumn[T] { return NullColumn[T]{c.Column.As(alias)} }

// SetNull assigns SQL NULL.
func (c NullColumn[T]) SetNull() Assignment { return Assignment{col: c.ColumnName(), kind: setNull} }

// SetPtr assigns NULL when v is nil and *v otherwise, so a model field of type
// *T can be written without branching at the call site. It is an ordinary
// Assignment: use it in Update.Set, Insert.Values and an upsert DoUpdate.
func (c NullColumn[T]) SetPtr(v *T) Assignment {
	if v == nil {
		return c.SetNull()
	}
	return c.Set(*v)
}

// EqPtr compares the column with *v, or tests IS NULL when v is nil. It never
// renders "= NULL", which matches no row.
func (c NullColumn[T]) EqPtr(v *T) Predicate {
	if v == nil {
		return c.IsNull()
	}
	return c.Eq(*v)
}

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
	// flags are the column flags; FlagSensitive masks bound values in hooks.
	flags uint16
}

// ColumnName returns the assigned column.
func (a Assignment) ColumnName() string { return a.col }

// ------------------------------------------------------------- ordering

// Nulls placement for ORDER BY.
type Nulls uint8

const (
	// NullsDefault leaves NULL placement to the database.
	NullsDefault Nulls = iota
	// NullsFirst sorts NULLs before other values.
	NullsFirst
	// NullsLast sorts NULLs after other values.
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

// AppendOrder renders the item as "expr ASC|DESC [NULLS FIRST|LAST]", failing
// the build with ErrUnsupported where the dialect has no NULLS ordering.
// Code outside this package that renders an ORDER BY inside its own
// expression (an ordered aggregate) calls it. AppendOrder is an extension
// point; see Ops.
func (o Order) AppendOrder(b *Builder) { o.appendSQL(b) }

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

// argExpr binds a value as a parameter. flags are the flags of the column it is
// compared with or assigned to; a sensitive column masks the value in hooks.
type argExpr struct {
	v     any
	flags uint16
}

func (a argExpr) AppendSQL(b *Builder) { b.argFor(a.flags, a.v) }

// Flagged is implemented by column descriptors, whose flags decide masking.
// A custom expression that wraps a column implements it by returning
// FlagsOf of the wrapped expression, so a sensitive column stays masked
// through the wrapper. Flagged is an extension point; see Ops.
type Flagged interface{ Flags() uint16 }

// FlagsOf returns the column flags of e, or 0 when e does not implement
// Flagged. Pass the result to Builder.ArgFor, Builder.RenderSensitive or
// Builder.ExprFor so values compared with a sensitive column are masked in
// hook events. FlagsOf is an extension point; see Ops.
func FlagsOf(e Expr) uint16 {
	if f, ok := e.(Flagged); ok {
		return f.Flags()
	}
	return 0
}

// flagsOf is the in-package spelling of FlagsOf.
func flagsOf(e Expr) uint16 { return FlagsOf(e) }

// Value wraps a Go value as a typed bound parameter.
func Value[T any](v T) Expression[T] { return typed[T]{argExpr{v: v}} }

// Typed wraps e as an Expression[T], for an expression whose SQL is built
// elsewhere and whose result type the caller knows. It does not change the
// SQL e renders and adds no comparison methods; embed Ops[T] in your own type
// for those. Typed is an extension point: e must write identifiers through
// Builder.Ident and values through Builder.Arg.
func Typed[T any](e Expr) Expression[T] { return typed[T]{e} }

// IsBound reports whether e is a Go value bound as a parameter by Value,
// possibly wrapped by Typed. A feature package uses it to cast a bound operand
// whose type the server cannot infer (a timestamp inside EXTRACT on
// PostgreSQL). It is false for a column, a function call or any other
// expression. IsBound is an extension point; see Ops.
func IsBound(e Expr) bool {
	for {
		w, ok := e.(interface{ inner() Expr })
		if !ok {
			break
		}
		e = w.inner()
	}
	_, bound := e.(argExpr)
	return bound
}

// PredicateFunc adapts a render function to a Predicate. The function writes
// the condition into the builder, using Builder.Ident for identifiers and
// Builder.Arg or Builder.ArgFor for values. PredicateFunc is an extension
// point; see Ops.
type PredicateFunc func(b *Builder)

// AppendSQL calls f.
func (f PredicateFunc) AppendSQL(b *Builder) { f(b) }

func (PredicateFunc) resultType() bool { return false }

type typed[T any] struct{ Expr }

func (typed[T]) resultType() T { var z T; return z }

// inner returns the expression a typed wrapper adds a result type to.
func (t typed[T]) inner() Expr { return t.Expr }

func (t typed[T]) isRaw() bool {
	r, ok := t.Expr.(interface{ isRaw() bool })
	return ok && r.isRaw()
}

type binOp struct {
	l  Expr
	op string
	r  Expr
}

func (o binOp) AppendSQL(b *Builder) {
	// A sensitive column on either side masks every argument of the other.
	fl := b.renderSens(o.l, flagsOf(o.l)|flagsOf(o.r))
	b.WriteString(o.op)
	b.exprFor(fl, o.r)
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
	switch b.d.Name {
	case dialect.SQLite: // SQLite spells it IS / IS NOT
		fl := b.renderSens(d.l, flagsOf(d.l)|flagsOf(d.r))
		if d.not {
			b.WriteString(" IS ")
		} else {
			b.WriteString(" IS NOT ")
		}
		b.exprFor(fl, d.r)
	case dialect.MySQL: // MySQL has the NULL-safe equality operator
		if !d.not {
			b.WriteString("NOT (")
		}
		fl := b.renderSens(d.l, flagsOf(d.l)|flagsOf(d.r))
		b.WriteString(" <=> ")
		b.exprFor(fl, d.r)
		if !d.not {
			b.writeByte(')')
		}
	default:
		fl := b.renderSens(d.l, flagsOf(d.l)|flagsOf(d.r))
		if d.not {
			b.WriteString(" IS NOT DISTINCT FROM ")
		} else {
			b.WriteString(" IS DISTINCT FROM ")
		}
		b.exprFor(fl, d.r)
	}
}
func (distinctFrom) resultType() bool { return false }

type between struct {
	e      Expr
	lo, hi any
	flags  uint16
}

func (x between) AppendSQL(b *Builder) {
	fl := b.renderSens(x.e, x.flags)
	b.WriteString(" BETWEEN ")
	b.argFor(fl, x.lo)
	b.WriteString(" AND ")
	b.argFor(fl, x.hi)
}
func (between) resultType() bool { return false }

type inList[T any] struct {
	e     Expr
	vs    []T
	not   bool
	flags uint16
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
	if m := b.d.MaxInList; m > 0 && len(x.vs) > m {
		b.SetErr(&Error{Kind: ErrLimitExceeded, Op: "build", Err: fmt.Errorf("IN list has %d values, over the limit of %d (WithMaxInList)", len(x.vs), m)})
		b.WriteString("1 = 0")
		return
	}
	if !b.checkBindCap(len(x.vs), "IN") {
		b.WriteString("1 = 0")
		return
	}
	fl := b.renderSens(x.e, x.flags)
	if x.not {
		b.WriteString(" NOT IN (")
	} else {
		b.WriteString(" IN (")
	}
	for i, v := range x.vs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.argFor(fl, v)
	}
	b.writeByte(')')
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
	b.writeByte('(')
	for i, p := range j.parts {
		if i > 0 {
			b.WriteString(j.op)
		}
		if r, ok := p.(interface{ isRaw() bool }); ok && r.isRaw() {
			// Raw SQL may hold a lower-precedence operator; every
			// structured predicate is self-delimiting.
			b.writeByte('(')
			p.AppendSQL(b)
			b.writeByte(')')
			continue
		}
		p.AppendSQL(b)
	}
	b.writeByte(')')
}
func (junction) resultType() bool { return false }

// And joins predicates with AND; an empty And is true.
func And(ps ...Predicate) Predicate { return newJunction(" AND ", ps) }

// Or joins predicates with OR; an empty Or is false.
func Or(ps ...Predicate) Predicate { return newJunction(" OR ", ps) }

// newJunction returns a lone part as itself so that an enclosing conjunction
// still groups it.
func newJunction(op string, ps []Predicate) Predicate {
	if len(ps) == 1 {
		return ps[0]
	}
	return junction{op, slices.Clone(ps)}
}

type not struct{ p Predicate }

func (n not) AppendSQL(b *Builder) { b.WriteString("NOT ("); n.p.AppendSQL(b); b.writeByte(')') }
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
// bound as a parameter and the statement always carries ESCAPE '!' on every
// dialect, the escape character EscapeLike uses. % and _ in the pattern stay
// wildcards; a ! in the pattern makes the next character literal. Escape user
// input with EscapeLike (or use ContainsLiteral) so it cannot act as a
// wildcard. A literal ! in a hand-written pattern must be written as !!.
func Like(e Expression[string], pattern string) Predicate {
	return like{e: e, p: pattern, flags: flagsOf(e)}
}

type like struct {
	e     Expr
	p     string
	flags uint16
}

func (x like) AppendSQL(b *Builder) {
	fl := b.renderSens(x.e, x.flags)
	b.WriteString(" LIKE ")
	b.argFor(fl, x.p)
	b.WriteString(likeEscape)
}
func (like) resultType() bool { return false }

// likeEscape is the clause every LIKE form appends; the character must match
// EscapeLike.
const likeEscape = ` ESCAPE '!'`

// ILike is PostgreSQL's case-insensitive LIKE. On SQLite, whose LIKE is
// already ASCII case-insensitive by default, it is ErrUnsupported to avoid
// implying Unicode case folding SQLite does not perform. Like Like, it always
// carries ESCAPE '!' to match EscapeLike.
func ILike(e Expression[string], pattern string) Predicate { return ilike{e, pattern, flagsOf(e)} }

type ilike struct {
	e     Expr
	p     string
	flags uint16
}

func (x ilike) AppendSQL(b *Builder) {
	if !b.d.ILike {
		b.unsupported("ILIKE")
		return
	}
	fl := b.renderSens(x.e, x.flags)
	b.WriteString(" ILIKE ")
	b.argFor(fl, x.p)
	b.WriteString(likeEscape)
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
	b.writeByte('(')
	if f.star {
		b.writeByte('*')
	}
	for i, a := range f.args {
		if i > 0 {
			b.WriteString(", ")
		}
		a.AppendSQL(b)
	}
	b.writeByte(')')
}

type typedFn[T any] struct{ fn }

func (typedFn[T]) resultType() T { var z T; return z }

// Number is the constraint of the numeric aggregates SUM.
type Number interface{ ~int64 | ~float64 }

// Agg is a typed aggregate expression such as COUNT(*) or SUM(col). Besides
// being an Expression[T] usable in a select list, it carries the comparison
// methods HAVING needs.
type Agg[T any] struct {
	fn
	exprOps[T]
}

// newFnAgg builds an aggregate whose comparison methods are bound to itself.
func newFnAgg[T any](f fn) Agg[T] {
	a := Agg[T]{fn: f}
	a.exprOps = selfOps[T](Agg[T]{fn: f})
	return a
}

func (Agg[T]) resultType() T { var z T; return z }

// As names the aggregate in a select list: SUM(col) AS name.
func (a Agg[T]) As(name string) Aliased[T] { return As[T](a, name) }

// CountAll is COUNT(*).
func CountAll() Agg[int64] { return newFnAgg[int64](fn{name: "COUNT", star: true}) }

// CountOf is COUNT(expr), which skips NULLs.
func CountOf(e Expr) Agg[int64] { return newFnAgg[int64](fn{name: "COUNT", args: []Expr{e}}) }

// CountDistinct is COUNT(DISTINCT expr).
func CountDistinct(e Expr) Agg[int64] {
	return newFnAgg[int64](fn{name: "COUNT", args: []Expr{SQL("DISTINCT ?", e)}})
}

// Sum is SUM(expr). The result is NULL over zero rows, so scan it into a
// nullable destination such as *int64 or *float64 (sql.Null types work too), or
// wrap it in Coalesce. Scanning NULL into a plain int64 returns an error.
func Sum[T Number](e Expression[T]) Agg[T] { return newFnAgg[T](fn{name: "SUM", args: []Expr{e}}) }

// Avg is AVG(expr) as float64. It is NULL over zero rows: scan it into *float64
// (or sql.NullFloat64), or wrap it in Coalesce.
func Avg(e Expression[float64]) Agg[float64] {
	return newFnAgg[float64](fn{name: "AVG", args: []Expr{e}})
}

// Min is MIN(expr) (NULL over zero rows). T is any type the database can
// order, such as time.Time, a decimal, or a string.
func Min[T any](e Expression[T]) Agg[T] { return newFnAgg[T](fn{name: "MIN", args: []Expr{e}}) }

// Max is MAX(expr) (NULL over zero rows).
func Max[T any](e Expression[T]) Agg[T] { return newFnAgg[T](fn{name: "MAX", args: []Expr{e}}) }

// Coalesce returns the first non-NULL argument.
func Coalesce[T any](e Expression[T], fallback T) Expression[T] {
	return typedFn[T]{fn{name: "COALESCE", args: []Expr{e, argExpr{v: fallback}}}}
}

// ------------------------------------------------------------- raw SQL

type rawExpr struct {
	sql  string
	args []any
}

func (r rawExpr) AppendSQL(b *Builder) { b.appendRaw(r.sql, r.args) }
func (rawExpr) resultType() bool       { return false }
func (rawExpr) isRaw() bool            { return true }

// SQL is an explicit escape hatch: a raw SQL fragment usable anywhere an
// expression or predicate is accepted.
//
// Safety contract: sql is inserted verbatim and must be a trusted constant.
// Every '?' is replaced by a bound parameter for the next argument (an Expr
// argument is rendered inline instead). Write "??" for a literal '?'. Never
// build sql from user input; pass user input as arguments.
//
// Raw arguments: In(slice) expands to a bind list (an empty slice is a build
// error), Ident(name) renders a quoted identifier, and @name binds Named(name,
// value). A ? or @name inside a string literal, quoted identifier, comment or
// dollar-quoted string is text, as are the PostgreSQL operators ?| ?& @? @@ @>.
//
// On PostgreSQL, native $n placeholders bind positional argument n instead
// of ?, and a reused $n shares one bind. A fragment uses one style: mixing
// $n with ? or @name is ErrValidation, a $n with no argument or an argument
// no $n uses is ErrValidation, and a $n on another dialect is ErrUnsupported.
// The number is the position within this fragment's own arguments, not the
// final statement's, so a fragment embedded after other binds is renumbered.
func SQL(sql string, args ...any) Predicate { return rawExpr{sql, slices.Clone(args)} }

// TypedSQL is SQL with a declared result type, for use in projections and
// typed comparisons. Momo cannot verify that the declaration is truthful.
func TypedSQL[T any](sql string, args ...any) Expression[T] {
	return typed[T]{rawExpr{sql, slices.Clone(args)}}
}

// Aliased is a select-list item "expr AS alias". T is the expression's
// result type, which Ref carries to the outer query.
type Aliased[T any] struct {
	e     Expr
	alias string
}

// AppendSQL writes the expression and its alias to b.
func (a Aliased[T]) AppendSQL(b *Builder) {
	if err := validName(a.alias); err != nil {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf("alias %q: %w", a.alias, err)})
		return
	}
	a.e.AppendSQL(b)
	b.WriteString(" AS ")
	b.Ident(a.alias)
}

// Ref is the typed column through which an outer query reads this alias from
// a subquery (FromSubquery alias) or CTE (name) called source. Momo cannot
// check that source exposes the alias.
func (a Aliased[T]) Ref(source string) Column[T] { return ColumnOf[T](source, a.alias) }

// As aliases an expression in a select list: expr AS alias. The alias is
// quoted and must be a plain identifier (letters, digits, underscore, "$",
// not starting with a digit, at most 63 bytes), otherwise the build fails
// with ErrValidation. Use it on columns, aggregates, CASE, function calls
// and TypedSQL fragments; to alias a scalar subquery wrap it in
// TypedSQL[T] with "?". Column.As is the table alias, so use
// As[T](col, name) for a column.
func As[T any](e Expression[T], alias string) Aliased[T] { return Aliased[T]{e, alias} }

// validName is the in-package spelling of ValidName.
func validName(s string) error { return ValidName(s) }

// ValidName checks an identifier-like name: non-empty, at most 63 bytes, a
// letter or underscore first, then letters, digits, underscores or dollar
// signs. It returns a descriptive error otherwise. Code outside the package
// uses it to check a function name or alias before writing it with
// Builder.Ident. ValidName is an extension point; see Ops.
func ValidName(s string) error {
	if s == "" {
		return errors.New("name is empty")
	}
	if len(s) > 63 {
		return errors.New("name is longer than 63 bytes")
	}
	for i, r := range s {
		switch {
		case r == '_' || unicode.IsLetter(r):
		case i > 0 && (unicode.IsDigit(r) || r == '$'):
		default:
			return fmt.Errorf("invalid character %q", r)
		}
	}
	return nil
}
