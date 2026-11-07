// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"slices"
)

// exprOps gives a scalar expression of result type T the comparison and
// ordering methods of Column: Eq, Ne, Lt, Le, Gt, Ge, the ...Expr forms, In,
// NotIn, Between, IsNull, IsNotNull, Asc and Desc. FuncExpr, CaseExpr,
// ArithExpr, CastExpr, ConcatExpr and LengthExpr embed it, so each method
// exists once. self is the host expression with its own exprOps left empty;
// the host's constructor sets it with selfOps.
type exprOps[T any] struct{ self Expr }

// selfOps binds the embedded methods to the host expression e.
func selfOps[T any](e Expr) exprOps[T] { return exprOps[T]{e} }

// unboundExpr stands in for an expression value built without its
// constructor (a zero CaseExpr); rendering it fails the build.
type unboundExpr struct{}

func (unboundExpr) AppendSQL(b *Builder) {
	b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("expression was not built with its constructor")})
}

func (o exprOps[T]) e() Expr {
	if o.self == nil {
		return unboundExpr{}
	}
	return o.self
}

func (o exprOps[T]) Eq(v T) Predicate  { return binOp{o.e(), " = ", argExpr{v: v}} }
func (o exprOps[T]) Ne(v T) Predicate  { return binOp{o.e(), " <> ", argExpr{v: v}} }
func (o exprOps[T]) Lt(v T) Predicate  { return binOp{o.e(), " < ", argExpr{v: v}} }
func (o exprOps[T]) Le(v T) Predicate  { return binOp{o.e(), " <= ", argExpr{v: v}} }
func (o exprOps[T]) Gt(v T) Predicate  { return binOp{o.e(), " > ", argExpr{v: v}} }
func (o exprOps[T]) Ge(v T) Predicate  { return binOp{o.e(), " >= ", argExpr{v: v}} }
func (o exprOps[T]) IsNull() Predicate { return postfix{o.e(), " IS NULL"} }

// IsNotNull is true when the expression is not NULL.
func (o exprOps[T]) IsNotNull() Predicate { return postfix{o.e(), " IS NOT NULL"} }

func (o exprOps[T]) EqExpr(x Expression[T]) Predicate { return binOp{o.e(), " = ", x} }
func (o exprOps[T]) NeExpr(x Expression[T]) Predicate { return binOp{o.e(), " <> ", x} }
func (o exprOps[T]) LtExpr(x Expression[T]) Predicate { return binOp{o.e(), " < ", x} }
func (o exprOps[T]) LeExpr(x Expression[T]) Predicate { return binOp{o.e(), " <= ", x} }
func (o exprOps[T]) GtExpr(x Expression[T]) Predicate { return binOp{o.e(), " > ", x} }
func (o exprOps[T]) GeExpr(x Expression[T]) Predicate { return binOp{o.e(), " >= ", x} }

// Between is inclusive on both ends.
func (o exprOps[T]) Between(lo, hi T) Predicate {
	return between{e: o.e(), lo: lo, hi: hi, flags: flagsOf(o.e())}
}

// In matches any of vs; an empty list is always false.
func (o exprOps[T]) In(vs ...T) Predicate {
	return inList[T]{o.e(), slices.Clone(vs), false, flagsOf(o.e())}
}

// NotIn is the negation of In; an empty list is always true.
func (o exprOps[T]) NotIn(vs ...T) Predicate {
	return inList[T]{o.e(), slices.Clone(vs), true, flagsOf(o.e())}
}

// Asc orders by the expression ascending.
func (o exprOps[T]) Asc() Order { return Order{Expr: o.e()} }

// Desc orders by the expression descending.
func (o exprOps[T]) Desc() Order { return Order{Expr: o.e(), Desc: true} }

func (o exprOps[T]) order() Order { return Order{Expr: o.e()} }

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
