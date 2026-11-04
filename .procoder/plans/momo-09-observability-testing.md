# momo-09-observability-testing — implementation plan

Status: draft
Spec: .procoder/specs/momo.md

## Goal

Make Momo observable and testable: extend the hook event, add zero-cost-when-off hooks (N+1 detector, slow-query reporter, slog logger), explicit EXPLAIN helpers, the `momotest` application test helpers, the separate `otelmomo` module, and the budget, goroutine, import-graph and streaming-memory guards that keep the core honest.

## Architecture

The core hook contract stays in package `momo` (`db.go`): `QueryEvent` gains `PoolWait`, `InTx`, `NumArgs` and `Dialect`, and a per-statement slot in the context lets a driver report pool wait through `momo.RecordPoolWait`. Hooks that need only `QueryEvent` and `Querier` (N+1 detector, slow-query reporter) live in the root package; `momolog` and `momotest` are separate packages of the main module; `otelmomo` is a separate Go module with its own `go.mod` and a `replace` directive, so the core never links OpenTelemetry. Every opt-in behaviour is a value in the caller's context or a registered hook: no global state and no goroutines.

Build priority: PostgreSQL first (milestone M1). Every database test runs through `testopen.Each(t, fn, opts...)` (momo-03; the variadic options reach `momo.New`), so it runs on each dialect registered in this build as a subtest named after the dialect, never with a name suffix. Pool wait is a PostgreSQL driver feature; the core side is exercised on every dialect through a wrapper driver that calls `momo.RecordPoolWait` directly, and `ExplainAnalyze` asserts `ErrUnsupported` on dialects other than PostgreSQL.

Root package layout: new root files `explain.go`, `tenant.go`, `nplusone.go`, `slowquery.go`; new root test files all start with `obs_` and every top-level name in them starts with `Test` followed by a name not used by momo-06 (momo-06 owns `TestEagerLoad*`, `TestPreloadOnQuery`, `TestRelationPredicates`, `TestRelationTraverseAndOrder`, `TestAssociationHelpers`, `TestCreateGraphAndFindOrCreate`, `TestUpsertByPK`, `TestPipelinedEagerLoad`, `TestJoinLoadToOne`, `TestNestedFilteredEagerLoad`, `TestPolymorphicAndThrough`, `TestRelationHookEvents`, `TestEagerLoadReloadClearsStaleChildren` and the helpers `ids`, `docIDs`, `commentIDs`, `allDocs`). This plan adds one non-test support package, `internal/obstest`, holding the recording hook `obstest.Recorder` (it keeps the whole `QueryEvent`, which `relfixture.Hook` does not).

Contract changes this plan makes (reported to the lead; ARCHITECTURE.md is NOT edited here):

- `QueryEvent` gains `PoolWait time.Duration`, `InTx bool`, `NumArgs int`, `Dialect *dialect.Dialect` and `Querier Querier` (the `*DB` or `*Tx` the statement ran on; momo-01 owns the file, this plan edits `db.go`).
- `func RecordPoolWait(ctx context.Context, d time.Duration)` is a new root function. momo-03's `pgxdriver` must call it from its pgxpool `AcquireTracer`: `TraceAcquireStart(ctx, pool, data) context.Context` returns the ctx it received (with its own start time stored), and `TraceAcquireEnd(ctx, pool, data)` calls `momo.RecordPoolWait(ctx, elapsed)`. This plan tests only the core side with a direct call.
- New root files owned by this plan: `explain.go` (`Explain`, `ExplainAnalyze`), `tenant.go` (`WithTenant`, `TenantFilter`), `nplusone.go` (`WithQueryScope`, `NewNPlusOneHook`), `slowquery.go` (`NewSlowQueryHook`, `WithPlainExplain`).
- `TenantFilter` takes the context as first parameter because a predicate is built before execution and must read the tenant from somewhere.
- `Explain` uses `EXPLAIN FORMAT=JSON` on MySQL and MariaDB (a single text column) instead of plain `EXPLAIN`, whose column count differs between servers and cannot be scanned through the `Rows` interface.
- `docs/PERFORMANCE.md` is created by this plan (Task 3) with the hook-overhead budget section; momo-12 Task 11 extends it. The enforced figure is the constant `selectNoHooksAllocBudget`, kept equal to the documented row by `TestPerformanceDocRecordsBudget`.

Test rule for this plan: tests that touch a database live in `package momo_test`; pure helpers use `*_internal_test.go` in `package momo`. The database tests assume momo-03 (`testopen.Postgres`, `testopen.Each` with variadic options, `pgxdriver`, `sqlitedriver`) and momo-06 (`internal/relfixture`) have landed.

## Constraints

- [S-18] "Observability: optional hooks for duration, pool wait, rows, query counts, relation loading, errors and tracing; parameters excluded from logs by default; SQL inspection and explicit EXPLAIN / EXPLAIN ANALYZE helpers; optional tenant scope helper documented as not a security boundary; no hidden goroutines, implicit migrations or global mutable state."
- [S-34] "Testing helpers for applications (package momotest): per-test transactions rolled back on cleanup, SQL assertion helpers using compile-without-execute, fixture loading, and ephemeral SQLite databases."
- [S-35] "OpenTelemetry integration as a separate Go module (otelmomo) implementing the hook interface with spans and metrics following OTel database semantic conventions; the core module gains no OTel dependency."
- [S-53] "N+1 detection hook for development: flags repeated statement shapes within one context beyond a threshold."
- [S-54] "Slow-query hook: threshold-based reporting with opt-in plain EXPLAIN capture; never EXPLAIN ANALYZE automatically."
- [S-55] "Index-usage assertions: a momotest helper that fails when a query's plan does not use the named index."
- [S-56] "Bounded-memory streaming: Each/Iter keep memory flat independent of result size."
- [S-87] "A ready-made slog debug logger hook with levels and slow threshold."
- Acceptance [S-18]: "`BenchmarkSelectNoHooks` allocations stay within the budget recorded in docs/PERFORMANCE.md; `TestHooks` asserts SQL, duration, rows and errors are reported and args only with WithArgLogging; `TestExplainAnalyzeExplicit` asserts plain Explain never executes the query; fails if disabled hooks allocate."
- Acceptance [S-35]: "`TestOtelSpans` (otelmomo module) asserts span names, `db.system`, `db.query.text` without parameter values, and error status via an in-memory exporter; `go list -deps` on the core module shows no OpenTelemetry package; fails if the core imports OTel."
- Generator and migration tooling dependencies (golang.org/x/tools etc.) stay out of the application runtime import graph; benchmark comparison ORMs live in a separate Go module.
- Security: identifiers always quoted, values always bound; raw SQL is an explicit escape hatch with a documented contract; parameter values not logged unless explicitly enabled.
- No hidden background goroutines, no implicit migrations at connect time, no unexplained global mutable state.
- Go: minimum 1.26 in go.mod; CI tests 1.26 and 1.27. Dependencies pinned in go.mod/go.sum.

Chosen interpretations (the spec is silent or inconsistent):

- S-35 acceptance names the attribute `db.system`; the current OpenTelemetry database conventions (semconv v1.43.0) call it `db.system.name`, and `db.system` is obsolete. `TestOtelSpans` asserts `db.system.name` and asserts that no `db.system` attribute is emitted.
- S-18 lists "tracing" as a hook concern; the core hook interface has no span API, so tracing lives only in `otelmomo`, which starts and ends spans from `BeforeQuery` and `AfterQuery`.
- S-54 does not say what plain EXPLAIN capture does inside a transaction or for parameterized statements. Plan capture runs plain EXPLAIN (never ANALYZE) with the statement's own bound arguments on the querier the statement ran on (`QueryEvent.Querier`); inside a transaction it runs in a savepoint that is always rolled back, so a failing capture never aborts the caller's transaction, and when the transaction is already aborted the report says `plan unavailable: transaction aborted`. Capture is skipped (reason in `SlowQuery.PlanSkipped`) when the statement has bound arguments while the DB was not opened with `WithArgLogging` (the hook has no values to bind).
- S-56 gives no memory bound; the named constant `streamMaxGrowB` is 16 MiB, documented in `docs/PERFORMANCE.md` and kept equal to it by a test.
- S-42 (sensitive columns) is owned by momo-05 Task 28: hook events and `Compiled.String()` carry `momo.Redacted` instead of the value of a `FlagSensitive` column. `TestHooks` and `TestMomotestSQLAssert` assert that; `momotest.AssertSQL` prints only the SQL, the argument count and `Compiled.String()`, never values.

## Interfaces

New and changed names, with the owning task:

```go
// db.go (Task 1)
type QueryEvent struct {
	Op, Table, Relation, SQL string
	Args       []any
	NumArgs    int                // always set; len of bound parameters
	InTx       bool               // statement ran on a *Tx
	PoolWait   time.Duration      // summed RecordPoolWait for this statement
	Dialect    *dialect.Dialect
	Querier    Querier            // the *DB or *Tx the statement ran on
	Start      time.Time
	Duration   time.Duration
	Rows       int64
	Err        error
	pool       *poolWaitSlot
}
func RecordPoolWait(ctx context.Context, d time.Duration)

// internal/obstest (Task 1, support code for the tests)
type Event struct{ momo.QueryEvent; CtxSQL string }
type Recorder struct{ /* mutex, events */ }          // implements momo.Hook
func (r *Recorder) Events() []Event
func (r *Recorder) SQLs() []string
func (r *Recorder) Reset()

// explain.go (Task 5)
func Explain(ctx context.Context, q Querier, c Compiled) ([]string, error)
func ExplainAnalyze(ctx context.Context, q Querier, c Compiled) ([]string, error)

// tenant.go (Task 6)
func WithTenant(ctx context.Context, tenant any) context.Context
func TenantFilter[T any](ctx context.Context, col Column[T]) Predicate

// nplusone.go (Task 12)
type NPlusOneFinding struct{ Shape, Example string; Count int }
func WithQueryScope(ctx context.Context) context.Context
func NewNPlusOneHook(threshold int, report func(NPlusOneFinding)) Hook

// slowquery.go (Task 13)
type SlowQuery struct{ SQL string; Duration time.Duration; Plan []string; PlanSkipped string }
type SlowOption func(*slowHook)
func WithPlainExplain(q Querier) SlowOption
func NewSlowQueryHook(threshold time.Duration, report func(SlowQuery), opts ...SlowOption) Hook

// momotest (Tasks 8-10)
func Tx(tb testing.TB, db *momo.DB) *momo.Tx
func AssertSQL(tb testing.TB, got momo.Compiled, want string)
func LoadFixtures(tb testing.TB, q momo.Querier, path string)
func SQLite(tb testing.TB, opts ...momo.Option) *momo.DB
func AssertUsesIndex(tb testing.TB, q momo.Querier, c momo.Compiled, index string)

// momolog (Task 11)
func New(l *slog.Logger, opts ...Option) momo.Hook
func Level(l slog.Level) Option
func SlowOver(d time.Duration) Option
func Values(on bool) Option

// otelmomo, separate module (Tasks 15-16)
func New(tp trace.TracerProvider, mp metric.MeterProvider, opts ...Option) (momo.Hook, error)
func WithNamespace(ns string) Option
```

Names consumed from other plans, with the exact shape this plan relies on:

```go
// momo-03 internal/testdb/testopen
func Postgres(t *testing.T, opts ...momo.Option) *momo.DB
func Each(t *testing.T, fn func(t *testing.T, db *momo.DB, d dialect.Name), opts ...momo.Option)
// momo-06 internal/relfixture
func Each(t *testing.T, fn func(t *testing.T, db *momo.DB, h *relfixture.Hook))
type Hook struct{ After []string; Events []momo.RelationEvent } // After holds the SQL of each finished statement
func CreateSchema(t testing.TB, db *momo.DB)
func SeedBasic(t testing.TB, db *momo.DB)
func PostgresDriver(t testing.TB) momo.Driver
```

`relfixture.Each` is assumed to open a fresh database per registered dialect with the fixture schema applied and `h` registered as a hook; tests call `relfixture.SeedBasic` themselves. Also consumed: `momo.Redacted`, `momo.NewColumn`, `momo.FlagSensitive` (momo-05 Task 28 and momo-01), `momo.Hook`, `momo.WithHook`, `momo.WithArgLogging`, `momo.Querier`, `momo.Compiled`, `momo.Exec` (with `?` placeholders on every dialect), `momo.Ident`, `momo.SQL`, `momo.Select`, `momo.Insert` with `Values` and `SQL() (Compiled, error)` (momo-05), `momo.Load`, `momo.RelationEvent` (momo-06), `sqlitedriver.Open`, `testdb.SQLitePath` (only in `momotest.SQLite`).

## Task 1: QueryEvent pool wait, transaction flag, argument count and dialect

Files:

- Modify `db.go` (`QueryEvent`, `execer`, `before`, `after`, `Begin`)
- Create `internal/obstest/obstest.go` (the `Recorder` hook, exercised by every test below)
- Create `obs_helpers_test.go` (package `momo_test`)
- Create `obs_poolwait_test.go` (package `momo_test`)

Interfaces:

```go
func RecordPoolWait(ctx context.Context, d time.Duration)
type QueryEvent struct { /* fields as in Interfaces above */ }
type execer struct { /* existing fields */ inTx bool; owner Querier }
```

Covers: S-18 (pool wait, query counts). Acceptance tests named here: `TestQueryEventPoolWait`, `TestRecordPoolWaitWithoutHooks`, `TestQueryEventInTx`.

- [ ] **Step 1: Write the support package** — create `internal/obstest/obstest.go`:

```go
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
```

- [ ] **Step 2: Write the test helpers** — create `obs_helpers_test.go`:

```go
package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

// obsSeed applies the fixture schema and seeds the basic rows.
func obsSeed(t testing.TB, db *momo.DB) {
	t.Helper()
	relfixture.CreateSchema(t, db)
	relfixture.SeedBasic(t, db)
}

func obsDocs(t testing.TB, q momo.Querier) []models.Document {
	t.Helper()
	docs, err := momo.Select[models.Document, *models.Document](q).OrderBy(document.ID.Asc()).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return docs
}

func obsAuthorCount(t testing.TB, q momo.Querier, id int64) int64 {
	t.Helper()
	n, err := momo.Select[models.Author, *models.Author](q).Where(author.ID.Eq(id)).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}
```

- [ ] **Step 3: Write the failing tests** — create `obs_poolwait_test.go`:

