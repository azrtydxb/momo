// SPDX-License-Identifier: Apache-2.0

package obs

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/azrtydxb/momo"
)

// SlowQuery describes one statement that took longer than the threshold of a
// slow-query hook. Plan holds the plain EXPLAIN output when it was captured;
// PlanSkipped says why it was not when a plan was requested with
// WithPlainExplain.
type SlowQuery struct {
	// SQL is the statement text as the driver received it.
	SQL string
	// Duration is the time the statement took, as the DB measured it.
	Duration time.Duration
	// Plan is the plain EXPLAIN output, one string per plan row; nil when no
	// plan was captured.
	Plan []string
	// PlanSkipped is the reason Plan is nil although WithPlainExplain was given.
	PlanSkipped string
}

// SlowOption configures NewSlowQuery.
type SlowOption func(*slowHook)

// WithPlainExplain turns on plan capture, running plain EXPLAIN, never EXPLAIN
// ANALYZE. A statement outside a transaction is explained through q. Pass a
// querier that does not carry the slow-query hook, for example a second
// momo.New on the same driver without it; the hook ignores the statements it
// issues itself either way. A statement inside a transaction is explained on
// that transaction.
func WithPlainExplain(q momo.Querier) SlowOption { return func(h *slowHook) { h.explain = q } }

type slowGuardKey struct{}

// slowExplainTimeout bounds one plan capture.
const slowExplainTimeout = 2 * time.Second

type slowHook struct {
	threshold time.Duration
	report    func(SlowQuery)
	explain   momo.Querier
}

var _ momo.Hook = (*slowHook)(nil)

// NewSlowQuery returns a hook that calls report for each statement that took
// longer than threshold. It is safe for concurrent use.
//
// With WithPlainExplain, report also gets the plan: one extra plain EXPLAIN
// (never ANALYZE) with the statement's own arguments, run on the goroutine
// that ran the slow statement, after it finished. Inside a transaction the
// EXPLAIN runs on that transaction in a savepoint that is always rolled back,
// so a failing capture never aborts the caller's transaction; a transaction
// that is already aborted is reported as "plan unavailable: transaction
// aborted". A statement with bound arguments is explained only when the DB was
// opened with momo.WithArgLogging, because the hook does not see the values
// otherwise; one that binds a sensitive value is never explained. The plan can
// echo literal values.
//
// report runs synchronously on the goroutine that ran the statement and must
// not query the database.
func NewSlowQuery(threshold time.Duration, report func(SlowQuery), opts ...SlowOption) momo.Hook {
	h := &slowHook{threshold: threshold, report: report}
	for _, o := range opts {
		o(h)
	}
	return h
}

// BeforeQuery implements momo.Hook.
func (h *slowHook) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context { return ctx }

// AfterQuery implements momo.Hook.
func (h *slowHook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
	if e.Duration <= h.threshold || ctx.Value(slowGuardKey{}) != nil {
		return
	}
	sq := SlowQuery{SQL: e.SQL, Duration: e.Duration}
	switch {
	case h.explain == nil:
	case e.NumArgs > 0 && e.Args == nil:
		sq.PlanSkipped = "statement has bound arguments and the DB was opened without WithArgLogging"
	case hasRedactedArg(e.Args):
		sq.PlanSkipped = "statement binds a sensitive argument"
	case !explainable(e.SQL):
		sq.PlanSkipped = "statement kind is not explainable"
	default:
		ectx, cancel := context.WithTimeout(context.WithValue(context.WithoutCancel(ctx), slowGuardKey{}, true), slowExplainTimeout)
		sq.Plan, sq.PlanSkipped = h.capture(ectx, e)
		cancel()
	}
	h.report(sq)
}

var errSlowRollback = errors.New("obs: slow query plan capture rollback")

// capture returns the plan of e, or the reason there is none.
func (h *slowHook) capture(ctx context.Context, e *momo.QueryEvent) ([]string, string) {
	c := momo.Compiled{SQL: e.SQL, Args: e.Args}
	if !e.InTx {
		plan, err := momo.Explain(ctx, h.explain, c)
		if err != nil {
			return nil, "EXPLAIN failed: " + err.Error()
		}
		return plan, ""
	}
	tx, ok := e.Querier.(*momo.Tx)
	if !ok {
		return nil, "plan unavailable: the transaction handle is not available to the hook"
	}
	var plan []string
	var explainErr error
	ran := false
	err := tx.Tx(ctx, func(inner *momo.Tx) error {
		ran = true
		plan, explainErr = momo.Explain(ctx, inner, c)
		if explainErr != nil {
			return explainErr
		}
		return errSlowRollback
	})
	switch {
	case !ran && errors.Is(err, momo.ErrUnsupported):
		return nil, "plan unavailable: the dialect has no savepoints"
	case !ran:
		return nil, "plan unavailable: transaction aborted"
	case explainErr != nil:
		return nil, "EXPLAIN failed: " + explainErr.Error()
	case err != nil && !errors.Is(err, errSlowRollback):
		return nil, "plan unavailable: " + err.Error()
	}
	return plan, ""
}

func hasRedactedArg(args []any) bool {
	for _, a := range args {
		if s, ok := a.(string); ok && s == momo.RedactedArg {
			return true
		}
	}
	return false
}

// explainable reports whether sql starts with a keyword the databases accept
// after EXPLAIN.
func explainable(sql string) bool {
	s := strings.TrimLeft(sql, " \t\r\n(")
	if i := strings.IndexAny(s, " \t\r\n("); i >= 0 {
		s = s[:i]
	}
	switch strings.ToUpper(s) {
	case "SELECT", "WITH", "INSERT", "UPDATE", "DELETE", "VALUES", "REPLACE", "MERGE":
		return true
	}
	return false
}
