// SPDX-License-Identifier: Apache-2.0

package obs

import (
	"context"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/nplusone"
)

// StatementFinding reports a statement shape that repeated more than the
// threshold within one scope. Count is the number of executions when the
// threshold was first exceeded.
type StatementFinding struct {
	// Shape is the SQL with placeholders and IN lists collapsed.
	Shape string
	// Example is the SQL text of the statement that crossed the threshold.
	Example string
	// Count is the number of executions when the threshold was first exceeded.
	Count int
}

// StatementNPlusOne is a development hook that finds any statement that runs
// again and again in one scope, such as a Select in a loop. It counts statement
// shapes in a scope created with WithScope and reports a StatementFinding once
// per shape and scope when the count exceeds the threshold. Transaction control
// and EXPLAIN statements are ignored.
//
// It differs from NPlusOne, which counts relation loads. They read the same
// scope, so one WithScope serves both. Create it with NewStatementNPlusOne; the
// zero value is not usable. It implements momo.Hook and is safe for concurrent
// use.
type StatementNPlusOne struct {
	threshold int
	report    func(StatementFinding)
}

var _ momo.Hook = (*StatementNPlusOne)(nil)

// NewStatementNPlusOne returns a detector that calls report once per statement
// shape per scope when the shape has run more than threshold times (a threshold
// below 1 counts as 1). Placeholders are collapsed, so batched IN loads of
// different sizes share one shape while per-row statements repeat it. A scope
// tracks at most 1024 distinct shapes; later new shapes in the same scope are
// not counted. report runs synchronously on the querying goroutine and must not
// query the database; a panic in report is recovered and dropped.
func NewStatementNPlusOne(threshold int, report func(StatementFinding)) *StatementNPlusOne {
	if threshold < 1 {
		threshold = 1
	}
	return &StatementNPlusOne{threshold: threshold, report: report}
}

// BeforeQuery counts the statement and reports when the threshold is exceeded.
func (s *StatementNPlusOne) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	shape, n, fire, ok := nplusone.Statement(ctx, e.Op, e.SQL, s.threshold)
	if ok && fire {
		s.safeReport(StatementFinding{Shape: shape, Example: e.SQL, Count: n})
	}
	return ctx
}

// AfterQuery does nothing.
func (s *StatementNPlusOne) AfterQuery(context.Context, *momo.QueryEvent) {}

// safeReport calls report and drops a panic, so a faulty reporter cannot break
// the query that triggered it.
func (s *StatementNPlusOne) safeReport(f StatementFinding) {
	defer func() { _ = recover() }()
	if s.report != nil {
		s.report(f)
	}
}