```go
package momo_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// waitDriver reports a fixed pool wait twice per statement, the way a pool
// tracer that observes two acquisitions would.
type waitDriver struct {
	momo.Driver
	wait time.Duration
}

func (d waitDriver) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	momo.RecordPoolWait(ctx, d.wait)
	momo.RecordPoolWait(ctx, d.wait)
	return d.Driver.Query(ctx, sql, args)
}

func (d waitDriver) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	momo.RecordPoolWait(ctx, d.wait)
	momo.RecordPoolWait(ctx, d.wait)
	return d.Driver.Exec(ctx, sql, args)
}

func TestQueryEventPoolWait(t *testing.T) {
	ctx := context.Background()
	rec := &obstest.Recorder{}
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		rec.Reset()
		wdb := momo.New(waitDriver{Driver: db.Driver(), wait: 5 * time.Millisecond}, momo.WithHook(rec))
		if _, err := momo.Exec(ctx, wdb, `CREATE TABLE pw (x INTEGER)`); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, wdb, `INSERT INTO pw (x) VALUES (?)`, 7); err != nil {
			t.Fatal(err)
		}
		ev := rec.Events()
		if len(ev) != 2 {
			t.Fatalf("want 2 events, got %d", len(ev))
		}
		for i, e := range ev {
			if e.PoolWait != 10*time.Millisecond {
				t.Fatalf("event %d PoolWait = %v, want 10ms (two recorded waits are summed)", i, e.PoolWait)
			}
			if e.InTx {
				t.Fatalf("event %d InTx = true outside a transaction", i)
			}
		}
		if ev[1].NumArgs != 1 || ev[1].Args != nil {
			t.Fatalf("NumArgs = %d Args = %v, want 1 and nil without WithArgLogging", ev[1].NumArgs, ev[1].Args)
		}
		if ev[0].Dialect == nil || ev[0].Dialect.Name != d {
			t.Fatalf("event Dialect = %+v, want %s", ev[0].Dialect, d)
		}
	})
}

func TestRecordPoolWaitWithoutHooks(t *testing.T) {
	ctx := context.Background()
	momo.RecordPoolWait(ctx, time.Hour) // no slot in the context: must be a no-op
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		wdb := momo.New(waitDriver{Driver: db.Driver(), wait: time.Hour})
		if _, err := momo.Exec(ctx, wdb, `CREATE TABLE pw (x INTEGER)`); err != nil {
			t.Fatal(err)
		}
	})
}

func TestQueryEventInTx(t *testing.T) {
	ctx := context.Background()
	rec := &obstest.Recorder{}
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if _, err := momo.Exec(ctx, db, `CREATE TABLE pw (x INTEGER)`); err != nil {
			t.Fatal(err)
		}
		rec.Reset()
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			_, err := momo.Exec(ctx, tx, `INSERT INTO pw (x) VALUES (?)`, 1)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		var ops []string
		var inTx []bool
		var owners []string
		for _, e := range rec.Events() {
			ops = append(ops, e.Op)
			inTx = append(inTx, e.InTx)
			owners = append(owners, fmt.Sprintf("%T", e.Querier))
		}
		wantOps := []string{"begin", "exec", "commit"}
		wantTx := []bool{false, true, true}
		wantOwners := []string{"*momo.DB", "*momo.Tx", "*momo.Tx"}
		for i := range wantOps {
			if i >= len(ops) || ops[i] != wantOps[i] || inTx[i] != wantTx[i] || owners[i] != wantOwners[i] {
				t.Fatalf("ops %v inTx %v owners %v, want %v %v %v", ops, inTx, owners, wantOps, wantTx, wantOwners)
			}
		}
	}, momo.WithHook(rec))
}
```

- [ ] **Step 4: Run and expect FAIL** — `go test ./ -run 'TestQueryEventPoolWait|TestRecordPoolWaitWithoutHooks|TestQueryEventInTx' -count=1` fails to compile with `undefined: momo.RecordPoolWait` (also `e.PoolWait undefined (type obstest.Event has no field or method PoolWait)`).

- [ ] **Step 5: Implement in `db.go`** — replace the `QueryEvent` struct with the version in Interfaces (keep the existing doc comments on each field) and add:

```go
type poolWaitKey struct{}

// poolWaitSlot accumulates the pool wait reported for one statement.
type poolWaitSlot struct{ ns atomic.Int64 }

// RecordPoolWait adds d to the pool wait of the statement whose context is
// ctx. Drivers call it when a connection acquisition waited (pgxdriver does
// so from its pgxpool AcquireTracer). It is a no-op when ctx does not belong
// to a hooked statement, so it costs nothing when no hook is registered.
func RecordPoolWait(ctx context.Context, d time.Duration) {
	if s, ok := ctx.Value(poolWaitKey{}).(*poolWaitSlot); ok {
		s.ns.Add(int64(d))
	}
}
```

Add `inTx bool` and `owner Querier` to `execer`. In `New` set `owner: db` on `db.ex`. In `Begin`, build the transaction execer with `inTx: true` and `owner: tx`:

```go
tx.ex = execer{db: db, q: dtx, d: db.ex.d, classify: db.ex.classify, inTx: true, owner: tx}
```

Change `before` so that nothing is allocated when there are no hooks (the early return stays first):

```go
func (e *execer) before(ctx context.Context, op, table, rel, sql string, args []any) (context.Context, *QueryEvent) {
	if len(e.db.hooks) == 0 {
		return ctx, nil
	}
	slot := &poolWaitSlot{}
	ctx = context.WithValue(ctx, poolWaitKey{}, slot)
	ev := &QueryEvent{
		Op: op, Table: table, Relation: rel, SQL: sql,
		NumArgs: len(args), InTx: e.inTx, Dialect: e.d, Querier: e.owner,
		Start: time.Now(), Rows: -1, pool: slot,
	}
	if e.db.logArgs {
		ev.Args = args
	}
	for _, h := range e.db.hooks {
		ctx = h.BeforeQuery(ctx, ev)
	}
	return ctx, ev
}
```

and in `after`, before calling the hooks: `ev.PoolWait = time.Duration(ev.pool.ns.Load())`.

- [ ] **Step 6: Run and expect PASS** — `go test ./ -run 'TestQueryEventPoolWait|TestRecordPoolWaitWithoutHooks|TestQueryEventInTx' -count=1` prints `ok`.

- [ ] **Step 7: Commit** — `git add db.go internal/obstest obs_helpers_test.go obs_poolwait_test.go && git commit -m "feat(hooks): record pool wait and transaction flag on query events"`.

## Task 2: TestHooks, the hook contract

Files:

- Create `obs_hooks_test.go` (package `momo_test`)
- Modify `db.go` only in the mutation steps (reverted)

Interfaces:

```go
func WithHook(h Hook) Option
func WithArgLogging() Option
```

Covers: S-18 (duration, rows, errors, args only with `WithArgLogging`, parameters excluded by default). Acceptance test named here: `TestHooks`.

The spike already reports these fields, so the test passes at once; steps 2 and 3 are mutations that prove the test catches a regression.

- [ ] **Step 1: Write the test** — create `obs_hooks_test.go`:

```go
package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestHooks(t *testing.T) {
	ctx := context.Background()

	t.Run("select reports sql duration rows and no args", func(t *testing.T) {
		rec := &obstest.Recorder{}
		testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
			obsSeed(t, db)
			rec.Reset()
			docs := obsDocs(t, db)
			ev := rec.Events()
			if len(ev) != 1 {
				t.Fatalf("want 1 event, got %d", len(ev))
			}
			e := ev[0]
			if e.Op != "select" || e.Table != "documents" || !strings.Contains(e.SQL, `"documents"`) {
				t.Fatalf("op/table/sql wrong: %+v", e.QueryEvent)
			}
			if e.Rows != int64(len(docs)) {
				t.Fatalf("Rows = %d, want %d", e.Rows, len(docs))
			}
			if e.Duration <= 0 {
				t.Fatalf("Duration = %v, want > 0", e.Duration)
			}
			if e.Err != nil || e.Args != nil {
				t.Fatalf("Err = %v Args = %v, want nil and nil", e.Err, e.Args)
			}
			if e.CtxSQL != e.SQL {
				t.Fatalf("context from BeforeQuery did not reach AfterQuery: %q", e.CtxSQL)
			}
		}, momo.WithHook(rec))
	})

	t.Run("args are excluded without WithArgLogging", func(t *testing.T) {
		rec := &obstest.Recorder{}
		testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
			obsSeed(t, db)
			rec.Reset()
			if _, err := momo.Select[models.Author, *models.Author](db).Where(author.Name.Eq("Ada")).All(ctx); err != nil {
				t.Fatal(err)
			}
			e := rec.Events()[0]
			if e.Args != nil || e.NumArgs != 1 {
				t.Fatalf("Args = %v NumArgs = %d, want nil and 1", e.Args, e.NumArgs)
			}
		}, momo.WithHook(rec))
	})

	t.Run("args are included with WithArgLogging", func(t *testing.T) {
		rec := &obstest.Recorder{}
		testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
			obsSeed(t, db)
			rec.Reset()
			if _, err := momo.Select[models.Author, *models.Author](db).Where(author.Name.Eq("Ada")).All(ctx); err != nil {
				t.Fatal(err)
			}
			e := rec.Events()[0]
			if len(e.Args) != 1 || e.Args[0] != "Ada" {
				t.Fatalf("Args = %v, want [Ada]", e.Args)
			}
		}, momo.WithHook(rec), momo.WithArgLogging())
	})

	t.Run("sensitive columns show the placeholder, not the value", func(t *testing.T) {
		secret := momo.NewColumn[string]("authors", "name", momo.FlagSensitive)
		rec := &obstest.Recorder{}
		testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
			obsSeed(t, db)
			rec.Reset()
			if _, err := momo.Select[models.Author, *models.Author](db).Where(secret.Eq("hunter2")).All(ctx); err != nil {
				t.Fatal(err)
			}
			e := rec.Events()[0]
			if len(e.Args) != 1 || e.Args[0] != momo.Redacted {
				t.Fatalf("Args = %v, want [momo.Redacted] for a FlagSensitive column", e.Args)
			}
		}, momo.WithHook(rec), momo.WithArgLogging())
	})

	t.Run("exec reports rows affected", func(t *testing.T) {
		rec := &obstest.Recorder{}
		testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
			obsSeed(t, db)
			rec.Reset()
			if _, err := momo.Exec(ctx, db, `INSERT INTO authors (id, name) VALUES (?, ?)`, int64(50), "X"); err != nil {
				t.Fatal(err)
			}
			e := rec.Events()[0]
			if e.Op != "exec" || e.Rows != 1 {
				t.Fatalf("Op = %q Rows = %d, want exec and 1", e.Op, e.Rows)
			}
		}, momo.WithHook(rec))
	})

	t.Run("errors are reported", func(t *testing.T) {
		rec := &obstest.Recorder{}
		testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
			obsSeed(t, db)
			rec.Reset()
			_, err := momo.Exec(ctx, db, `SELECT * FROM no_such_table`)
			if err == nil {
				t.Fatal("want an error for a missing table")
			}
			e := rec.Events()[0]
			if e.Err == nil || !errors.Is(err, e.Err) {
				t.Fatalf("event Err = %v, returned error = %v", e.Err, err)
			}
			if e.Rows != -1 {
				t.Fatalf("Rows = %d, want -1 for a failed statement", e.Rows)
			}
		}, momo.WithHook(rec))
	})
}
```

- [ ] **Step 2: Run and expect PASS** — `go test ./ -run '^TestHooks$' -count=1` prints `ok`.

- [ ] **Step 3: Mutation, argument leak** — in `db.go` `before`, change `if e.db.logArgs {` to `if true {`. Run `go test ./ -run '^TestHooks$' -count=1`; expect FAIL with `Args = [Ada] NumArgs = 1, want nil and 1`. Restore the line. Then, in momo-05's redaction in `before`, skip the `Redacted` substitution; expect FAIL with `want [momo.Redacted] for a FlagSensitive column`. Restore it.

- [ ] **Step 4: Mutation, row count** — in `hookedRows.Next` remove the `r.n++` line. Run the same command; expect FAIL with `Rows = 0, want 4`. Restore the line.

- [ ] **Step 5: Run and expect PASS** — `go test ./ -run '^TestHooks$' -count=1` prints `ok`.

- [ ] **Step 6: Commit** — `git add obs_hooks_test.go && git commit -m "test(hooks): assert hook events report sql, duration, rows, errors and gated args"`.

## Task 3: Disabled-hook allocation budget and docs/PERFORMANCE.md

Files:

- Create `obs_hooks_bench_test.go` (package `momo_test`, `BenchmarkSelectNoHooks`)
- Create `obs_budget_test.go` (package `momo_test`, `TestSelectNoHooksAllocBudget`, `TestPerformanceDocRecordsBudget`)
- Create `docs/PERFORMANCE.md` (hook-overhead budget section; momo-12 Task 11 extends it)

Interfaces:

```go
const selectNoHooksAllocBudget = 0 // replaced in Step 5 by the measured integer
func BenchmarkSelectNoHooks(b *testing.B)
```

Covers: S-18. Acceptance tests named here: `BenchmarkSelectNoHooks`, `TestSelectNoHooksAllocBudget`, `TestPerformanceDocRecordsBudget`. The budget is measured on PostgreSQL (milestone M1).

- [ ] **Step 1: Write the benchmark** — create `obs_hooks_bench_test.go`:

```go
package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

// BenchmarkSelectNoHooks measures a primary-key lookup on a DB that has no
// hooks registered. Its allocs/op is the budget enforced by
// TestSelectNoHooksAllocBudget.
func BenchmarkSelectNoHooks(b *testing.B) {
	db := momo.New(relfixture.PostgresDriver(b))
	relfixture.CreateSchema(b, db)
	relfixture.SeedBasic(b, db)
	ctx := context.Background()
	q := momo.Select[models.Author, *models.Author](db).Where(author.ID.Eq(1))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := q.One(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
```

- [ ] **Step 2: Write the failing budget test** — create `obs_budget_test.go`:

```go
package momo_test

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// selectNoHooksAllocBudget is allocs/op of BenchmarkSelectNoHooks measured on
// PostgreSQL on the commit before hooks gained PoolWait/InTx.
const selectNoHooksAllocBudget = 0

func TestSelectNoHooksAllocBudget(t *testing.T) {
	db := testopen.Postgres(t)
	relfixture.CreateSchema(t, db)
	relfixture.SeedBasic(t, db)
	ctx := context.Background()
	q := momo.Select[models.Author, *models.Author](db).Where(author.ID.Eq(1))
	allocs := testing.AllocsPerRun(200, func() {
		if _, err := q.One(ctx); err != nil {
			t.Fatal(err)
		}
	})
	if allocs > selectNoHooksAllocBudget {
		t.Fatalf("allocs/op %v exceeds budget %d: disabled hooks must add no allocation", allocs, selectNoHooksAllocBudget)
	}
}

func TestPerformanceDocRecordsBudget(t *testing.T) {
	raw, err := os.ReadFile("docs/PERFORMANCE.md")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^\| BenchmarkSelectNoHooks allocs/op \| (\d+) \|$`).FindSubmatch(raw)
	if m == nil {
		t.Fatal("docs/PERFORMANCE.md has no row '| BenchmarkSelectNoHooks allocs/op | N |'")
	}
	got, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatal(err)
	}
	if got != selectNoHooksAllocBudget {
		t.Fatalf("docs/PERFORMANCE.md records %d, test constant is %d", got, selectNoHooksAllocBudget)
	}
}
```

- [ ] **Step 3: Run and expect FAIL** — with the PostgreSQL service up (`make db-up`, `MOMO_TEST_POSTGRES_URL` set), `go test ./ -run '^TestSelectNoHooksAllocBudget$' -count=1` fails with `exceeds budget 0: disabled hooks must add no allocation`; `go test ./ -run '^TestPerformanceDocRecordsBudget$' -count=1` fails with `open docs/PERFORMANCE.md: no such file or directory`.

- [ ] **Step 4: Measure the budget on the baseline** — the baseline is the commit before Task 1. Run:

```sh
SHA=$(git log -n1 --format=%H --grep='^feat(hooks): record pool wait')
BASE=$(git rev-parse "$SHA~1")
git worktree add --detach ../momo-baseline "$BASE"
cp obs_hooks_bench_test.go ../momo-baseline/
(cd ../momo-baseline && go test ./ -run '^$' -bench '^BenchmarkSelectNoHooks$' -benchmem -count=3)
git worktree remove --force ../momo-baseline
go test ./ -run '^$' -bench '^BenchmarkSelectNoHooks$' -benchmem -count=3
```

Read the largest `allocs/op` figure of each run. Both runs must print the same figure N. If the second run prints more, Task 1 added an allocation to the no-hook path: move it behind the `len(e.db.hooks) == 0` early return in `before`, then rerun.

- [ ] **Step 5: Record the budget** — in `obs_budget_test.go` replace `= 0` with `= N` (the integer from Step 4; for example a printed `37 allocs/op` gives `= 37`). Create `docs/PERFORMANCE.md`:

```markdown
# Performance

