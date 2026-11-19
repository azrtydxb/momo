// SPDX-License-Identifier: Apache-2.0

// Package nplusone holds the scope and the counting rules shared by the two
// N+1 detectors. The root package cannot import obs (obs imports the root), so
// the deprecated momo.NewNPlusOneHook and momo.WithQueryScope and the obs
// detectors all count here. One scope serves both kinds of counting: statement
// shapes and relation loads.
package nplusone

import (
	"context"
	"regexp"
	"sync"
)

// MaxKeys bounds the memory one scope can hold: at most this many distinct
// statement shapes and, separately, this many distinct relations.
const MaxKeys = 1024

type scopeKey struct{}

// counter counts occurrences per key and remembers which keys were reported.
type counter struct {
	counts   map[string]int
	reported map[string]bool
}

// add counts one occurrence of key. It returns the new count and whether the
// threshold was just exceeded for the first time. A key that is not tracked yet
// is ignored once MaxKeys keys are tracked.
func (c *counter) add(key string, threshold int) (int, bool) {
	if c.counts == nil {
		c.counts = map[string]int{}
		c.reported = map[string]bool{}
	}
	if _, seen := c.counts[key]; !seen && len(c.counts) >= MaxKeys {
		return 0, false
	}
	c.counts[key]++
	n := c.counts[key]
	fire := n > threshold && !c.reported[key]
	if fire {
		c.reported[key] = true
	}
	return n, fire
}

// Scope is the state of one unit of work.
type Scope struct {
	mu        sync.Mutex
	shapes    counter
	relations counter
}

// With returns a context that carries a new Scope.
func With(ctx context.Context) context.Context {
	return context.WithValue(ctx, scopeKey{}, &Scope{})
}

// From returns the scope of ctx, or nil when ctx has none.
func From(ctx context.Context) *Scope {
	sc, _ := ctx.Value(scopeKey{}).(*Scope)
	return sc
}

// Statement counts one statement in the scope of ctx. It returns the shape of
// the statement, the count of that shape and whether the count just exceeded
// threshold for the first time. ok is false when ctx has no scope, when op is
// transaction control or EXPLAIN, or when the scope already tracks MaxKeys other
// shapes.
func Statement(ctx context.Context, op, sql string, threshold int) (shape string, n int, fire, ok bool) {
	sc := From(ctx)
	if sc == nil {
		return "", 0, false, false
	}
	switch op {
	case "begin", "commit", "rollback", "savepoint", "explain":
		return "", 0, false, false
	}
	shape = Shape(sql)
	sc.mu.Lock()
	n, fire = sc.shapes.add(shape, threshold)
	sc.mu.Unlock()
	return shape, n, fire, n > 0
}

// Relation counts one relation load, named by key, in the scope of ctx. It
// returns the count and whether the count just exceeded threshold for the
// first time. ok is false when ctx has no scope or the scope already tracks
// MaxKeys other relations.
func Relation(ctx context.Context, key string, threshold int) (n int, fire, ok bool) {
	sc := From(ctx)
	if sc == nil {
		return 0, false, false
	}
	sc.mu.Lock()
	n, fire = sc.relations.add(key, threshold)
	sc.mu.Unlock()
	return n, fire, n > 0
}

var (
	shapePGPlaceholder = regexp.MustCompile(`\$\d+`)
	shapeInList        = regexp.MustCompile(`\(\?(, \?)*\)`)
	shapeTupleList     = regexp.MustCompile(`\(\?\)(, \(\?\))+`)
)

// Shape collapses numbered placeholders to ? and any run of placeholders inside
// parentheses to one, so batched IN loads of different sizes share a shape
// while per-row statements repeat it.
func Shape(sql string) string {
	s := shapePGPlaceholder.ReplaceAllString(sql, "?")
	s = shapeInList.ReplaceAllString(s, "(?)")
	return shapeTupleList.ReplaceAllString(s, "(?)")
}
