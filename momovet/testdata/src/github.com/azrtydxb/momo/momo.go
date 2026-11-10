// SPDX-License-Identifier: Apache-2.0

// Package momo is a stub of the raw-SQL surface of github.com/azrtydxb/momo
// for the analyzer fixtures: same names and parameter positions, no behavior.
package momo

import "context"

type Querier interface{}

type DB struct{}

type Predicate interface{}

type Expression[T any] interface{}

type Subquery interface{}

func Exec(ctx context.Context, q Querier, sql string, args ...any) (int64, error) { return 0, nil }

func Scalars[V any](ctx context.Context, q Querier, sql string, args ...any) ([]V, error) {
	return nil, nil
}

func MapRows(ctx context.Context, q Querier, sql string, args ...any) ([]map[string]any, error) {
	return nil, nil
}

func Query[T any](ctx context.Context, q Querier, sql string, args ...any) ([]T, error) {
	return nil, nil
}

func QueryRows[T any](ctx context.Context, q Querier, sql string, args ...any) func(func(T, error) bool) {
	return nil
}

func SQL(sql string, args ...any) Predicate { return nil }

func TypedSQL[T any](sql string, args ...any) Expression[T] { return nil }

func RawSubquery(sql string, args ...any) Subquery { return nil }