Budgets are measured, never aspirational. This page starts with the hook
overhead budget that `TestSelectNoHooksAllocBudget` enforces; the benchmark
suite extends it with methodology, versions and regression policy.

## Hook overhead budget

| Benchmark                        | Budget |
| -------------------------------- | ------ |
| BenchmarkSelectNoHooks allocs/op | N      |
```

Write the same integer N in the row; the second column holds the bare integer so `TestPerformanceDocRecordsBudget` can parse it. Add a Performance entry to the page index in `docs/README.md`, copying the row format of the neighbouring entries, then run `go test ./internal/repocheck -count=1` and expect `ok`.

- [ ] **Step 6: Run and expect PASS** — `go test ./ -run 'TestSelectNoHooksAllocBudget|TestPerformanceDocRecordsBudget' -count=1` prints `ok`.

- [ ] **Step 7: Mutation, allocation on the disabled path** — in `db.go` add `var allocSink *QueryEvent` at package level and, as the first line of `before`, `allocSink = &QueryEvent{}`. Run `go test ./ -run '^TestSelectNoHooksAllocBudget$' -count=1`; expect FAIL with `exceeds budget`. Remove both lines.

- [ ] **Step 8: Commit** — `git add obs_hooks_bench_test.go obs_budget_test.go docs && git commit -m "test(hooks): gate disabled-hook allocations against a measured budget"`.

## Task 4: No hidden goroutines and no OpenTelemetry in the core

Files:

- Create `obs_isolation_test.go` (package `momo_test`)

Interfaces:

```go
func TestNoHiddenGoroutines(t *testing.T)
func TestCoreHasNoOTel(t *testing.T)
func otelImports(deps []string) []string // test helper
```

Covers: S-18 (no hidden goroutines), S-35 (core gains no OTel dependency). Acceptance tests named here: `TestNoHiddenGoroutines`, `TestCoreHasNoOTel`. The spike already satisfies both, so the tests pass at once and steps 2, 4 and 5 are mutations proving each guard can fail.

- [ ] **Step 1: Write the guards** — create `obs_isolation_test.go`:

```go
package momo_test

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// otelImports returns the OpenTelemetry packages among deps.
func otelImports(deps []string) []string {
	var out []string
	for _, d := range deps {
		if strings.Contains(d, "go.opentelemetry.io") {
			out = append(out, d)
		}
	}
	return out
}

func TestOtelImportsDetector(t *testing.T) {
	got := otelImports([]string{"fmt", "go.opentelemetry.io/otel/trace", "github.com/azrtydxb/momo/dialect"})
	if len(got) != 1 || got[0] != "go.opentelemetry.io/otel/trace" {
		t.Fatalf("otelImports = %v, want the trace package only", got)
	}
}

func TestCoreHasNoOTel(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/azrtydxb/momo").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	if bad := otelImports(strings.Fields(string(out))); len(bad) > 0 {
		t.Fatalf("core imports OpenTelemetry: %v", bad)
	}
}

func TestNoHiddenGoroutines(t *testing.T) {
	ctx := context.Background()
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		// Warm the pool so connection set-up goroutines of the driver
		// exist before the baseline; Momo itself must add none.
		if _, err := momo.Exec(ctx, db, `SELECT 1`); err != nil {
			t.Fatal(err)
		}
		runtime.GC()
		before := runtime.NumGoroutine()

		hdb := momo.New(db.Driver(), momo.WithHook(&obstest.Recorder{}))
		obsSeed(t, hdb)
		for i := 0; i < 200; i++ {
			obsDocs(t, hdb)
		}
		if err := hdb.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			_, err := momo.Exec(ctx, tx, `INSERT INTO authors (id, name) VALUES (?, ?)`, int64(70), "G")
			return err
		}); err != nil {
			t.Fatal(err)
		}

		deadline := time.Now().Add(2 * time.Second)
		for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if after := runtime.NumGoroutine(); after > before {
			t.Fatalf("goroutines before=%d after=%d: Momo must start none", before, after)
		}
	})
}
```

- [ ] **Step 2: Run the detector test against a broken helper** — temporarily change `strings.Contains(d, "go.opentelemetry.io")` in `otelImports` to `strings.Contains(d, "go.opentelemetry.ioX")` and run `go test ./ -run '^TestOtelImportsDetector$' -count=1`; expect FAIL with `otelImports = [], want the trace package only`. Restore the correct string.

- [ ] **Step 3: Run and expect PASS** — `go test ./ -run 'TestOtelImportsDetector|TestCoreHasNoOTel|TestNoHiddenGoroutines' -count=1` prints `ok`.

- [ ] **Step 4: Mutation, goroutine** — in `New` (db.go) add `go func() { select {} }()` as the first line. Run `go test ./ -run '^TestNoHiddenGoroutines$' -count=1`; expect FAIL with `Momo must start none`. Remove the line.

- [ ] **Step 5: Mutation, OpenTelemetry import** — run:

```sh
go get go.opentelemetry.io/otel@v1.47.0
printf 'package momo\n\nimport _ "go.opentelemetry.io/otel"\n' > zz_otel_mutation.go
go test ./ -run '^TestCoreHasNoOTel$' -count=1
rm zz_otel_mutation.go
git checkout -- go.mod go.sum
```

Expect the test run to FAIL with `core imports OpenTelemetry`. After the checkout, `git status --short go.mod go.sum` prints nothing.

- [ ] **Step 6: Commit** — `git add obs_isolation_test.go && git commit -m "test(core): guard against hidden goroutines and OpenTelemetry imports"`.

## Task 5: Explicit EXPLAIN and EXPLAIN ANALYZE helpers

Files:

- Create `explain.go`
- Create `obs_explain_test.go` (package `momo_test`)

Interfaces:

```go
func Explain(ctx context.Context, q Querier, c Compiled) ([]string, error)
func ExplainAnalyze(ctx context.Context, q Querier, c Compiled) ([]string, error)
```

Covers: S-18 (SQL inspection and explicit EXPLAIN / EXPLAIN ANALYZE helpers; never automatic). Acceptance test named here: `TestExplainAnalyzeExplicit`. On PostgreSQL it asserts timings and rollback; on other dialects it asserts `ErrUnsupported` and that nothing was issued.

- [ ] **Step 1: Write the failing test** — create `obs_explain_test.go`:

```go
package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestExplainAnalyzeExplicit(t *testing.T) {
	ctx := context.Background()
	rec := &obstest.Recorder{}
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		obsSeed(t, db)
		rec.Reset()

		// Select, Count and Exec paths issue no EXPLAIN text.
		sel := momo.Select[models.Author, *models.Author](db).Where(author.ID.Eq(1))
		if _, err := sel.All(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := sel.Count(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, db, `UPDATE authors SET name = ? WHERE id = ?`, "Ada", int64(1)); err != nil {
			t.Fatal(err)
		}
		for _, s := range rec.SQLs() {
			if strings.Contains(strings.ToUpper(s), "EXPLAIN") {
				t.Fatalf("ordinary path issued %q", s)
			}
		}

		// Explain of a write issues one EXPLAIN statement and does not run the write.
		ins, err := momo.Insert[models.Author, *models.Author](db).Values(author.ID.Set(99), author.Name.Set("ghost")).SQL()
		if err != nil {
			t.Fatal(err)
		}
		prefix := "EXPLAIN "
		if d == dialect.SQLite {
			prefix = "EXPLAIN QUERY PLAN "
		}
		start := len(rec.SQLs())
		if _, err := momo.Explain(ctx, db, ins); err != nil {
			t.Fatal(err)
		}
		stmts := rec.SQLs()[start:]
		if len(stmts) != 1 || stmts[0] != prefix+ins.SQL {
			t.Fatalf("Explain issued %q, want exactly %q", stmts, prefix+ins.SQL)
		}
		if n := obsAuthorCount(t, db, 99); n != 0 {
			t.Fatalf("Explain executed the INSERT: %d rows", n)
		}

		// Explain of a read returns plan lines that mention the table.
		c, err := sel.SQL()
		if err != nil {
			t.Fatal(err)
		}
		lines, err := momo.Explain(ctx, db, c)
		if err != nil {
			t.Fatal(err)
		}
		if len(lines) == 0 || !strings.Contains(strings.Join(lines, "\n"), "authors") {
			t.Fatalf("plan lines %q do not mention the table", lines)
		}

		// ExplainAnalyze.
		start = len(rec.SQLs())
		if d != dialect.Postgres {
			if _, err := momo.ExplainAnalyze(ctx, db, c); !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("ExplainAnalyze on %s: err = %v, want ErrUnsupported", d, err)
			}
			if got := len(rec.SQLs()) - start; got != 0 {
				t.Fatalf("unsupported ExplainAnalyze issued %d statements", got)
			}
			return
		}
		lines, err = momo.ExplainAnalyze(ctx, db, ins)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(lines, "\n"), "actual time") {
			t.Fatalf("ANALYZE plan has no timings: %q", lines)
		}
		if n := obsAuthorCount(t, db, 99); n != 0 {
			t.Fatalf("ExplainAnalyze left the INSERT committed: %d rows", n)
		}
		lines, err = momo.ExplainAnalyze(ctx, db, c)
		if err != nil {
			t.Fatal(err)
		}
		if len(lines) == 0 {
			t.Fatal("ExplainAnalyze of a SELECT returned no plan")
		}
	}, momo.WithHook(rec))
}
```

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run '^TestExplainAnalyzeExplicit$' -count=1` fails to compile with `undefined: momo.Explain` and `undefined: momo.ExplainAnalyze`.

- [ ] **Step 3: Implement `explain.go`**:

```go
package momo

import (
	"context"
	"errors"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

var errExplainRollback = errors.New("momo: explain rollback")

// Explain returns the database's plan for c without executing it:
// EXPLAIN on PostgreSQL, EXPLAIN FORMAT=JSON on MySQL and MariaDB (one text
// column) and EXPLAIN QUERY PLAN on SQLite. Hooks observe the statement with
// Op "explain". Momo never calls Explain on its own.
func Explain(ctx context.Context, q Querier, c Compiled) ([]string, error) {
	prefix := "EXPLAIN "
	switch q.Dialect().Name {
	case dialect.SQLite:
		prefix = "EXPLAIN QUERY PLAN "
	case dialect.MySQL:
		prefix = "EXPLAIN FORMAT=JSON "
	}
	return planLines(ctx, q, prefix+c.SQL, c.Args)
}

// ExplainAnalyze runs c under EXPLAIN (ANALYZE, FORMAT TEXT) and returns the
// plan with actual timings. ANALYZE executes the statement: a SELECT runs
// as is; anything else runs inside a transaction (or savepoint) that is
// always rolled back. PostgreSQL only; other dialects return ErrUnsupported.
// Momo never calls ExplainAnalyze on its own.
func ExplainAnalyze(ctx context.Context, q Querier, c Compiled) ([]string, error) {
	if q.Dialect().Name != dialect.Postgres {
		return nil, &Error{Kind: ErrUnsupported, Op: "explain analyze"}
	}
	sql := "EXPLAIN (ANALYZE, FORMAT TEXT) " + c.SQL
	if isReadStatement(c.SQL) {
		return planLines(ctx, q, sql, c.Args)
	}
	var lines []string
	run := func(tx *Tx) error {
		var err error
		if lines, err = planLines(ctx, tx, sql, c.Args); err != nil {
			return err
		}
		return errExplainRollback
	}
	var err error
	switch v := q.(type) {
	case *DB:
		err = v.Tx(ctx, TxOptions{}, run)
	case *Tx:
		err = v.Tx(ctx, run)
	default:
		return nil, &Error{Kind: ErrUnsupported, Op: "explain analyze"}
	}
	if errors.Is(err, errExplainRollback) {
		err = nil
	}
	return lines, err
}

// isReadStatement reports whether the first keyword is SELECT or VALUES.
// WITH is treated as a write because a CTE may contain data-modifying
// statements.
func isReadStatement(sql string) bool {
	k := firstKeyword(sql)
	return k == "SELECT" || k == "VALUES"
}

func firstKeyword(sql string) string {
	s := strings.TrimLeft(sql, " \t\r\n(")
	if i := strings.IndexAny(s, " \t\r\n("); i >= 0 {
		s = s[:i]
	}
	return strings.ToUpper(s)
}

func planLines(ctx context.Context, q Querier, sql string, args []any) ([]string, error) {
	r, err := q.conn().query(ctx, "explain", "", "", Compiled{SQL: sql, Args: args})
	if err != nil {
		return nil, err
	}
	defer r.Close()
	sqlite := q.Dialect().Name == dialect.SQLite
	var lines []string
	for r.Next() {
		var line string
		if sqlite {
			var id, parent, unused int64
			err = r.Scan(&id, &parent, &unused, &line)
		} else {
			err = r.Scan(&line)
		}
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return lines, r.Close()
}
```

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestExplainAnalyzeExplicit$' -count=1` prints `ok`.

- [ ] **Step 5: Mutation, Explain executes** — in `Explain` replace the initial `prefix := "EXPLAIN "` with `prefix := ""` and delete the SQLite case. Run the test; expect FAIL with `Explain issued ["INSERT INTO`. Restore.

- [ ] **Step 6: Commit** — `git add explain.go obs_explain_test.go && git commit -m "feat(observability): add explicit Explain and ExplainAnalyze helpers"`.

## Task 6: Tenant scope helper

Files:

- Create `tenant.go`
- Create `obs_tenant_test.go` (package `momo_test`)

Interfaces:

```go
func WithTenant(ctx context.Context, tenant any) context.Context
func TenantFilter[T any](ctx context.Context, col Column[T]) Predicate
```

Covers: S-18 (optional tenant scope helper documented as not a security boundary). Acceptance test named here: `TestTenantFilter`.

- [ ] **Step 1: Write the failing test** — create `obs_tenant_test.go`:

