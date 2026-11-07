// SPDX-License-Identifier: Apache-2.0

package momo

import "context"

type tenantKey struct{}

// WithTenant returns a context carrying tenant for TenantFilter.
func WithTenant(ctx context.Context, tenant any) context.Context {
	return context.WithValue(ctx, tenantKey{}, tenant)
}

// TenantFilter returns col = <tenant from ctx>. When ctx has no tenant, or
// the tenant's dynamic type is not exactly T, it returns the always-false
// predicate 1 = 0, so a missing tenant never widens a query.
//
// TenantFilter is a convenience, NOT a security boundary: a query that omits
// it, raw SQL, or another connection sees every tenant's rows. Enforce
// isolation in the database with row-level security and use this only to
// avoid repeating the predicate.
func TenantFilter[T any](ctx context.Context, col Column[T]) Predicate {
	if v, ok := ctx.Value(tenantKey{}).(T); ok {
		return col.Eq(v)
	}
	return SQL("1 = 0")
}
