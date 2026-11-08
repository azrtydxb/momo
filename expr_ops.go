// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"slices"
)

// Ops gives a scalar expression of result type T the comparison and ordering
// methods of Column: Eq, Ne, Lt, Le, Gt, Ge, the ...Expr forms, In, NotIn,
// Between, IsNull, IsNotNull, Asc and Desc. FuncExpr, CaseExpr, ArithExpr,
// CastExpr, ConcatExpr and LengthExpr embed it, so each method exists once.
//
// Ops is an extension point. Embedding momo.Ops[T] in your own struct makes
// that struct an Expression[T] (an Expression[bool] is a Predicate), which is
// how code outside this package defines custom expressions. Build the Ops
// value with NewOps and give it the finished host, then write the host's
// AppendSQL with Builder.Ident for identifiers and Builder.Arg (or ArgFor) for
// values, never by concatenating text into the SQL.
//
// The zero value is not bound to a host: rendering a comparison made from it
// fails the build with ErrValidation.
type Ops[T any] struct{ self Expr }

// exprOps is the in-package spelling of Ops, kept so the embedding sites read
// as they always did. There is one implementation.
type exprOps[T any] = Ops[T]

// NewOps binds the embedded comparison methods to host, the expression that
// embeds the returned Ops. Call it from the host's constructor on a copy of
// the host whose Ops field is still empty:
//
//	u := upper{src: src}
//	u.Ops = momo.NewOps[string](u)
//
// NewOps is an extension point; see Ops.
func NewOps[T any](host Expr) Ops[T] { return Ops[T]{host} }

// selfOps binds the embedded methods to the host expression e.
func selfOps[T any](e Expr) Ops[T] { return NewOps[T](e) }

// unboundExpr stands in for an expression value built without its
// constructor (a zero CaseExpr); rendering it fails the build.
type unboundExpr struct{}

func (unboundExpr) AppendSQL(b *Builder) {
	b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("expression was not built with its constructor")})
}

// resultType makes an embedding struct an Expression[T]. A host that declares
// its own resultType shadows this one.
func (Ops[T]) resultType() T { var z T; return z }

func (o Ops[T]) e() Expr {
	if o.self == nil {
		return unboundExpr{}
	}
	return o.self
}

// Eq is true when the expression equals v.
func (o Ops[T]) Eq(v T) Predicate {
	return binOp{o.e(), " = ", argExpr{v: v, flags: flagsOf(o.e())}}
}

// Ne is true when the expression differs from v.
func (o Ops[T]) Ne(v T) Predicate {
	return binOp{o.e(), " <> ", argExpr{v: v, flags: flagsOf(o.e())}}
}

// Lt is true when the expression is less than v.
func (o Ops[T]) Lt(v T) Predicate {
	return binOp{o.e(), " < ", argExpr{v: v, flags: flagsOf(o.e())}}
}

// Le is true when the expression is less than or equal to v.
func (o Ops[T]) Le(v T) Predicate {
	return binOp{o.e(), " <= ", argExpr{v: v, flags: flagsOf(o.e())}}
}

// Gt is true when the expression is greater than v.
func (o Ops[T]) Gt(v T) Predicate {
	return binOp{o.e(), " > ", argExpr{v: v, flags: flagsOf(o.e())}}
}

// Ge is true when the expression is greater than or equal to v.
func (o Ops[T]) Ge(v T) Predicate {
	return binOp{o.e(), " >= ", argExpr{v: v, flags: flagsOf(o.e())}}
}

// IsNull is true when the expression is NULL.
func (o Ops[T]) IsNull() Predicate { return postfix{o.e(), " IS NULL"} }

// IsNotNull is true when the expression is not NULL.
func (o Ops[T]) IsNotNull() Predicate { return postfix{o.e(), " IS NOT NULL"} }

// EqExpr is true when the expression equals x.
func (o Ops[T]) EqExpr(x Expression[T]) Predicate { return binOp{o.e(), " = ", x} }