```go
package momo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/store"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestTenantFilter(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		obsSeed(t, db)
		stores := func(ctx context.Context) momo.SelectQuery[models.Store] {
			return momo.Select[models.Store, *models.Store](db).Where(momo.TenantFilter(ctx, store.TenantID))
		}

		t.Run("scoped to the tenant in the context", func(t *testing.T) {
			ctx := momo.WithTenant(context.Background(), int64(1))
			c, err := stores(ctx).SQL()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(c.SQL, `"stores"."tenant_id" =`) || len(c.Args) != 1 || c.Args[0] != int64(1) {
				t.Fatalf("SQL %q args %v", c.SQL, c.Args)
			}
			rows, err := stores(ctx).All(ctx)
			if err != nil || len(rows) != 3 {
				t.Fatalf("tenant 1 stores = %d err %v, want 3", len(rows), err)
			}
		})

		t.Run("fails closed without a tenant or with the wrong type", func(t *testing.T) {
			for name, ctx := range map[string]context.Context{
				"absent":     context.Background(),
				"wrong type": momo.WithTenant(context.Background(), 1), // int, column is int64
				"nil":        momo.WithTenant(context.Background(), nil),
			} {
				c, err := stores(ctx).SQL()
				if err != nil || !strings.Contains(c.SQL, "1 = 0") {
					t.Fatalf("%s: SQL %q err %v, want 1 = 0", name, c.SQL, err)
				}
				rows, err := stores(ctx).All(ctx)
				if err != nil || len(rows) != 0 {
					t.Fatalf("%s: %d rows err %v, want none", name, len(rows), err)
				}
			}
		})

		t.Run("is not a security boundary", func(t *testing.T) {
			ctx := momo.WithTenant(context.Background(), int64(1))
			all, err := momo.Select[models.Store, *models.Store](db).All(ctx)
			if err != nil || len(all) != 4 {
				t.Fatalf("unfiltered query returned %d rows err %v, want all 4 tenants' stores", len(all), err)
			}
			n, err := momo.Select[models.Store, *models.Store](db).Where(momo.SQL("1 = 1")).Count(ctx)
			if err != nil || n != 4 {
				t.Fatalf("raw predicate count %d err %v, want 4", n, err)
			}
		})
	})
}
```

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run '^TestTenantFilter$' -count=1` fails with `undefined: momo.TenantFilter` and `undefined: momo.WithTenant`.

- [ ] **Step 3: Implement `tenant.go`**:

```go
package momo

import "context"

type tenantKey struct{}

// WithTenant returns a context carrying tenant for TenantFilter.
func WithTenant(ctx context.Context, tenant any) context.Context {
	return context.WithValue(ctx, tenantKey{}, tenant)
}

// TenantFilter returns col = <tenant from ctx>. When ctx has no tenant, or
// the tenant's dynamic type is not exactly T, it returns the always-false
// predicate 1 = 0, so a missing tenant never widens a query.
//
// TenantFilter is a convenience, NOT a security boundary: a query that omits
// it, raw SQL, or another connection sees every tenant's rows. Enforce
// isolation in the database with row-level security (S-38) and use this only
// to avoid repeating the predicate.
func TenantFilter[T any](ctx context.Context, col Column[T]) Predicate {
	if v, ok := ctx.Value(tenantKey{}).(T); ok {
		return col.Eq(v)
	}
	return SQL("1 = 0")
}
```

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestTenantFilter$' -count=1` prints `ok`.

- [ ] **Step 5: Commit** — `git add tenant.go obs_tenant_test.go && git commit -m "feat(observability): add fail-closed tenant filter helper"`.

## Task 7: Relation loading is reported to hooks

Files:

- Create `obs_relevents_test.go` (package `momo_test`)
- Modify `db.go` only in the mutation step (reverted); momo-06 owns the relation hook plumbing

Interfaces:

```go
type RelationEvent struct { Parent, Name string; Kind RelationKind; Parents int; Children int64; Queries int; Duration time.Duration; Err error }
```

(defined by momo-06 Task 5; this task does not redefine it, and uses `relfixture.Hook` and `relfixture.Each` from momo-06)

Covers: S-18 (relation loading, query counts). Acceptance test named here: `TestRelationEventsReported`. It passes on first run because momo-06 emits the events; step 3 is the mutation that proves it bites.

- [ ] **Step 1: Write the test** — create `obs_relevents_test.go`:

```go
package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
)

func TestRelationEventsReported(t *testing.T) {
	ctx := context.Background()
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := obsDocs(t, db)
		h.After = nil
		h.Events = nil

		if err := momo.Load(ctx, db, docs, document.Author); err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(ctx, db, docs, document.Comments); err != nil {
			t.Fatal(err)
		}
		if len(h.Events) != 2 {
			t.Fatalf("want 2 relation events, got %d", len(h.Events))
		}
		a, c := h.Events[0], h.Events[1]
		if a.Parent != "documents" || a.Name != "Author" || a.Kind != momo.BelongsTo || a.Parents != 4 || a.Children != 2 || a.Queries != 1 {
			t.Fatalf("Author event %+v", a)
		}
		if c.Parent != "documents" || c.Name != "Comments" || c.Kind != momo.HasMany || c.Parents != 4 || c.Children != 4 || c.Queries != 1 {
			t.Fatalf("Comments event %+v", c)
		}
		if a.Duration <= 0 {
			t.Fatalf("Duration = %v, want > 0", a.Duration)
		}
		if len(h.After) != 2 {
			t.Fatalf("want 2 child statements, got %d: %q", len(h.After), h.After)
		}
	})
}
```

- [ ] **Step 2: Run and expect PASS** — `go test ./ -run '^TestRelationEventsReported$' -count=1` prints `ok`.

- [ ] **Step 3: Mutation, relation hooks never registered** — in `db.go` `WithHook`, comment out the lines that append to `db.relHooks`. Run the same command; expect FAIL with `want 2 relation events, got 0`. Restore the lines.

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestRelationEventsReported$' -count=1` prints `ok`.

- [ ] **Step 5: Commit** — `git add obs_relevents_test.go && git commit -m "test(hooks): assert relation load events and child statements"`.

## Task 8: momotest per-test transactions and SQL assertions

Files:

- Create `momotest/doc.go`
- Create `momotest/momotest.go` (`Tx`, `AssertSQL`)
- Create `momotest/rectb_test.go` and `momotest/momotest_test.go` (package `momotest_test`)

Interfaces:

```go
func Tx(tb testing.TB, db *momo.DB) *momo.Tx
func AssertSQL(tb testing.TB, got momo.Compiled, want string)
```

Covers: S-34 (per-test transactions, compile-without-execute SQL assertions). Acceptance tests named here: `TestMomotestTxRollback`, `TestMomotestSQLAssert`.

- [ ] **Step 1: Write the failing tests** — create `momotest/rectb_test.go`:

```go
package momotest_test

import (
	"fmt"
	"testing"
)

// recTB records failures instead of failing the real test.
type recTB struct {
	testing.TB
	errs    []string
	helpers int
}

func (r *recTB) Helper() { r.helpers++ }

func (r *recTB) Errorf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

func (r *recTB) Fatalf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
	panic("recTB: Fatalf: " + r.errs[len(r.errs)-1])
}
```

Create `momotest/momotest_test.go`:

```go
package momotest_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/momotest"
)

func authorCount(t testing.TB, q momo.Querier, id int64) int64 {
	t.Helper()
	n, err := momo.Select[models.Author, *models.Author](q).Where(author.ID.Eq(id)).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMomotestTxRollback(t *testing.T) {
	ctx := context.Background()
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		relfixture.CreateSchema(t, db)
		relfixture.SeedBasic(t, db)

		t.Run("inner", func(t *testing.T) {
			tx := momotest.Tx(t, db)
			if _, err := momo.Exec(ctx, tx, `INSERT INTO authors (id, name) VALUES (?, ?)`, int64(99), "temp"); err != nil {
				t.Fatal(err)
			}
			if n := authorCount(t, tx, 99); n != 1 {
				t.Fatalf("row invisible inside its own transaction: %d", n)
			}
		})
		if n := authorCount(t, db, 99); n != 0 {
			t.Fatalf("row written in momotest.Tx survived cleanup (count %d)", n)
		}

		ok := t.Run("explicit commit then cleanup stays quiet", func(t *testing.T) {
			tx := momotest.Tx(t, db)
			if _, err := momo.Exec(ctx, tx, `INSERT INTO authors (id, name) VALUES (?, ?)`, int64(98), "kept"); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		})
		if !ok {
			t.Fatal("cleanup reported ErrTxDone after an explicit Commit")
		}
		if n := authorCount(t, db, 98); n != 1 {
			t.Fatalf("committed row missing: %d", n)
		}
	})
}

func TestMomotestSQLAssert(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		c, err := momo.Select[models.Author, *models.Author](db).Where(author.Name.Eq("hunter2")).SQL()
		if err != nil {
			t.Fatal(err)
		}
		before := len(h.After)

		good := &recTB{TB: t}
		momotest.AssertSQL(good, c, strings.ReplaceAll(c.SQL, " FROM ", "\n\tFROM "))
		if len(good.errs) != 0 {
			t.Fatalf("whitespace-only difference failed: %v", good.errs)
		}
		if good.helpers == 0 {
			t.Fatal("AssertSQL did not call tb.Helper")
		}

		bad := &recTB{TB: t}
		momotest.AssertSQL(bad, c, c.SQL+" LIMIT 1")
		if len(bad.errs) != 1 {
			t.Fatalf("mismatch reported %d errors, want 1", len(bad.errs))
		}
		msg := bad.errs[0]
		if !strings.Contains(msg, "SQL mismatch") || !strings.Contains(msg, c.SQL) || !strings.Contains(msg, "args: 1 bound") {
			t.Fatalf("message lacks SQL or argument count: %s", msg)
		}
		if strings.Contains(msg, "hunter2") {
			t.Fatalf("message leaks an argument value: %s", msg)
		}
		if len(h.After) != before {
			t.Fatalf("AssertSQL path executed %d statements; it must only compile", len(h.After)-before)
		}

		secret := momo.NewColumn[string]("authors", "name", momo.FlagSensitive)
		sc, serr := momo.Select[models.Author, *models.Author](db).Where(secret.Eq("hunter2")).SQL()
		if serr != nil {
			t.Fatal(serr)
		}
		sbad := &recTB{TB: t}
		momotest.AssertSQL(sbad, sc, sc.SQL+" LIMIT 1")
		if len(sbad.errs) != 1 || !strings.Contains(sbad.errs[0], fmt.Sprint(momo.Redacted)) || strings.Contains(sbad.errs[0], "hunter2") {
			t.Fatalf("sensitive dump must show %v and never the value: %v", momo.Redacted, sbad.errs)
		}
	})
}
```

- [ ] **Step 2: Run and expect FAIL** — `go test ./momotest/ -run 'TestMomotestTxRollback|TestMomotestSQLAssert' -count=1` fails with `no required module provides package github.com/azrtydxb/momo/momotest` (or `undefined: momotest.Tx` once `doc.go` exists).

- [ ] **Step 3: Implement** — create `momotest/doc.go` (`// Package momotest provides test helpers for applications built on Momo.` then `package momotest`) and `momotest/momotest.go`:

```go
package momotest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
)

// Tx begins a transaction on db and rolls it back when the test ends, so a
// test never leaves rows behind. A Commit by the test is allowed: the
// cleanup then sees momo.ErrTxDone and stays quiet.
func Tx(tb testing.TB, db *momo.DB) *momo.Tx {
	tb.Helper()
	tx, err := db.Begin(context.Background(), momo.TxOptions{})
	if err != nil {
		tb.Fatalf("momotest.Tx: begin: %v", err)
	}
	tb.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, momo.ErrTxDone) {
			tb.Errorf("momotest.Tx: rollback: %v", err)
		}
	})
	return tx
}

// AssertSQL compares a compiled statement with want, ignoring differences in
// whitespace. Build got with a query's SQL method: nothing is executed. On
// mismatch it prints both SQL texts, the number of bound arguments and
// got.String(), which never shows argument values (sensitive columns show
// momo.Redacted, momo-05 Task 28).
func AssertSQL(tb testing.TB, got momo.Compiled, want string) {
	tb.Helper()
	g, w := normalizeSQL(got.SQL), normalizeSQL(want)
	if g != w {
		tb.Errorf("momotest.AssertSQL: SQL mismatch\n got: %s\nwant: %s\nargs: %d bound\ndump: %s", g, w, len(got.Args), got.String())
	}
}

func normalizeSQL(s string) string { return strings.Join(strings.Fields(s), " ") }
```

- [ ] **Step 4: Run and expect PASS** — `go test ./momotest/ -run 'TestMomotestTxRollback|TestMomotestSQLAssert' -count=1` prints `ok`.

- [ ] **Step 5: Mutation, cleanup commits** — in `Tx` replace `tx.Rollback(` with `tx.Commit(`. Run the same command; expect FAIL with `row written in momotest.Tx survived cleanup (count 1)`. Restore.

- [ ] **Step 6: Mutation, leaked arguments** — in `AssertSQL` change the message tail to `"args: %v", got.Args`. Run; expect FAIL with `message leaks an argument value`. Restore.

- [ ] **Step 7: Commit** — `git add momotest && git commit -m "feat(momotest): add per-test transactions and SQL assertions"`.

## Task 9: momotest fixtures and ephemeral SQLite

Files:

- Create `momotest/fixtures.go` (`LoadFixtures`)
- Create `momotest/sqlite.go` (`SQLite`)
- Create `momotest/testdata/fixtures.yaml`, `momotest/testdata/bad.yaml`
- Create `momotest/fixtures_test.go` (package `momotest_test`)
- Modify `go.mod`, `go.sum` (`go.yaml.in/yaml/v3`)

Interfaces:

```go
func LoadFixtures(tb testing.TB, q momo.Querier, path string)
func SQLite(tb testing.TB, opts ...momo.Option) *momo.DB
```

Covers: S-34 (fixture loading, ephemeral SQLite databases). Acceptance tests named here: `TestMomotestFixtures`, `TestMomotestSQLiteEphemeral`.

- [ ] **Step 1: Create the fixture files** — `momotest/testdata/fixtures.yaml`:

```yaml
tags:
  - id: 1
    name: go
authors:
  - id: 1
    name: Ada
documents:
  - id: 1
    author_id: 1
    title: A1
  - id: 2
    author_id: null
    title: Orphan
```

`momotest/testdata/bad.yaml`:

```yaml
- 1
- 2
```

- [ ] **Step 2: Write the failing tests** — create `momotest/fixtures_test.go`:

```go
package momotest_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/momotest"
)

func TestMomotestFixtures(t *testing.T) {
	ctx := context.Background()
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		h.After = nil

		momotest.LoadFixtures(t, db, "testdata/fixtures.yaml")

		var ins []string
		for _, s := range h.After {
			if strings.HasPrefix(s, "INSERT INTO") {
				ins = append(ins, s)
			}
		}
		if len(ins) != 4 {
			t.Fatalf("want 4 inserts, got %d: %q", len(ins), ins)
		}
		for i, table := range []string{`"tags"`, `"authors"`, `"documents"`, `"documents"`} {
			if !strings.Contains(ins[i], "INSERT INTO "+table) {
				t.Fatalf("insert %d = %q, want table %s (file order)", i, ins[i], table)
			}
		}
		if !strings.Contains(ins[2], `("author_id", "id", "title")`) {
			t.Fatalf("columns are not sorted and quoted: %q", ins[2])
		}
		docs, err := momo.Select[models.Document, *models.Document](db).OrderBy(document.ID.Asc()).All(ctx)
		if err != nil || len(docs) != 2 {
			t.Fatalf("documents = %d err %v", len(docs), err)
		}
		if docs[0].AuthorID == nil || *docs[0].AuthorID != 1 || docs[1].AuthorID != nil {
			t.Fatalf("author ids = %v, %v; want 1 and NULL", docs[0].AuthorID, docs[1].AuthorID)
		}

		rt := &recTB{TB: t}
		func() {
			defer func() { recover() }()
			momotest.LoadFixtures(rt, db, "testdata/bad.yaml")
		}()
		if len(rt.errs) != 1 || !strings.Contains(rt.errs[0], "top level") {
			t.Fatalf("bad file errors = %v, want one mentioning the top level", rt.errs)
		}
	})
}

func TestMomotestSQLiteEphemeral(t *testing.T) {
	ctx := context.Background()
	var file string
	t.Run("inner", func(t *testing.T) {
		db := momotest.SQLite(t)
		if _, err := momo.Exec(ctx, db, `CREATE TABLE only_here (x INTEGER)`); err != nil {
			t.Fatal(err)
		}
		rows, err := db.Driver().Query(ctx, `PRAGMA database_list`, nil)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var seq int64
			var name, f string
			if err := rows.Scan(&seq, &name, &f); err != nil {
				t.Fatal(err)
			}
			if name == "main" {
				file = f
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		other := momotest.SQLite(t)
		if _, err := momo.Exec(ctx, other, `SELECT x FROM only_here`); err == nil {
			t.Fatal("second SQLite database sees the first one's table")
		}
	})
	if file == "" {
		t.Fatal("database file path not found")
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("database file %s still exists after the test (err %v)", file, err)
	}
}
```

