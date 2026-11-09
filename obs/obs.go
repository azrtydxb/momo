// SPDX-License-Identifier: Apache-2.0

// Package obs holds ready-made observability hooks for Momo: a Recorder that
// keeps every statement and relation load, an NPlusOne detector that reports a
// relation loaded once per row, and a log/slog hook.
//
// Each hook implements momo.Hook and momo.RelationHook, so one momo.WithHook
// call registers it for both statement events and relation load events. All
// of them are safe for concurrent use, as the hook contract requires.
//
// The hooks see the argument values the DB hands to every hook: nil unless the
// DB was opened with momo.WithArgLogging, and with the value bound from a
// sensitive column replaced by momo.RedactedArg. None of the hooks here can
// recover a masked value, and none of them logs an argument unless asked to.
package obs

import (
	"context"
	"slices"
	"sync"

	"github.com/azrtydxb/momo"
)

// Event is a copy of an AfterQuery event plus the value the Recorder put into
// the context in BeforeQuery.
type Event struct {
	momo.QueryEvent
	// CtxSQL is the SQL text the Recorder stored in the context in
	// BeforeQuery. It equals SQL when the context reached AfterQuery intact.
	CtxSQL string
}

type recorderKey struct{}

// Recorder is a momo.Hook and momo.RelationHook that records every finished
// statement and every finished relation load, in the order they finished. Use
// it in tests to assert on the statements a piece of code ran. The zero value
// is ready to use.
//
// Events carry the argument values as the DB reports them to hooks: masked
// with momo.RedactedArg for sensitive columns. The slices a Recorder returns
// are copies, so a caller cannot change what it holds.
type Recorder struct {
	mu     sync.Mutex
	events []Event
	rels   []momo.RelationEvent
}

var (
	_ momo.Hook         = (*Recorder)(nil)
	_ momo.RelationHook = (*Recorder)(nil)
)

// BeforeQuery stores the statement text in the returned context.
func (r *Recorder) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	return context.WithValue(ctx, recorderKey{}, e.SQL)
}

// AfterQuery records the finished statement.
func (r *Recorder) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
	s, _ := ctx.Value(recorderKey{}).(string)
	ev := Event{QueryEvent: *e, CtxSQL: s}
	ev.Args = slices.Clone(e.Args)
	r.mu.Lock()
	r.events = append(r.events, ev)
	r.mu.Unlock()
}

// BeforeRelation does nothing.
func (r *Recorder) BeforeRelation(ctx context.Context, _ *momo.RelationEvent) context.Context {
	return ctx
}

// AfterRelation records the finished relation load.
func (r *Recorder) AfterRelation(_ context.Context, e *momo.RelationEvent) {
	r.mu.Lock()
	r.rels = append(r.rels, *e)
	r.mu.Unlock()
}

// Events returns a copy of the recorded statements.
func (r *Recorder) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Event, len(r.events))
	for i, e := range r.events {
		out[i] = e
		out[i].Args = slices.Clone(e.Args)
	}
	return out
}

// SQLs returns the SQL text of each recorded statement.
func (r *Recorder) SQLs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.events))
	for i, e := range r.events {
		out[i] = e.SQL
	}
	return out
}

// Relations returns a copy of the recorded relation load events.
func (r *Recorder) Relations() []momo.RelationEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.rels)
}

// Reset forgets all recorded events.
func (r *Recorder) Reset() {
	r.mu.Lock()
	r.events = nil
	r.rels = nil
	r.mu.Unlock()
}