// NeExpr is true when the expression differs from x.
func (o Ops[T]) NeExpr(x Expression[T]) Predicate { return binOp{o.e(), " <> ", x} }

// LtExpr is true when the expression is less than x.
func (o Ops[T]) LtExpr(x Expression[T]) Predicate { return binOp{o.e(), " < ", x} }

// LeExpr is true when the expression is less than or equal to x.
func (o Ops[T]) LeExpr(x Expression[T]) Predicate { return binOp{o.e(), " <= ", x} }

// GtExpr is true when the expression is greater than x.
func (o Ops[T]) GtExpr(x Expression[T]) Predicate { return binOp{o.e(), " > ", x} }

// GeExpr is true when the expression is greater than or equal to x.
func (o Ops[T]) GeExpr(x Expression[T]) Predicate { return binOp{o.e(), " >= ", x} }

// Between is inclusive on both ends.
func (o Ops[T]) Between(lo, hi T) Predicate {
	return between{e: o.e(), lo: lo, hi: hi, flags: flagsOf(o.e())}
}

// In matches any of vs; an empty list is always false.
func (o Ops[T]) In(vs ...T) Predicate {
	return inList[T]{o.e(), slices.Clone(vs), false, flagsOf(o.e())}
}

// NotIn is the negation of In; an empty list is always true.
func (o Ops[T]) NotIn(vs ...T) Predicate {
	return inList[T]{o.e(), slices.Clone(vs), true, flagsOf(o.e())}
}

// IsDistinctFrom is a NULL-safe inequality.
func (o Ops[T]) IsDistinctFrom(v T) Predicate {
	return distinctFrom{o.e(), argExpr{v: v, flags: flagsOf(o.e())}, false}
}

// IsNotDistinctFrom is a NULL-safe equality.
func (o Ops[T]) IsNotDistinctFrom(v T) Predicate {
	return distinctFrom{o.e(), argExpr{v: v, flags: flagsOf(o.e())}, true}
}

// InQuery matches the single-column result of a subquery.
func (o Ops[T]) InQuery(q Subquery) Predicate { return inQuery{o.e(), q, false} }

// NotInQuery is the negation of InQuery. Beware SQL's NOT IN semantics when
// the subquery can yield NULL.
func (o Ops[T]) NotInQuery(q Subquery) Predicate { return inQuery{o.e(), q, true} }

// Asc orders by the expression ascending.
func (o Ops[T]) Asc() Order { return Order{Expr: o.e()} }

// Desc orders by the expression descending.
func (o Ops[T]) Desc() Order { return Order{Expr: o.e(), Desc: true} }

func (o Ops[T]) order() Order { return Order{Expr: o.e()} }

// aliasRef renders a select-list alias by name, for ORDER BY.
type aliasRef string

func (a aliasRef) AppendSQL(b *Builder) { b.Ident(string(a)) }

// isAliasOrder reports whether o orders by a select-list alias.
func isAliasOrder(o Order) bool {
	_, ok := o.Expr.(aliasRef)
	return ok
}

// Asc orders by the alias ascending.
func (a Aliased[T]) Asc() Order { return Order{Expr: aliasRef(a.alias)} }

// Desc orders by the alias descending.
func (a Aliased[T]) Desc() Order { return Order{Expr: aliasRef(a.alias), Desc: true} }

func (a Aliased[T]) order() Order { return a.Asc() }

func (Aliased[T]) isAliased() {}

// rejectAliased records ErrValidation when e is an Aliased select-list item,
// which is valid only in a select list.
func rejectAliased(err *error, clause string, e Expr) {
	if _, ok := e.(interface{ isAliased() }); ok && *err == nil {
		*err = &Error{Kind: ErrValidation, Op: "build", Err: errors.New("an As(...) alias is only valid in a select list, not in " + clause + "; use the aliased expression itself")}
	}
}
