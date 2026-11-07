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
}

// ---------------------------------------------------------------- columns

// Column is a typed column descriptor generated for each model field. T is
// the column's Go value type with nullability removed (a *string field has
// a Column[string]-compatible NullColumn[string]).
//
// Column values are small, immutable and safe to share between goroutines.
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
func (c colCore[T]) resultType() T        { var z T; return z }

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

func (e ExcludedExpr[T]) AppendSQL(b *Builder) {
	if b.conflictTarget {
		b.SetErr(&Error{Kind: ErrValidation, Op: "insert", Err: fmt.Errorf("Excluded() column %q is not valid in the ON CONFLICT target predicate; use it in the DO UPDATE Where", e.name)})
		return
	}
	b.WriteString("EXCLUDED.")
	b.Ident(e.name)
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

// argExpr binds a value as a parameter. flags are the flags of the column it is
// compared with or assigned to; a sensitive column masks the value in hooks.
type argExpr struct {
	v     any
	flags uint16
}

func (a argExpr) AppendSQL(b *Builder) { b.argFor(a.flags, a.v) }

// flagged is implemented by column descriptors, whose flags decide masking.
type flagged interface{ Flags() uint16 }

// flagsOf returns the column flags of e, or 0 when e is not a column.
func flagsOf(e Expr) uint16 {
	if f, ok := e.(flagged); ok {
		return f.Flags()
	}
	return 0
}

// Value wraps a Go value as a typed bound parameter.
func Value[T any](v T) Expression[T] { return typed[T]{argExpr{v: v}} }

type typed[T any] struct{ Expr }

func (typed[T]) resultType() T { var z T; return z }
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
		d.l.AppendSQL(b)
		if d.not {
			b.WriteString(" IS ")
		} else {
			b.WriteString(" IS NOT ")
		}
		d.r.AppendSQL(b)
	case dialect.MySQL: // MySQL has the NULL-safe equality operator
		if !d.not {
			b.WriteString("NOT (")
		}
		d.l.AppendSQL(b)
		b.WriteString(" <=> ")
		d.r.AppendSQL(b)
		if !d.not {
			b.writeByte(')')
		}
	default:
		d.l.AppendSQL(b)
		if d.not {
			b.WriteString(" IS NOT DISTINCT FROM ")
		} else {
			b.WriteString(" IS DISTINCT FROM ")
		}
		d.r.AppendSQL(b)
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

// newAgg builds an aggregate whose comparison methods are bound to itself.
func newAgg[T any](f fn) Agg[T] {
	a := Agg[T]{fn: f}
	a.exprOps = selfOps[T](Agg[T]{fn: f})
	return a
}

func (Agg[T]) resultType() T { var z T; return z }

// As names the aggregate in a select list: SUM(col) AS name.
func (a Agg[T]) As(name string) Aliased[T] { return As[T](a, name) }

// CountAll is COUNT(*).
func CountAll() Agg[int64] { return newAgg[int64](fn{name: "COUNT", star: true}) }

// CountOf is COUNT(expr), which skips NULLs.
func CountOf(e Expr) Agg[int64] { return newAgg[int64](fn{name: "COUNT", args: []Expr{e}}) }

// CountDistinct is COUNT(DISTINCT expr).
func CountDistinct(e Expr) Agg[int64] {
	return newAgg[int64](fn{name: "COUNT", args: []Expr{SQL("DISTINCT ?", e)}})
}

// Sum is SUM(expr). The result is NULL over zero rows, so scan it into a
// nullable destination such as *int64 or *float64 (sql.Null types work too), or
// wrap it in Coalesce. Scanning NULL into a plain int64 returns an error.
func Sum[T Number](e Expression[T]) Agg[T] { return newAgg[T](fn{name: "SUM", args: []Expr{e}}) }

// Avg is AVG(expr) as float64. It is NULL over zero rows: scan it into *float64
// (or sql.NullFloat64), or wrap it in Coalesce.
func Avg(e Expression[float64]) Agg[float64] {
	return newAgg[float64](fn{name: "AVG", args: []Expr{e}})
}

// Min is MIN(expr) (NULL over zero rows). T is any type the database can
// order, such as time.Time, a decimal, or a string.
func Min[T any](e Expression[T]) Agg[T] { return newAgg[T](fn{name: "MIN", args: []Expr{e}}) }

// Max is MAX(expr) (NULL over zero rows).
func Max[T any](e Expression[T]) Agg[T] { return newAgg[T](fn{name: "MAX", args: []Expr{e}}) }

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

// validName checks an identifier-like name (alias, function name part).
func validName(s string) error {
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
