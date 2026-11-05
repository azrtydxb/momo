// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"time"
)

// RelationEvent describes one relation load (S-18). Children and Duration are
// filled before AfterRelation; Queries counts child statements issued.
type RelationEvent struct {
	Parent   string
	Name     string
	Kind     RelationKind
	Parents  int
	Children int64
	Queries  int
	Duration time.Duration
	Err      error
}

// RelationHook is an optional extension of Hook. Hooks registered with
// WithHook that implement it receive relation events.
type RelationHook interface {
	BeforeRelation(ctx context.Context, e *RelationEvent) context.Context
	AfterRelation(ctx context.Context, e *RelationEvent)
}

var noopRelationEnd = func(error) {}

// beginRelation calls BeforeRelation on relation hooks and returns the end
// function. Without relation hooks it allocates nothing.
func beginRelation(ctx context.Context, db *DB, e *RelationEvent) (context.Context, func(error)) {
	if db == nil || len(db.relHooks) == 0 {
		return ctx, noopRelationEnd
	}
	e.Duration = 0
	start := time.Now()
	for _, h := range db.relHooks {
		ctx = h.BeforeRelation(ctx, e)
	}
	return ctx, func(err error) {
		e.Err = err
		e.Duration = time.Since(start)
		for _, h := range db.relHooks {
			h.AfterRelation(ctx, e)
		}
	}
}
