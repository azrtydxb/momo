// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"regexp"
	"sync"
)

// NPlusOneFinding reports a statement shape that repeated more than the
// threshold within one query scope. Count is the number of executions when
// the threshold was first exceeded.
type NPlusOneFinding struct {
	Shape   string // SQL with placeholders and IN lists collapsed
	Example string // the SQL text of the statement that crossed the threshold
	Count   int
}

// maxScopeShapes bounds the memory one query scope can hold.
const maxScopeShapes = 1024

type queryScopeKey struct{}

type queryScope struct {
	mu       sync.Mutex
	counts   map[string]int
	reported map[string]bool
}

// WithQueryScope returns a context that groups statements for the N+1
// detector. Wrap one unit of work (an HTTP request, a job) and pass the
// context to every query in it. Contexts without a scope are not tracked.
func WithQueryScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, queryScopeKey{}, &queryScope{counts: map[string]int{}, reported: map[string]bool{}})
}

var (
	shapePGPlaceholder = regexp.MustCompile(`\$\d+`)
	shapeInList        = regexp.MustCompile(`\(\?(, \?)*\)`)
	shapeTupleList     = regexp.MustCompile(`\(\?\)(, \(\?\))+`)
)

// normalizeShape collapses numbered placeholders to ? and any run of
// placeholders inside parentheses to one, so batched IN loads of different
// sizes share a shape while per-row statements repeat it.
func normalizeShape(sql string) string {
	s := shapePGPlaceholder.ReplaceAllString(sql, "?")
	s = shapeInList.ReplaceAllString(s, "(?)")
	return shapeTupleList.ReplaceAllString(s, "(?)")
}

type nPlusOneHook struct {
	threshold int
	report    func(NPlusOneFinding)
}

// NewNPlusOneHook returns a development hook that calls report once per
// statement shape per scope when the shape has run more than threshold times
// (a threshold below 1 counts as 1). Transaction control and EXPLAIN
// statements are ignored. A scope tracks at most 1024 distinct shapes; later
// new shapes in the same scope are not counted. report runs synchronously on
// the querying goroutine and must not query the database; a panic in report
// is recovered and dropped.
func NewNPlusOneHook(threshold int, report func(NPlusOneFinding)) Hook {
	if threshold < 1 {
		threshold = 1
	}
	return &nPlusOneHook{threshold: threshold, report: report}
}

func (h *nPlusOneHook) BeforeQuery(ctx context.Context, e *QueryEvent) context.Context {
	sc, ok := ctx.Value(queryScopeKey{}).(*queryScope)
	if !ok {
		return ctx
	}
	switch e.Op {
	case "begin", "commit", "rollback", "savepoint", "explain":
		return ctx
	}
	shape := normalizeShape(e.SQL)
	sc.mu.Lock()
	if _, seen := sc.counts[shape]; !seen && len(sc.counts) >= maxScopeShapes {
		sc.mu.Unlock()
		return ctx
	}
	sc.counts[shape]++
	n := sc.counts[shape]
	fire := n > h.threshold && !sc.reported[shape]
	if fire {
		sc.reported[shape] = true
	}
	sc.mu.Unlock()
	if fire {
		h.safeReport(NPlusOneFinding{Shape: shape, Example: e.SQL, Count: n})
	}
	return ctx
}

func (h *nPlusOneHook) AfterQuery(context.Context, *QueryEvent) {}

// safeReport calls report and drops a panic, so a faulty reporter cannot
// break the query that triggered it.
func (h *nPlusOneHook) safeReport(f NPlusOneFinding) {
	defer func() { _ = recover() }()
	h.report(f)
}
