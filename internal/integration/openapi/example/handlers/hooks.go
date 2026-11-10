// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"sync"
)

// Seen records the errors OnError was called with. The example's momo.yaml
// names OnError as the on_error option of the openapi plugin, so every handler
// passes each error it answers here before it writes the fixed message; an
// application would log, count or trace it instead.
var Seen = &ErrorLog{}

// Entry is one error the handlers answered.
type Entry struct {
	Op  string
	Err error
}

// ErrorLog is a safe list of Entry values.
type ErrorLog struct {
	mu      sync.Mutex
	entries []Entry
}

// OnError is the on_error hook of the generated handlers.
func OnError(_ context.Context, op string, err error) {
	Seen.mu.Lock()
	defer Seen.mu.Unlock()
	Seen.entries = append(Seen.entries, Entry{Op: op, Err: err})
}

// Entries returns a copy of what the hook has seen.
func (l *ErrorLog) Entries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Entry(nil), l.entries...)
}

// Reset forgets what the hook has seen.
func (l *ErrorLog) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = nil
}