- [ ] **Step 3: Run and expect FAIL** — `go test ./momotest/ -run 'TestMomotestFixtures|TestMomotestSQLiteEphemeral' -count=1` fails with `undefined: momotest.LoadFixtures` and `undefined: momotest.SQLite`.

- [ ] **Step 4: Implement** — run `go get go.yaml.in/yaml/v3@v3.0.5`, then create `momotest/sqlite.go`:

```go
package momotest

import (
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/sqlitedriver"
)

// SQLite returns a momo.DB on a new SQLite file inside tb.TempDir(). The
// database is closed and its file removed when the test ends. Options go to
// momo.New, for example momo.WithHook.
func SQLite(tb testing.TB, opts ...momo.Option) *momo.DB {
	tb.Helper()
	sqldb, drv, err := sqlitedriver.Open(filepath.Join(tb.TempDir(), "momotest.db"), sqlitedriver.Config{})
	if err != nil {
		tb.Fatalf("momotest.SQLite: %v", err)
	}
	tb.Cleanup(func() {
		if err := sqldb.Close(); err != nil {
			tb.Errorf("momotest.SQLite: close: %v", err)
		}
	})
	return momo.New(drv, opts...)
}
```

and `momotest/fixtures.go`:

```go
package momotest

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/azrtydxb/momo"
)

// LoadFixtures inserts the rows of a YAML file into q. The file is a mapping
// of table name to a list of rows (column to value); tables are inserted in
// file order and each row in list order, so foreign keys can be satisfied by
// ordering. Columns are written sorted. A YAML null becomes NULL. Table and
// column names are quoted with momo.Ident; values are bound.
func LoadFixtures(tb testing.TB, q momo.Querier, path string) {
	tb.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("momotest.LoadFixtures: %v", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		tb.Fatalf("momotest.LoadFixtures %s: %v", path, err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		tb.Fatalf("momotest.LoadFixtures %s: top level must be a mapping of table name to rows", path)
	}
	top := doc.Content[0]
	for i := 0; i+1 < len(top.Content); i += 2 {
		table, rows := top.Content[i].Value, top.Content[i+1]
		if rows.Kind != yaml.SequenceNode {
			tb.Fatalf("momotest.LoadFixtures %s: table %s must be a list of rows", path, table)
		}
		for n, rn := range rows.Content {
			var row map[string]any
			if err := rn.Decode(&row); err != nil {
				tb.Fatalf("momotest.LoadFixtures %s: table %s row %d: %v", path, table, n+1, err)
			}
			sql, args := insertStatement(table, row)
			if _, err := momo.Exec(context.Background(), q, sql, args...); err != nil {
				tb.Fatalf("momotest.LoadFixtures %s: table %s row %d: %v", path, table, n+1, err)
			}
		}
	}
}

func insertStatement(table string, row map[string]any) (string, []any) {
	cols := make([]string, 0, len(row))
	for c := range row {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	args := []any{momo.Ident(table)}
	for _, c := range cols {
		args = append(args, momo.Ident(c))
	}
	for _, c := range cols {
		args = append(args, row[c])
	}
	ph := strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ")
	return "INSERT INTO ? (" + ph + ") VALUES (" + ph + ")", args
}
```

- [ ] **Step 5: Run and expect PASS** — `go test ./momotest/ -run 'TestMomotestFixtures|TestMomotestSQLiteEphemeral' -count=1` prints `ok`.

- [ ] **Step 6: Mutation, unordered tables** — in `LoadFixtures` replace the loop over `top.Content` by decoding into `map[string][]map[string]any` and ranging over the map. Run the fixtures test with `-count=5`; expect FAIL with `(file order)` within five runs (map iteration is random). Restore.

- [ ] **Step 7: Commit** — `git add momotest go.mod go.sum && git commit -m "feat(momotest): add YAML fixtures and ephemeral SQLite databases"`.

## Task 10: Index-usage assertion

Files:

- Create `momotest/index.go` (`AssertUsesIndex`, `planUsesIndex`)
- Create `momotest/index_internal_test.go` (package `momotest`)
- Create `momotest/index_test.go` (package `momotest_test`)

Interfaces:

```go
func AssertUsesIndex(tb testing.TB, q momo.Querier, c momo.Compiled, index string)
func planUsesIndex(d dialect.Name, lines []string, index string) (found, seq bool)
```

Covers: S-55. Acceptance test named here: `TestAssertUsesIndex` (plus `TestPlanUsesIndex`).

- [ ] **Step 1: Write the failing plan-parser test** — create `momotest/index_internal_test.go`:

```go
package momotest

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestPlanUsesIndex(t *testing.T) {
	cases := []struct {
		name       string
		d          dialect.Name
		lines      []string
		index      string
		found, seq bool
	}{
		{"pg index scan", dialect.Postgres, []string{"Index Scan using docs_title_idx on docs  (cost=0.15..8.17 rows=1 width=4)", "  Index Cond: (title = $1)"}, "docs_title_idx", true, false},
		{"pg index only", dialect.Postgres, []string{"Index Only Scan using docs_title_idx on docs"}, "docs_title_idx", true, false},
		{"pg backward", dialect.Postgres, []string{"Index Scan Backward using docs_title_idx on docs"}, "docs_title_idx", true, false},
		{"pg bitmap", dialect.Postgres, []string{"Bitmap Heap Scan on docs", "  ->  Bitmap Index Scan on docs_title_idx"}, "docs_title_idx", true, false},
		{"pg seq scan", dialect.Postgres, []string{"Seq Scan on docs  (cost=0.00..1.04 rows=1 width=4)", "  Filter: (title = $1)"}, "docs_title_idx", false, true},
		{"pg longer name is not a match", dialect.Postgres, []string{"Index Scan using docs_title_idx2 on docs"}, "docs_title_idx", false, false},
		{"sqlite index", dialect.SQLite, []string{"SEARCH items USING INDEX items_sku_idx (sku=?)"}, "items_sku_idx", true, false},
		{"sqlite covering", dialect.SQLite, []string{"SEARCH items USING COVERING INDEX items_sku_idx (sku=?)"}, "items_sku_idx", true, false},
		{"sqlite scan", dialect.SQLite, []string{"SCAN items"}, "items_sku_idx", false, true},
		{"sqlite longer name is not a match", dialect.SQLite, []string{"SEARCH items USING INDEX items_sku_idx2 (sku=?)"}, "items_sku_idx", false, false},
	}
	for _, c := range cases {
		found, seq := planUsesIndex(c.d, c.lines, c.index)
		if found != c.found || seq != c.seq {
			t.Errorf("%s: found=%v seq=%v, want found=%v seq=%v", c.name, found, seq, c.found, c.seq)
		}
	}
}
```

- [ ] **Step 2: Write the failing acceptance test** — create `momotest/index_test.go`:

```go
package momotest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/momotest"
)

func TestAssertUsesIndex(t *testing.T) {
	ctx := context.Background()
	db := momotest.SQLite(t)
	for _, ddl := range []string{
		`CREATE TABLE items (id INTEGER PRIMARY KEY, sku TEXT NOT NULL, qty INTEGER NOT NULL)`,
		`CREATE INDEX items_sku_idx ON items (sku)`,
	} {
		if _, err := momo.Exec(ctx, db, ddl); err != nil {
			t.Fatal(err)
		}
	}
	indexed := momo.Compiled{SQL: `SELECT id FROM items WHERE sku = ?`, Args: []any{"a"}}
	scan := momo.Compiled{SQL: `SELECT id FROM items WHERE qty = ?`, Args: []any{1}}

	t.Run("indexed lookup passes", func(t *testing.T) {
		rt := &recTB{TB: t}
		momotest.AssertUsesIndex(rt, db, indexed, "items_sku_idx")
		if len(rt.errs) != 0 {
			t.Fatalf("unexpected failure: %v", rt.errs)
		}
	})
	t.Run("sequential scan fails", func(t *testing.T) {
		rt := &recTB{TB: t}
		momotest.AssertUsesIndex(rt, db, scan, "items_sku_idx")
		if len(rt.errs) != 1 || !strings.Contains(rt.errs[0], "sequential scan") {
			t.Fatalf("errors = %v, want one mentioning a sequential scan", rt.errs)
		}
	})
	t.Run("a different index fails", func(t *testing.T) {
		rt := &recTB{TB: t}
		momotest.AssertUsesIndex(rt, db, indexed, "items_other_idx")
		if len(rt.errs) != 1 || !strings.Contains(rt.errs[0], `"items_other_idx"`) {
			t.Fatalf("errors = %v, want one naming the missing index", rt.errs)
		}
	})
}
```

- [ ] **Step 3: Run and expect FAIL** — `go test ./momotest/ -run 'TestPlanUsesIndex|TestAssertUsesIndex' -count=1` fails with `undefined: planUsesIndex` and `undefined: momotest.AssertUsesIndex`.

- [ ] **Step 4: Implement `momotest/index.go`**:

```go
package momotest

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// AssertUsesIndex fails the test unless the plan of c uses the named index.
// It runs momo.Explain (plain EXPLAIN, never ANALYZE), so c is not executed.
// Supported on PostgreSQL and SQLite. The plan is printed on failure; the
// database may echo literal values into it, so use non-sensitive test data.
func AssertUsesIndex(tb testing.TB, q momo.Querier, c momo.Compiled, index string) {
	tb.Helper()
	d := q.Dialect().Name
	if d != dialect.Postgres && d != dialect.SQLite {
		tb.Fatalf("momotest.AssertUsesIndex: unsupported dialect %q", d)
	}
	lines, err := momo.Explain(context.Background(), q, c)
	if err != nil {
		tb.Fatalf("momotest.AssertUsesIndex: explain: %v", err)
	}
	found, seq := planUsesIndex(d, lines, index)
	if found {
		return
	}
	plan := strings.Join(lines, "\n")
	if seq {
		tb.Errorf("momotest.AssertUsesIndex: plan contains a sequential scan and does not use index %q\nplan:\n%s", index, plan)
		return
	}
	tb.Errorf("momotest.AssertUsesIndex: index %q is not used\nplan:\n%s", index, plan)
}

// planUsesIndex scans plan lines. found: the index name appears as a whole
// token in an index access node. seq: some node reads a table sequentially.
func planUsesIndex(d dialect.Name, lines []string, index string) (found, seq bool) {
	name := regexp.QuoteMeta(index)
	var use *regexp.Regexp
	switch d {
	case dialect.Postgres:
		use = regexp.MustCompile(`Index (Only )?Scan( Backward)? using ` + name + `\b|Bitmap Index Scan on ` + name + `\b`)
	default:
		use = regexp.MustCompile(`USING (COVERING )?INDEX ` + name + `\b`)
	}
	for _, l := range lines {
		if use.MatchString(l) {
			found = true
		}
		switch d {
		case dialect.Postgres:
			if strings.Contains(l, "Seq Scan") {
				seq = true
			}
		default:
			if strings.Contains(l, "SCAN ") && !strings.Contains(l, " USING ") {
				seq = true
			}
		}
	}
	return found, seq
}
```

- [ ] **Step 5: Run and expect PASS** — `go test ./momotest/ -run 'TestPlanUsesIndex|TestAssertUsesIndex' -count=1` prints `ok`.

- [ ] **Step 6: Mutation, helper ignores the plan** — at the top of `planUsesIndex` insert `return true, false`. Run the same command; expect FAIL with `sequential scan fails` and `a different index fails`. Remove the line.

- [ ] **Step 7: Commit** — `git add momotest/index.go momotest/index_internal_test.go momotest/index_test.go && git commit -m "feat(momotest): add AssertUsesIndex based on plain EXPLAIN"`.

## Task 11: momolog, the slog hook

Files:

- Create `momolog/momolog.go`
- Create `momolog/momolog_test.go` (package `momolog_test`)

Interfaces:

```go
func New(l *slog.Logger, opts ...Option) momo.Hook
type Option func(*hook)
func Level(l slog.Level) Option
func SlowOver(d time.Duration) Option
func Values(on bool) Option
```

Covers: S-87. Acceptance test named here: `TestSlogHook`.

- [ ] **Step 1: Write the failing test** — create `momolog/momolog_test.go`:

```go
package momolog_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/momolog"
)

// capture runs one parameterized select for a value that matches nothing, on
// each dialect, and passes the log text of that select to verify.
func capture(t *testing.T, hookOpts []momolog.Option, dbOpts []momo.Option, verify func(t *testing.T, out string)) {
	t.Helper()
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	opts := append([]momo.Option{momo.WithHook(momolog.New(l, hookOpts...))}, dbOpts...)
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		relfixture.CreateSchema(t, db)
		buf.Reset()
		if _, err := momo.Select[models.Author, *models.Author](db).Where(author.Name.Eq("s3cr3t-needle")).All(context.Background()); err != nil {
			t.Fatal(err)
		}
		verify(t, buf.String())
	}, opts...)
}

func TestSlogHook(t *testing.T) {
	t.Run("record has op table sql duration and rows, no values", func(t *testing.T) {
		capture(t, []momolog.Option{momolog.Level(slog.LevelDebug)}, nil, func(t *testing.T, out string) {
			for _, want := range []string{"level=DEBUG", "op=select", "table=authors", "sql=", "FROM", "duration=", "rows=0"} {
				if !strings.Contains(out, want) {
					t.Fatalf("log lacks %q:\n%s", want, out)
				}
			}
			if strings.Contains(out, "s3cr3t-needle") {
				t.Fatalf("argument value leaked:\n%s", out)
			}
		})
	})

	t.Run("WithArgLogging alone does not log values", func(t *testing.T) {
		capture(t, nil, []momo.Option{momo.WithArgLogging()}, func(t *testing.T, out string) {
			if strings.Contains(out, "s3cr3t-needle") {
				t.Fatalf("argument value leaked:\n%s", out)
			}
		})
	})

	t.Run("Values alone is not enough, it also needs WithArgLogging", func(t *testing.T) {
		capture(t, []momolog.Option{momolog.Values(true)}, nil, func(t *testing.T, out string) {
			if strings.Contains(out, "s3cr3t-needle") {
				t.Fatalf("value logged without WithArgLogging:\n%s", out)
			}
		})
	})

	t.Run("Values plus WithArgLogging logs the value", func(t *testing.T) {
		capture(t, []momolog.Option{momolog.Values(true)}, []momo.Option{momo.WithArgLogging()}, func(t *testing.T, out string) {
			if !strings.Contains(out, "s3cr3t-needle") {
				t.Fatalf("value missing with Values(true) and WithArgLogging:\n%s", out)
			}
		})
	})

	t.Run("slow statements log at WARN, fast ones at DEBUG", func(t *testing.T) {
		capture(t, []momolog.Option{momolog.SlowOver(time.Nanosecond)}, nil, func(t *testing.T, out string) {
			if !strings.Contains(out, "level=WARN") {
				t.Fatalf("slow statement not at WARN:\n%s", out)
			}
		})
		capture(t, []momolog.Option{momolog.SlowOver(time.Hour)}, nil, func(t *testing.T, out string) {
			if !strings.Contains(out, "level=DEBUG") {
				t.Fatalf("fast statement not at DEBUG:\n%s", out)
			}
		})
	})

	t.Run("errors are logged", func(t *testing.T) {
		var buf bytes.Buffer
		l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
			buf.Reset()
			if _, err := momo.Exec(context.Background(), db, `SELECT * FROM no_such_table`); err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(buf.String(), "err=") {
				t.Fatalf("log lacks err=:\n%s", buf.String())
			}
		}, momo.WithHook(momolog.New(l)))
	})
}
```

