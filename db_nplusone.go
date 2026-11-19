// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"

	"github.com/azrtydxb/momo/internal/nplusone"
)

// NPlusOneFinding reports a statement shape that repeated more than the
// threshold within one query scope. Count is the number of executions when
// the threshold was first exceeded.
//
// Deprecated: use obs.StatementFinding.
type NPlusOneFinding struct {
	Shape   string // SQL with placeholders and IN lists collapsed
	Example string // the SQL text of the statement that crossed the threshold
	Count   int
}

// WithQueryScope returns a context that groups statements for the N+1
// detector. Wrap one unit of work (an HTTP request, a job) and pass the
// context to every query in it. Contexts without a scope are not tracked.
//
// Deprecated: use obs.WithScope. It returns the same scope, which serves
// obs.NewStatementNPlusOne and obs.NewNPlusOne together.
func WithQueryScope(ctx context.Context) context.Context {
	return nplusone.With(ctx)
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
//
// Deprecated: use obs.NewStatementNPlusOne, which counts the same way and
// reads the scope that obs.WithScope makes. This function and WithQueryScope
// stay for one release.
func NewNPlusOneHook(threshold int, report func(NPlusOneFinding)) Hook {
	if threshold < 1 {
		threshold = 1
	}
	return &nPlusOneHook{threshold: threshold, report: report}
}

func (h *nPlusOneHook) BeforeQuery(ctx context.Context, e *QueryEvent) context.Context {
	shape, n, fire, ok := nplusone.Statement(ctx, e.Op, e.SQL, h.threshold)
	if ok && fire {
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
