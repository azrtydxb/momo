// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"slices"
)

// Preload returns a copy of the query that loads the given relations for every
// row that All, One and First return. The receiver is unchanged, and queries
// derived from one base never share their preload lists.
//
// The relations load with LoadAll after the parent rows are read: one
// statement per relation (per bind-limit chunk), run on the query's Querier,
// so inside a transaction they run in that transaction. A result with no rows
// issues no relation statement, and an error from a relation load is returned
// instead of the rows. Relation hooks fire as they do for LoadAll.
//
// Each and Iter stream rows without collecting them, so they cannot batch
// relation loads: with preloads they return an error matching ErrUnsupported.
func (sq SelectQuery[T]) Preload(loaders ...Loader[T]) SelectQuery[T] {
	add := make([]any, len(loaders))
	for i, l := range loaders {
		add[i] = l
	}
	sq.s.preloads = append(slices.Clip(sq.s.preloads), add...)
	return sq
}

// runPreloads loads all preloaded relations for rows in one LoadAll call, so
// that the batched path applies.
func (sq SelectQuery[T]) runPreloads(ctx context.Context, rows []T) error {
	if len(sq.s.preloads) == 0 || len(rows) == 0 {
		return nil
	}
	loaders := make([]Loader[T], len(sq.s.preloads))
	for i, p := range sq.s.preloads {
		loaders[i] = p.(Loader[T])
	}
	return LoadAll(ctx, sq.q, rows, loaders...)
}

// errStreamPreload is the error Each and Iter return when preloads are set.
func (sq SelectQuery[T]) errStreamPreload() error {
	if len(sq.s.preloads) == 0 {
		return nil
	}
	return &Error{Kind: ErrUnsupported, Op: "select", Table: sq.meta.Table,
		Err: errors.New("Preload is not supported with Each or Iter; use All, One or First")}
}