- [ ] **Step 2: Run and expect FAIL** — `go test ./momolog/ -run '^TestSlogHook$' -count=1` fails with `no required module provides package github.com/azrtydxb/momo/momolog` (or `undefined: momolog.New` once the package file exists empty).

- [ ] **Step 3: Implement `momolog/momolog.go`**:

```go
// Package momolog provides a log/slog hook for Momo.
package momolog

import (
	"context"
	"log/slog"
	"time"

	"github.com/azrtydxb/momo"
)

type hook struct {
	l      *slog.Logger
	level  slog.Level
	slow   time.Duration
	values bool
}

// Option configures New.
type Option func(*hook)

// Level sets the level for ordinary statements (default Debug).
func Level(l slog.Level) Option { return func(h *hook) { h.level = l } }

// SlowOver logs statements slower than d at Warn.
func SlowOver(d time.Duration) Option { return func(h *hook) { h.slow = d } }

// Values includes bound argument values in the record. It has an effect only
// when the DB was also opened with momo.WithArgLogging; otherwise the event
// carries no values to log.
func Values(on bool) Option { return func(h *hook) { h.values = on } }

// New returns a momo.Hook that logs one record per finished statement with
// the attributes op, table, relation (when set), sql, duration, rows and err
// (when set).
func New(l *slog.Logger, opts ...Option) momo.Hook {
	h := &hook{l: l, level: slog.LevelDebug}
	for _, o := range opts {
		o(h)
	}
	return h
}

func (h *hook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context { return ctx }

func (h *hook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
	level := h.level
	if h.slow > 0 && e.Duration > h.slow {
		level = slog.LevelWarn
	}
	if !h.l.Enabled(ctx, level) {
		return
	}
	attrs := make([]slog.Attr, 0, 9)
	attrs = append(attrs, slog.String("op", e.Op))
	if e.Table != "" {
		attrs = append(attrs, slog.String("table", e.Table))
	}
	if e.Relation != "" {
		attrs = append(attrs, slog.String("relation", e.Relation))
	}
	attrs = append(attrs,
		slog.String("sql", e.SQL),
		slog.Duration("duration", e.Duration),
		slog.Int64("rows", e.Rows),
	)
	if h.values && e.Args != nil {
		attrs = append(attrs, slog.Any("args", e.Args))
	}
	if e.Err != nil {
		attrs = append(attrs, slog.String("err", e.Err.Error()))
	}
	h.l.LogAttrs(ctx, level, "momo.query", attrs...)
}
```

- [ ] **Step 4: Run and expect PASS** — `go test ./momolog/ -run '^TestSlogHook$' -count=1` prints `ok`.

- [ ] **Step 5: Mutation, params leak** — change `if h.values && e.Args != nil {` to `if e.Args != nil {`. Run `go test ./momolog/ -run '^TestSlogHook$' -count=1`; expect FAIL with `argument value leaked`. Restore the condition.

- [ ] **Step 6: Run and expect PASS** — `go test ./momolog/ -run '^TestSlogHook$' -count=1` prints `ok`.

- [ ] **Step 7: Commit** — `git add momolog && git commit -m "feat(momolog): add slog hook with levels and slow threshold"`.

## Task 12: N+1 detector

Files:

- Create `nplusone.go`
- Create `nplusone_internal_test.go` (package `momo`)
- Create `obs_nplusone_test.go` (package `momo_test`)

Interfaces:

```go
type NPlusOneFinding struct{ Shape, Example string; Count int }
func WithQueryScope(ctx context.Context) context.Context
func NewNPlusOneHook(threshold int, report func(NPlusOneFinding)) Hook
func normalizeShape(sql string) string
```

Covers: S-53. Acceptance test named here: `TestNPlusOneHook` (plus `TestNormalizeShape`).

- [ ] **Step 1: Write the failing shape test** — create `nplusone_internal_test.go`:

```go
package momo

import "testing"

func TestNormalizeShape(t *testing.T) {
	cases := []struct{ in, want string }{
		{`SELECT * FROM "a" WHERE "a"."id" IN (?, ?, ?)`, `SELECT * FROM "a" WHERE "a"."id" IN (?)`},
		{`SELECT * FROM "a" WHERE "a"."id" IN ($1, $2)`, `SELECT * FROM "a" WHERE "a"."id" IN (?)`},
		{`SELECT * FROM "a" WHERE "a"."id" = $3`, `SELECT * FROM "a" WHERE "a"."id" = ?`},
		{`SELECT * FROM "a" WHERE "a"."id" = ?`, `SELECT * FROM "a" WHERE "a"."id" = ?`},
		{`SELECT * FROM "a" WHERE ("x", "y") IN ((?, ?), (?, ?), (?, ?))`, `SELECT * FROM "a" WHERE ("x", "y") IN ((?))`},
	}
	for _, c := range cases {
		if got := normalizeShape(c.in); got != c.want {
			t.Errorf("normalizeShape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Write the failing detector test** — create `obs_nplusone_test.go`:

```go
package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// nPlusOne runs fn on every dialect with a DB carrying the detector hook.
func nPlusOne(t *testing.T, threshold int, fn func(t *testing.T, db *momo.DB, found *[]momo.NPlusOneFinding)) {
	t.Helper()
	var found []momo.NPlusOneFinding
	hook := momo.NewNPlusOneHook(threshold, func(f momo.NPlusOneFinding) { found = append(found, f) })
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		obsSeed(t, db)
		found = nil
		fn(t, db, &found)
	}, momo.WithHook(hook))
}

func nPlusOneLoop(t *testing.T, ctx context.Context, db *momo.DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := momo.Select[models.Author, *models.Author](db).Where(author.ID.Eq(int64(1))).One(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNPlusOneHook(t *testing.T) {
	t.Run("a loop of single-row selects is reported once", func(t *testing.T) {
		nPlusOne(t, 3, func(t *testing.T, db *momo.DB, found *[]momo.NPlusOneFinding) {
			nPlusOneLoop(t, momo.WithQueryScope(context.Background()), db, 5)
			if len(*found) != 1 {
				t.Fatalf("want exactly one finding for 5 repeats at threshold 3, got %d", len(*found))
			}
			f := (*found)[0]
			if f.Count != 4 || f.Shape == "" || f.Example == "" {
				t.Fatalf("finding %+v, want Count 4 (reported when the threshold is first exceeded)", f)
			}
		})
	})

	t.Run("without a scope nothing is tracked", func(t *testing.T) {
		nPlusOne(t, 3, func(t *testing.T, db *momo.DB, found *[]momo.NPlusOneFinding) {
			nPlusOneLoop(t, context.Background(), db, 5)
			if len(*found) != 0 {
				t.Fatalf("unscoped context reported %d findings", len(*found))
			}
		})
	})

	t.Run("scopes are independent", func(t *testing.T) {
		nPlusOne(t, 3, func(t *testing.T, db *momo.DB, found *[]momo.NPlusOneFinding) {
			nPlusOneLoop(t, momo.WithQueryScope(context.Background()), db, 3)
			nPlusOneLoop(t, momo.WithQueryScope(context.Background()), db, 3)
			if len(*found) != 0 {
				t.Fatalf("3 repeats per scope at threshold 3 reported %d findings", len(*found))
			}
		})
	})

	t.Run("one batched relation load stays silent", func(t *testing.T) {
		nPlusOne(t, 1, func(t *testing.T, db *momo.DB, found *[]momo.NPlusOneFinding) {
			ctx := momo.WithQueryScope(context.Background())
			docs, err := momo.Select[models.Document, *models.Document](db).OrderBy(document.ID.Asc()).All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := momo.Load(ctx, db, docs, document.Author); err != nil {
				t.Fatal(err)
			}
			if len(*found) != 0 {
				t.Fatalf("batched load reported %+v", *found)
			}
		})
	})
}
```

- [ ] **Step 3: Run and expect FAIL** — `go test ./ -run 'TestNormalizeShape|TestNPlusOneHook' -count=1` fails with `undefined: normalizeShape`, `undefined: momo.NewNPlusOneHook` and `undefined: momo.WithQueryScope`.

- [ ] **Step 4: Implement `nplusone.go`**:

```go
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
	pgPlaceholder  = regexp.MustCompile(`\$\d+`)
	inList         = regexp.MustCompile(`\(\?(, \?)*\)`)
	tupleList      = regexp.MustCompile(`\(\?\)(, \(\?\))+`)
)

// normalizeShape collapses numbered placeholders to ? and any run of
// placeholders inside parentheses to one, so batched IN loads of different
// sizes share a shape while per-row statements repeat it.
func normalizeShape(sql string) string {
	s := pgPlaceholder.ReplaceAllString(sql, "?")
	s = inList.ReplaceAllString(s, "(?)")
	return tupleList.ReplaceAllString(s, "(?)")
}

type nPlusOneHook struct {
	threshold int
	report    func(NPlusOneFinding)
}

// NewNPlusOneHook returns a development hook that calls report once per
// statement shape per scope when the shape has run more than threshold times
// (a threshold below 1 counts as 1). Transaction control and EXPLAIN
// statements are ignored. report runs synchronously on the querying
// goroutine and must not query the database.
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
	sc.counts[shape]++
	n := sc.counts[shape]
	fire := n > h.threshold && !sc.reported[shape]
	if fire {
		sc.reported[shape] = true
	}
	sc.mu.Unlock()
	if fire {
		h.report(NPlusOneFinding{Shape: shape, Example: e.SQL, Count: n})
	}
	return ctx
}

func (h *nPlusOneHook) AfterQuery(ctx context.Context, e *QueryEvent) {}
```

- [ ] **Step 5: Run and expect PASS** — `gofmt -w nplusone.go && go test ./ -run 'TestNormalizeShape|TestNPlusOneHook' -count=1` prints `ok`.

- [ ] **Step 6: Mutation, detector misses the loop** — in `normalizeShape` return `sql` unchanged for `$`-free input by deleting the `inList` replacement line. Run; expect FAIL in `TestNormalizeShape` with `normalizeShape("SELECT * FROM \"a\" WHERE \"a\".\"id\" IN (?, ?, ?)")`. Restore.

- [ ] **Step 7: Commit** — `git add nplusone.go nplusone_internal_test.go obs_nplusone_test.go && git commit -m "feat(observability): add scoped N+1 detection hook"`.

## Task 13: Slow-query hook with opt-in plain EXPLAIN

Files:

- Create `slowquery.go`
- Create `obs_slowquery_test.go` (package `momo_test`)

Interfaces:

```go
type SlowQuery struct{ SQL string; Duration time.Duration; Plan []string; PlanSkipped string }
func WithPlainExplain(q Querier) SlowOption
func NewSlowQueryHook(threshold time.Duration, report func(SlowQuery), opts ...SlowOption) Hook
```

Covers: S-54. Acceptance test named here: `TestSlowQueryHook`.

- [ ] **Step 1: Write the failing test** — create `obs_slowquery_test.go`:

```go
package momo_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

const slowCTE = `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x < 2000000) SELECT count(*) FROM c`
const slowFailingCTE = `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x < 2000000) SELECT sum(x) / (count(*) - count(*)) FROM c`
const slowCTEParam = `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x < ?) SELECT count(*) FROM c`

// badExplainDriver turns every EXPLAIN statement into one that fails (a
// missing table), the way a rejected EXPLAIN would. On PostgreSQL such a
// failure aborts the transaction unless a savepoint contains it.
type badExplainDriver struct{ momo.Driver }

func failExplain(sql string, args []any) (string, []any) {
	if strings.HasPrefix(sql, "EXPLAIN ") {
		return `SELECT * FROM no_such_table_for_explain`, nil
	}
	return sql, args
}

func (d badExplainDriver) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	sql, args = failExplain(sql, args)
	return d.Driver.Query(ctx, sql, args)
}

func (d badExplainDriver) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	sql, args = failExplain(sql, args)
	return d.Driver.Exec(ctx, sql, args)
}

func (d badExplainDriver) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	tx, err := d.Driver.Begin(ctx, opts)
	if err != nil {
		return nil, err
	}
	return badExplainTx{tx}, nil
}

type badExplainTx struct{ momo.DriverTx }

func (t badExplainTx) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	sql, args = failExplain(sql, args)
	return t.DriverTx.Query(ctx, sql, args)
}

func (t badExplainTx) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	sql, args = failExplain(sql, args)
	return t.DriverTx.Exec(ctx, sql, args)
}

type slowSink struct {
	mu sync.Mutex
	qs []momo.SlowQuery
}

func (s *slowSink) add(q momo.SlowQuery) {
	s.mu.Lock()
	s.qs = append(s.qs, q)
	s.mu.Unlock()
}

// only returns the reports for exactly sql, ignoring unrelated statements
// (connection set-up, BEGIN) that may also cross a low threshold.
func (s *slowSink) only(sql string) []momo.SlowQuery {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []momo.SlowQuery
	for _, q := range s.qs {
		if q.SQL == sql {
			out = append(out, q)
		}
	}
	return out
}

func (s *slowSink) anyContaining(sub string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, q := range s.qs {
		if strings.Contains(q.SQL, sub) {
			return true
		}
	}
	return false
}

