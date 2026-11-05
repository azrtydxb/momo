// SPDX-License-Identifier: Apache-2.0

// Package obstest is test support for the observability tests: a recording
// hook that keeps the whole QueryEvent.
package obstest

import (
	"context"
	"sync"

	"github.com/azrtydxb/momo"
)

// Event is a copy of an AfterQuery event plus the value the same hook put
// into the context in BeforeQuery.
type Event struct {
	momo.QueryEvent
	CtxSQL string
}

type ctxKey struct{}

// Recorder is a momo.Hook that records every finished statement.
type Recorder struct {
	mu     sync.Mutex
	events []Event
}

func (r *Recorder) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	return context.WithValue(ctx, ctxKey{}, e.SQL)
}

func (r *Recorder) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
	s, ok := ctx.Value(ctxKey{}).(string)
	if !ok {
		s = ""
	}
	r.mu.Lock()
	r.events = append(r.events, Event{QueryEvent: *e, CtxSQL: s})
	r.mu.Unlock()
}

// Events returns a copy of the recorded events.
func (r *Recorder) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.events...)
}

// SQLs returns the SQL text of each recorded event.
func (r *Recorder) SQLs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.events))
	for i, e := range r.events {
		out[i] = e.SQL
	}
	return out
}

// Reset forgets all recorded events.
func (r *Recorder) Reset() {
	r.mu.Lock()
	r.events = nil
	r.mu.Unlock()
}
