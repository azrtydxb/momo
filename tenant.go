// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
)

type tenantKey struct{}

// WithTenant returns a context carrying tenant for TenantFilter. This key is
// unrelated to the setting that row-level security helpers read; see
// TenantFilter.
func WithTenant(ctx context.Context, tenant any) context.Context {
	return context.WithValue(ctx, tenantKey{}, tenant)
}

type tenantPredicate[T any] struct {
	col    Column[T]
	tenant T
	ok     bool
}

func (p tenantPredicate[T]) resultType() bool { return false }

func (p tenantPredicate[T]) AppendSQL(b *Builder) {
	if !p.ok {
		// Fail the build instead of emitting a constant: a constant false
		// predicate turns into "everything" under Not.
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("TenantFilter: no tenant of the column's type in the context")})
		b.appendRaw("1 = 0", nil)
		return
	}
	p.col.Eq(p.tenant).AppendSQL(b)
}

// TenantFilter returns col = <tenant from ctx>. When ctx has no tenant, or
// the tenant's dynamic type is not exactly T, building the query fails with
// ErrValidation ("no tenant in context"), so a missing tenant never widens a
// query, also not under Not or Or. The zero value of T (0, "") is a real
// tenant value and is filtered on like any other.
//
// TenantFilter is an application-side convenience, NOT a security boundary.
// It is applied only where you add it to a Where: not automatically on
// Update or Delete, not to the tenant column of an Insert, not to Preload,
// Load or LoadCounts queries, joins or subqueries, and not to raw SQL. Enforce
// isolation in the database with row-level security; the tenant that
// TenantFilter reads (WithTenant) is a different context value from the one
// the RLS helpers set on the connection.
func TenantFilter[T any](ctx context.Context, col Column[T]) Predicate {
	v, ok := ctx.Value(tenantKey{}).(T)
	return tenantPredicate[T]{col: col, tenant: v, ok: ok}
}