func TestSlowQueryHook(t *testing.T) {
	ctx := context.Background()
	const threshold = 50 * time.Millisecond
	rec := &obstest.Recorder{}
	// db from Each carries only the recorder: it is the hook-free "plain"
	// querier whose EXPLAIN statements the recorder lists.
	testopen.Each(t, func(t *testing.T, plain *momo.DB, d dialect.Name) {
		obsSeed(t, plain)
		if _, err := momo.Exec(ctx, plain, `SELECT 1`); err != nil { // warm the pool
			t.Fatal(err)
		}
		drv := plain.Driver()

		t.Run("reports only slow statements and captures a plain plan", func(t *testing.T) {
			rec.Reset()
			sink := &slowSink{}
			db := momo.New(drv, momo.WithHook(momo.NewSlowQueryHook(threshold, sink.add, momo.WithPlainExplain(plain))))
			if _, err := momo.Select[models.Author, *models.Author](db).Where(author.ID.Eq(1)).One(ctx); err != nil {
				t.Fatal(err)
			}
			if sink.anyContaining(`"authors"`) {
				t.Fatal("a fast statement was reported")
			}
			if _, err := momo.Exec(ctx, db, slowCTE); err != nil {
				t.Fatal(err)
			}
			got := sink.only(slowCTE)
			if len(got) != 1 {
				t.Fatalf("want 1 slow report, got %d", len(got))
			}
			if got[0].Duration <= threshold {
				t.Fatalf("report %+v", got[0])
			}
			if len(got[0].Plan) == 0 || got[0].PlanSkipped != "" {
				t.Fatalf("Plan = %q PlanSkipped = %q, want a plan", got[0].Plan, got[0].PlanSkipped)
			}
			explains := 0
			for _, s := range rec.SQLs() {
				if strings.Contains(strings.ToUpper(s), "ANALYZE") {
					t.Fatalf("ANALYZE was issued: %q", s)
				}
				if strings.HasPrefix(s, "EXPLAIN ") {
					explains++
				}
			}
			if explains != 1 {
				t.Fatalf("want exactly one EXPLAIN statement, got %d", explains)
			}
		})

		t.Run("no plan unless WithPlainExplain is given", func(t *testing.T) {
			sink := &slowSink{}
			db := momo.New(drv, momo.WithHook(momo.NewSlowQueryHook(threshold, sink.add)))
			if _, err := momo.Exec(ctx, db, slowCTE); err != nil {
				t.Fatal(err)
			}
			got := sink.only(slowCTE)
			if len(got) != 1 || got[0].Plan != nil || got[0].PlanSkipped != "" {
				t.Fatalf("report %+v, want a bare report", got)
			}
		})

		t.Run("plan captured inside a transaction through a savepoint", func(t *testing.T) {
			sink := &slowSink{}
			db := momo.New(drv, momo.WithHook(momo.NewSlowQueryHook(threshold, sink.add, momo.WithPlainExplain(plain))))
			err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
				if _, err := momo.Exec(ctx, tx, slowCTE); err != nil {
					return err
				}
				_, err := momo.Exec(ctx, tx, `SELECT 1`)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			got := sink.only(slowCTE)
			if len(got) != 1 || len(got[0].Plan) == 0 || got[0].PlanSkipped != "" {
				t.Fatalf("report %+v, want a plan captured in the transaction", got)
			}
		})

		t.Run("a failing capture leaves the transaction usable", func(t *testing.T) {
			sink := &slowSink{}
			db := momo.New(badExplainDriver{drv}, momo.WithHook(momo.NewSlowQueryHook(threshold, sink.add, momo.WithPlainExplain(plain))))
			err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
				if _, err := momo.Exec(ctx, tx, slowCTE); err != nil {
					return err
				}
				_, err := momo.Exec(ctx, tx, `INSERT INTO authors (id, name) VALUES (?, ?)`, int64(77), "after")
				return err
			})
			if err != nil {
				t.Fatalf("transaction broken by the failed capture: %v", err)
			}
			got := sink.only(slowCTE)
			if len(got) != 1 || got[0].Plan != nil || !strings.Contains(got[0].PlanSkipped, "EXPLAIN failed") {
				t.Fatalf("report %+v, want PlanSkipped to say EXPLAIN failed", got)
			}
			if n := obsAuthorCount(t, plain, 77); n != 1 {
				t.Fatalf("row written after the failed capture is missing: %d", n)
			}
		})

		t.Run("an aborted transaction is reported as such", func(t *testing.T) {
			if d != dialect.Postgres {
				return // only PostgreSQL aborts a transaction after a failed statement
			}
			sink := &slowSink{}
			db := momo.New(drv, momo.WithHook(momo.NewSlowQueryHook(threshold, sink.add, momo.WithPlainExplain(plain))))
			err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
				_, err := momo.Exec(ctx, tx, slowFailingCTE)
				return err
			})
			if err == nil {
				t.Fatal("want the division by zero error")
			}
			got := sink.only(slowFailingCTE)
			if len(got) != 1 || got[0].Plan != nil || got[0].PlanSkipped != "plan unavailable: transaction aborted" {
				t.Fatalf("report %+v, want 'plan unavailable: transaction aborted'", got)
			}
		})

		t.Run("parameterized statements need WithArgLogging", func(t *testing.T) {
			quiet, loud := &slowSink{}, &slowSink{}
			dbQuiet := momo.New(drv, momo.WithHook(momo.NewSlowQueryHook(threshold, quiet.add, momo.WithPlainExplain(plain))))
			dbLoud := momo.New(drv, momo.WithArgLogging(), momo.WithHook(momo.NewSlowQueryHook(threshold, loud.add, momo.WithPlainExplain(plain))))
			for _, db := range []*momo.DB{dbQuiet, dbLoud} {
				if _, err := momo.Exec(ctx, db, slowCTEParam, 2000000); err != nil {
					t.Fatal(err)
				}
			}
			q, l := quiet.only(slowCTEParam), loud.only(slowCTEParam)
			if len(q) != 1 || q[0].Plan != nil || !strings.Contains(q[0].PlanSkipped, "arguments") {
				t.Fatalf("without WithArgLogging: %+v", q)
			}
			if len(l) != 1 || len(l[0].Plan) == 0 {
				t.Fatalf("with WithArgLogging: %+v", l)
			}
		})

		t.Run("the explain statement is not reported again", func(t *testing.T) {
			first, second := &slowSink{}, &slowSink{}
			// observed has its own slow hook with a 1ns threshold: without the
			// recursion guard it would report the EXPLAIN statement.
			observed := momo.New(drv, momo.WithHook(momo.NewSlowQueryHook(time.Nanosecond, second.add)))
			db := momo.New(drv, momo.WithHook(momo.NewSlowQueryHook(threshold, first.add, momo.WithPlainExplain(observed))))
			if _, err := momo.Exec(ctx, db, slowCTE); err != nil {
				t.Fatal(err)
			}
			if second.anyContaining("EXPLAIN") {
				t.Fatal("an EXPLAIN statement was reported")
			}
			if len(first.only(slowCTE)) != 1 {
				t.Fatalf("want 1 report, got %d", len(first.only(slowCTE)))
			}
		})
	}, momo.WithHook(rec))
}
```

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run '^TestSlowQueryHook$' -count=1` fails with `undefined: momo.NewSlowQueryHook`, `undefined: momo.WithPlainExplain` and `undefined: momo.SlowQuery`.

- [ ] **Step 3: Implement `slowquery.go`**:

```go
package momo

import (
	"context"
	"errors"
	"time"
)

// SlowQuery describes one statement slower than the hook's threshold.
// Plan holds the plain EXPLAIN output when it was captured; PlanSkipped says
// why it was not when a plan was requested with WithPlainExplain.
type SlowQuery struct {
	SQL         string
	Duration    time.Duration
	Plan        []string
	PlanSkipped string
}

// SlowOption configures NewSlowQueryHook.
type SlowOption func(*slowHook)

// WithPlainExplain turns on plan capture, running plain EXPLAIN (never
// ANALYZE). Statements outside a transaction are explained through q; pass a
// *DB that is NOT the one carrying the slow-query hook, for example a second
// momo.New on the same driver without hooks. Statements inside a transaction
// are explained on that transaction.
func WithPlainExplain(q Querier) SlowOption { return func(h *slowHook) { h.explain = q } }

type slowGuardKey struct{}

const slowExplainTimeout = 2 * time.Second

type slowHook struct {
	threshold time.Duration
	report    func(SlowQuery)
	explain   Querier
}

// NewSlowQueryHook calls report for each statement that took longer than
// threshold. Capturing a plan runs one extra plain EXPLAIN (never ANALYZE)
// with the statement's own arguments, on the goroutine that ran the slow
// statement, after it finished. Inside a transaction the EXPLAIN runs on that
// transaction in a savepoint that is always rolled back, so a failing capture
// never aborts the caller's transaction; an already aborted transaction is
// reported as "plan unavailable: transaction aborted". Parameterized
// statements are skipped unless the DB has WithArgLogging. report runs
// synchronously and must not query the database.
func NewSlowQueryHook(threshold time.Duration, report func(SlowQuery), opts ...SlowOption) Hook {
	h := &slowHook{threshold: threshold, report: report}
	for _, o := range opts {
		o(h)
	}
	return h
}

func (h *slowHook) BeforeQuery(ctx context.Context, e *QueryEvent) context.Context { return ctx }

func (h *slowHook) AfterQuery(ctx context.Context, e *QueryEvent) {
	if e.Duration <= h.threshold || ctx.Value(slowGuardKey{}) != nil {
		return
	}
	sq := SlowQuery{SQL: e.SQL, Duration: e.Duration}
	switch {
	case h.explain == nil:
	case e.NumArgs > 0 && e.Args == nil:
		sq.PlanSkipped = "statement has bound arguments and the DB was opened without WithArgLogging"
	case !isExplainable(e.SQL):
		sq.PlanSkipped = "statement kind is not explainable"
	default:
		ectx, cancel := context.WithTimeout(context.WithValue(context.WithoutCancel(ctx), slowGuardKey{}, true), slowExplainTimeout)
		sq.Plan, sq.PlanSkipped = h.capture(ectx, e)
		cancel()
	}
	h.report(sq)
}

var errSlowRollback = errors.New("momo: slow query plan capture rollback")

// capture returns the plan, or the reason there is none.
func (h *slowHook) capture(ctx context.Context, e *QueryEvent) ([]string, string) {
	c := Compiled{SQL: e.SQL, Args: e.Args}
	if !e.InTx {
		plan, err := Explain(ctx, h.explain, c)
		if err != nil {
			return nil, "EXPLAIN failed: " + err.Error()
		}
		return plan, ""
	}
	tx, ok := e.Querier.(*Tx)
	if !ok {
		return nil, "plan unavailable: the transaction handle is not available to the hook"
	}
	var plan []string
	var explainErr error
	ran := false
	err := tx.Tx(ctx, func(inner *Tx) error {
		ran = true
		plan, explainErr = Explain(ctx, inner, c)
		if explainErr != nil {
			return explainErr
		}
		return errSlowRollback
	})
	switch {
	case !ran && errors.Is(err, ErrUnsupported):
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

func isExplainable(sql string) bool {
	switch firstKeyword(sql) {
	case "SELECT", "WITH", "INSERT", "UPDATE", "DELETE", "VALUES", "REPLACE", "MERGE":
		return true
	}
	return false
}
```

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestSlowQueryHook$' -count=1` prints `ok`.

- [ ] **Step 5: Mutation, no recursion guard** — remove the `|| ctx.Value(slowGuardKey{}) != nil` condition. Run; expect FAIL with `an EXPLAIN statement was reported`. Restore.

- [ ] **Step 5b: Mutation, no savepoint** — in `capture`, replace the `tx.Tx(...)` call by `plan, explainErr = Explain(ctx, tx, c)` with `ran = true` and `err = nil`. Run; expect FAIL with `transaction broken by the failed capture` (PostgreSQL reports `current transaction is aborted`). Restore.

- [ ] **Step 6: Mutation, ANALYZE** — in `AfterQuery` replace `Explain(` with `ExplainAnalyze(`. Run; expect FAIL with `want a plan` (the SQLite dialect rejects ExplainAnalyze, so the report carries `EXPLAIN failed` instead of a plan). Restore.

- [ ] **Step 7: Commit** — `git add slowquery.go obs_slowquery_test.go && git commit -m "feat(observability): add slow-query hook with opt-in plain EXPLAIN"`.

## Task 14: Bounded-memory streaming

Files:

- Create `obs_streaming_test.go` (package `momo_test`)
- Modify `docs/PERFORMANCE.md` (streaming bound row)
- Modify `select.go` only in the mutation step (reverted)

Interfaces:

```go
func (sq SelectQuery[T]) Each(ctx context.Context, fn func(*T) error) error
```

Covers: S-56. Acceptance test named here: `TestStreamingBoundedMemory`. The test uses the fixture `images` table (no foreign keys) instead of a new table, so no model is redefined. If the test fails because `Each` or `Iter` buffers, the fix belongs to the query owner (momo-05 and the foundation plan that owns `select.go`); this plan adds no buffering and no workaround.

- [ ] **Step 1: Write the test** — create `obs_streaming_test.go`:

```go
package momo_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

const (
	streamRows     = 1_000_000
	streamSample   = 100_000
	streamMaxGrowB = 16 << 20 // 16 MiB: the plan's chosen bound for S-56
)

func heapAlloc() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

func TestStreamingBoundedMemory(t *testing.T) {
	ctx := context.Background()
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		relfixture.CreateSchema(t, db)
		if _, err := momo.Exec(ctx, db, `WITH RECURSIVE c(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM c WHERE n < 1000000)
INSERT INTO images (id, owner_type, owner_id, url) SELECT n, 'document', n, 'https://example.test/img/' || n FROM c`); err != nil {
			t.Fatal(err)
		}

		base := heapAlloc()
		var maxHeap uint64
		seen := 0
		err := momo.Select[models.Image, *models.Image](db).Each(ctx, func(im *models.Image) error {
			seen++
			if seen%streamSample == 0 {
				if h := heapAlloc(); h > maxHeap {
					maxHeap = h
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if seen != streamRows {
			t.Fatalf("iterated %d rows, want %d", seen, streamRows)
		}
		if growth := int64(maxHeap) - int64(base); growth > streamMaxGrowB {
			t.Fatalf("heap grew %d bytes while streaming %d rows, bound %d: Each is buffering", growth, streamRows, streamMaxGrowB)
		}
	})
}
```

- [ ] **Step 2: Run and expect PASS** — `go test ./ -run '^TestStreamingBoundedMemory$' -count=1` prints `ok` (the spike streams, so the test passes at once; the next step proves it can fail).

- [ ] **Step 3: Mutation, buffering Each** — in `select.go`, replace the body of `Each` with a version that first collects everything:

```go
all, err := sq.All(ctx)
if err != nil {
	return err
}
for i := range all {
	if err := fn(&all[i]); err != nil {
		return err
	}
}
return nil
```

Run `go test ./ -run '^TestStreamingBoundedMemory$' -count=1`; expect FAIL with `Each is buffering`. Restore `Each` with `git checkout -- select.go`.

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestStreamingBoundedMemory$' -count=1` prints `ok`.

- [ ] **Step 5: Write the failing bound-documentation test** — add to `obs_streaming_test.go` (imports `os`, `regexp`, `strconv`):

```go
func TestPerformanceDocRecordsStreamBound(t *testing.T) {
	raw, err := os.ReadFile("docs/PERFORMANCE.md")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^\| Streaming heap growth bound \(MiB\) \| (\d+) \|$`).FindSubmatch(raw)
	if m == nil {
		t.Fatal("docs/PERFORMANCE.md has no row '| Streaming heap growth bound (MiB) | N |'")
	}
	got, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatal(err)
	}
	if got != streamMaxGrowB>>20 {
		t.Fatalf("docs/PERFORMANCE.md records %d MiB, constant is %d MiB", got, streamMaxGrowB>>20)
	}
}
```

Run `go test ./ -run '^TestPerformanceDocRecordsStreamBound$' -count=1`; expect FAIL with `docs/PERFORMANCE.md has no row '| Streaming heap growth bound (MiB) | N |'`.

- [ ] **Step 6: Document the bound** — append to `docs/PERFORMANCE.md`:

```markdown
## Streaming

| Limit                             | Value |
| --------------------------------- | ----- |
| Streaming heap growth bound (MiB) | 16    |
```

Run `go test ./ -run '^TestPerformanceDocRecordsStreamBound$' -count=1`; prints `ok`.

- [ ] **Step 7: Commit** — `git add obs_streaming_test.go docs && git commit -m "test(streaming): assert Each keeps heap flat over one million rows"`.

## Task 15: otelmomo module, spans

Files:

- Create `otelmomo/go.mod`, `otelmomo/go.sum`
- Create `otelmomo/doc.go`, `otelmomo/otelmomo.go`, `otelmomo/system.go`
- Create `otelmomo/system_internal_test.go` (package `otelmomo`), `otelmomo/otelmomo_test.go` (package `otelmomo_test`)

Interfaces:

```go
func New(tp trace.TracerProvider, mp metric.MeterProvider, opts ...Option) (momo.Hook, error)
type Option func(*config)
func WithNamespace(ns string) Option
func systemName(d *dialect.Dialect) dbconv.SystemNameAttr
```

Covers: S-35 (separate module, spans per OTel database conventions, no globals). Acceptance tests named here: `TestOtelSpans` (plus `TestSystemName`, `TestNewRejectsNilProviders`); `TestCoreHasNoOTel` is Task 4.

- [ ] **Step 1: Create the module** — run from the repository root:

```sh
mkdir -p otelmomo && cd otelmomo
go mod init github.com/azrtydxb/momo/otelmomo
go mod edit -go=1.26 -require=github.com/azrtydxb/momo@v0.0.0 -replace=github.com/azrtydxb/momo=../
printf '// Package otelmomo is a momo.Hook that emits OpenTelemetry spans and metrics\n// following the database client semantic conventions.\npackage otelmomo\n' > doc.go
go get go.opentelemetry.io/otel@v1.47.0 go.opentelemetry.io/otel/metric@v1.47.0 go.opentelemetry.io/otel/trace@v1.47.0 go.opentelemetry.io/otel/sdk@v1.47.0 go.opentelemetry.io/otel/sdk/metric@v1.47.0
```

The tests import the fixture models through `github.com/azrtydxb/momo/internal/...`; Go's internal-package rule is checked on import paths, and `github.com/azrtydxb/momo/otelmomo` lies inside the tree rooted at `github.com/azrtydxb/momo`, so the import is permitted although the module differs.

- [ ] **Step 2: Write the failing tests** — create `otelmomo/system_internal_test.go`:

```go
package otelmomo

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestSystemName(t *testing.T) {
	cases := []struct {
		d    *dialect.Dialect
		want string
	}{
		{dialect.For(dialect.Postgres), "postgresql"},
		{dialect.For(dialect.SQLite), "sqlite"},
		{dialect.MySQLFlavor(dialect.FlavorMySQL), "mysql"},
		{dialect.MySQLFlavor(dialect.FlavorMariaDB), "mariadb"},
		{nil, "other_sql"},
	}
	for _, c := range cases {
		if got := string(systemName(c.d)); got != c.want {
			t.Errorf("systemName = %q, want %q", got, c.want)
		}
	}
}
```

Create `otelmomo/otelmomo_test.go`:

```go
package otelmomo_test

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/otelmomo"
)

// wantSystem is the db.system.name value for a dialect.
func wantSystem(d dialect.Name) string {
	if d == dialect.Postgres {
		return "postgresql"
	}
	return string(d)
}

func attrs(kvs []attribute.KeyValue) map[string]string {
	m := map[string]string{}
	for _, kv := range kvs {
		m[string(kv.Key)] = kv.Value.Emit()
	}
	return m
}

func TestOtelSpans(t *testing.T) {
	ctx := context.Background()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() {
		if err := tp.Shutdown(ctx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	h, err := otelmomo.New(tp, sdkmetric.NewMeterProvider(), otelmomo.WithNamespace("appdb"))
	if err != nil {
		t.Fatal(err)
	}
	// WithArgLogging makes argument values available to the hook; the span
	// must still never carry them.
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		relfixture.CreateSchema(t, db)
		before := len(sr.Ended())
		if _, err := momo.Select[models.Author, *models.Author](db).Where(author.Name.Eq("needle-value")).All(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, db, `SELECT * FROM missing_table`); err == nil {
			t.Fatal("want an error for a missing table")
		}

		byName := map[string]sdktrace.ReadOnlySpan{}
		for _, s := range sr.Ended()[before:] {
			byName[s.Name()] = s
			for k, v := range attrs(s.Attributes()) {
				if strings.Contains(v, "needle-value") {
					t.Fatalf("span %q attribute %s leaks an argument value", s.Name(), k)
				}
				if k == "db.system" {
					t.Fatalf("span %q uses the obsolete db.system attribute", s.Name())
				}
			}
		}

		sel, ok := byName["SELECT authors"]
		if !ok {
			t.Fatalf("no span named %q; have %v", "SELECT authors", sr.Ended()[before:])
		}
		if sel.SpanKind() != trace.SpanKindClient {
			t.Fatalf("span kind = %v, want client", sel.SpanKind())
		}
		a := attrs(sel.Attributes())
		if a["db.system.name"] != wantSystem(d) || a["db.namespace"] != "appdb" || a["db.collection.name"] != "authors" || a["db.operation.name"] != "SELECT" {
			t.Fatalf("attributes %v", a)
		}
		if !strings.Contains(a["db.query.text"], "FROM") || strings.Contains(a["db.query.text"], "needle") {
			t.Fatalf("db.query.text = %q, want parameterized SQL", a["db.query.text"])
		}
		if sel.Status().Code == codes.Error {
			t.Fatalf("successful span has error status: %+v", sel.Status())
		}

		bad, ok := byName["SELECT"]
		if !ok {
			t.Fatalf("no span named SELECT for the table-less statement; have %v", sr.Ended()[before:])
		}
		if bad.Status().Code != codes.Error {
			t.Fatalf("failed span status = %+v, want Error", bad.Status())
		}
		if attrs(bad.Attributes())["error.type"] == "" {
			t.Fatalf("failed span lacks error.type: %v", attrs(bad.Attributes()))
		}
	}, momo.WithHook(h), momo.WithArgLogging())
}

func TestNewRejectsNilProviders(t *testing.T) {
	if _, err := otelmomo.New(nil, sdkmetric.NewMeterProvider()); err == nil {
		t.Fatal("nil TracerProvider accepted")
	}
	if _, err := otelmomo.New(sdktrace.NewTracerProvider(), nil); err == nil {
		t.Fatal("nil MeterProvider accepted")
	}
}
```

- [ ] **Step 3: Resolve imports** — in `otelmomo/` run `go mod tidy`. It succeeds (tidy reads imports only); `go.sum` now exists.

- [ ] **Step 4: Run and expect FAIL** — in `otelmomo/` run `go test ./... -run 'TestSystemName|TestOtelSpans|TestNewRejectsNilProviders' -count=1`; expect FAIL with `undefined: systemName` and `undefined: otelmomo.New`.

- [ ] **Step 5: Implement `otelmomo/system.go`**:

```go
package otelmomo

import (
	"go.opentelemetry.io/otel/semconv/v1.43.0/dbconv"

	"github.com/azrtydxb/momo/dialect"
)

// systemName maps a Momo dialect to the db.system.name value.
func systemName(d *dialect.Dialect) dbconv.SystemNameAttr {
	if d == nil {
		return dbconv.SystemNameOtherSQL
	}
	switch d.Name {
	case dialect.Postgres:
		return dbconv.SystemNamePostgreSQL
	case dialect.SQLite:
		return dbconv.SystemNameSQLite
	case dialect.MySQL:
		if d.Flavor == dialect.FlavorMariaDB {
			return dbconv.SystemNameMariaDB
		}
		return dbconv.SystemNameMySQL
	}
	return dbconv.SystemNameOtherSQL
}
```

and `otelmomo/otelmomo.go`:

```go
package otelmomo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/azrtydxb/momo"
)

const scope = "github.com/azrtydxb/momo/otelmomo"

type config struct{ namespace string }

// Option configures New.
type Option func(*config)

// WithNamespace sets db.namespace (the database name) on every span.
func WithNamespace(ns string) Option { return func(c *config) { c.namespace = ns } }

type hook struct {
	tracer trace.Tracer
	ns     string
}

// New returns a momo.Hook that records one client span per statement.
// Providers are passed in; the package uses no global provider. SQL text is
// recorded as db.query.text; argument values never are.
func New(tp trace.TracerProvider, mp metric.MeterProvider, opts ...Option) (momo.Hook, error) {
	if tp == nil {
		return nil, errors.New("otelmomo: nil TracerProvider")
	}
	if mp == nil {
		return nil, errors.New("otelmomo: nil MeterProvider")
	}
	var c config
	for _, o := range opts {
		o(&c)
	}
	return &hook{tracer: tp.Tracer(scope), ns: c.namespace}, nil
}

func operationName(sql string) string {
	s := strings.TrimLeft(sql, " \t\r\n(")
	if i := strings.IndexAny(s, " \t\r\n("); i >= 0 {
		s = s[:i]
	}
	return strings.ToUpper(s)
}

func (h *hook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	op := operationName(e.SQL)
	name := op
	attrs := []attribute.KeyValue{
		semconv.DBSystemNameKey.String(string(systemName(e.Dialect))),
		semconv.DBOperationName(op),
		semconv.DBQueryText(e.SQL),
	}
	if e.Table != "" {
		name = op + " " + e.Table
		attrs = append(attrs, semconv.DBCollectionName(e.Table))
	}
	if h.ns != "" {
		attrs = append(attrs, semconv.DBNamespace(h.ns))
	}
	ctx, _ = h.tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attrs...))
	return ctx
}

func errorType(err error) string {
	var me *momo.Error
	if errors.As(err, &me) && me.Kind != nil {
		return me.Kind.Error()
	}
	return fmt.Sprintf("%T", err)
}

func (h *hook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
	span := trace.SpanFromContext(ctx)
	if e.Err != nil {
		et := errorType(e.Err)
		span.SetAttributes(semconv.ErrorTypeKey.String(et))
		span.SetStatus(codes.Error, et)
	}
	span.End()
}
```

Then run `go mod tidy` in `otelmomo/` again so `go.sum` lists `dbconv` and the semconv package.

- [ ] **Step 6: Run and expect PASS** — in `otelmomo/` run `go test ./... -run 'TestSystemName|TestOtelSpans|TestNewRejectsNilProviders' -count=1`; prints `ok`. In the repository root run `go build ./... && go list ./... | grep -c otelmomo`; the count is `0` (the nested module is not part of the root module).

- [ ] **Step 7: Mutation, parameter leak** — in `BeforeQuery` add `attrs = append(attrs, attribute.String("db.query.parameters", fmt.Sprint(e.Args)))`. Run the span test; expect FAIL with `leaks an argument value`. Remove the line.

- [ ] **Step 8: Commit** — `git add otelmomo && git commit -m "feat(otelmomo): add OpenTelemetry span hook as a separate module"`.

## Task 16: otelmomo metrics

Files:

- Modify `otelmomo/otelmomo.go` (instrument creation in `New`, recording in `AfterQuery`)
- Modify `otelmomo/otelmomo_test.go` (add `TestOtelMetrics`)

Interfaces:

```go
type hook struct { tracer trace.Tracer; ns string; dur dbconv.ClientOperationDuration }
```

Covers: S-35 (metrics following the database conventions). Acceptance test named here: `TestOtelMetrics`.

- [ ] **Step 1: Write the failing test** — append to `otelmomo/otelmomo_test.go` (add the imports `go.opentelemetry.io/otel/sdk/metric/metricdata`):

```go
func TestOtelMetrics(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	h, err := otelmomo.New(sdktrace.NewTracerProvider(), mp, otelmomo.WithNamespace("appdb"))
	if err != nil {
		t.Fatal(err)
	}
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		relfixture.CreateSchema(t, db)
		if _, err := momo.Select[models.Author, *models.Author](db).Where(author.Name.Eq("x")).All(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, db, `SELECT * FROM missing_table`); err == nil {
			t.Fatal("want an error")
		}

		var rm metricdata.ResourceMetrics
		if err := reader.Collect(ctx, &rm); err != nil {
			t.Fatal(err)
		}
		var hist metricdata.Metrics
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				if m.Name == "db.client.operation.duration" {
					hist = m
				}
			}
		}
		data, ok := hist.Data.(metricdata.Histogram[float64])
		if !ok {
			t.Fatalf("db.client.operation.duration histogram missing: %+v", rm)
		}
		if hist.Unit != "s" {
			t.Fatalf("unit = %q, want s", hist.Unit)
		}
		var sawSelect, sawError bool
		for _, dp := range data.DataPoints {
			sys, has := dp.Attributes.Value(attribute.Key("db.system.name"))
			if !has {
				t.Fatalf("data point lacks db.system.name: %v", dp.Attributes)
			}
			if sys.AsString() != wantSystem(d) {
				continue // a data point of another dialect's run
			}
			op, hasOp := dp.Attributes.Value(attribute.Key("db.operation.name"))
			coll, hasColl := dp.Attributes.Value(attribute.Key("db.collection.name"))
			if hasOp && hasColl && op.AsString() == "SELECT" && coll.AsString() == "authors" && dp.Count == 1 && dp.Sum > 0 {
				sawSelect = true
			}
			if _, has := dp.Attributes.Value(attribute.Key("error.type")); has {
				sawError = true
			}
		}
		if !sawSelect || !sawError {
			t.Fatalf("sawSelect=%v sawError=%v in %+v", sawSelect, sawError, data.DataPoints)
		}
	}, momo.WithHook(h))
}
```

- [ ] **Step 2: Run and expect FAIL** — in `otelmomo/` run `go test ./... -run '^TestOtelMetrics$' -count=1`; expect FAIL with `db.client.operation.duration histogram missing`.

- [ ] **Step 3: Implement** — in `otelmomo/otelmomo.go` import `"go.opentelemetry.io/otel/semconv/v1.43.0/dbconv"`, add the field `dur dbconv.ClientOperationDuration` to `hook`, and change `New` to create the instrument and `AfterQuery` to record it:

```go
	dur, err := dbconv.NewClientOperationDuration(mp.Meter(scope))
	if err != nil {
		return nil, fmt.Errorf("otelmomo: create db.client.operation.duration: %w", err)
	}
	return &hook{tracer: tp.Tracer(scope), ns: c.namespace, dur: dur}, nil
```

```go
func (h *hook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
	span := trace.SpanFromContext(ctx)
	attrs := []attribute.KeyValue{semconv.DBOperationName(operationName(e.SQL))}
	if e.Table != "" {
		attrs = append(attrs, semconv.DBCollectionName(e.Table))
	}
	if h.ns != "" {
		attrs = append(attrs, semconv.DBNamespace(h.ns))
	}
	if e.Err != nil {
		et := errorType(e.Err)
		attrs = append(attrs, semconv.ErrorTypeKey.String(et))
		span.SetAttributes(semconv.ErrorTypeKey.String(et))
		span.SetStatus(codes.Error, et)
	}
	h.dur.Record(ctx, e.Duration.Seconds(), systemName(e.Dialect), attrs...)
	span.End()
}
```

- [ ] **Step 4: Run and expect PASS** — in `otelmomo/` run `go mod tidy && go test ./... -count=1`; prints `ok`.

- [ ] **Step 5: Mutation, wrong unit** — change `e.Duration.Seconds()` to `float64(e.Duration.Milliseconds())`. A sub-millisecond SQLite statement then records `0`, so the `dp.Sum > 0` check fails. Run `go test ./... -run '^TestOtelMetrics$' -count=1` in `otelmomo/`; expect FAIL with `sawSelect=false`. Restore `e.Duration.Seconds()`.

- [ ] **Step 6: Run the whole plan** — in the repository root run `go vet ./... && go test ./... -count=1`; in `otelmomo/` run `go vet ./... && go test ./... -count=1`; both print `ok` for every package.

- [ ] **Step 7: Commit** — `git add otelmomo && git commit -m "feat(otelmomo): record db.client.operation.duration histogram"`.
