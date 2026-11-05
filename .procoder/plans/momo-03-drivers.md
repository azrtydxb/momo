# momo-03-drivers — implementation plan

Status: draft
Spec: .procoder/specs/momo.md

## Goal

Deliver the driver adapters (pgx native and database/sql for PostgreSQL, mattn + sqlite-vec and pure-Go modernc for SQLite) with transactions and savepoints, structured driver-error classification, pinned connections, pool statistics, bulk COPY/batch paths, explicit read-replica routing, PgBouncer-safe execution, and SQLite recommended configuration, each proven by tests against real databases.

Delivery: milestone M1 (PostgreSQL) is tasks 1–14; milestone M2 (SQLite and modernc) is tasks 15–25; task 26 is the final gate for both.

## Architecture

Each adapter is a separate package under `driver/` that imports `momo` and only its own database driver. The root `momo` package gains optional capability interfaces (`Pinner`, `Copier`, `Batcher`, `PoolStater`, `CapabilityReporter`) and read-replica routing in `db.go`; adapters satisfy them by implementing methods, and the root never imports a driver. Each adapter converts its raw driver error into `*momo.Error` in `ClassifyError`, keeping the original error reachable through `errors.As`.

## Constraints

Copied verbatim from `.procoder/specs/momo.md` (Constraints section) where they bind this plan:

- SQLite driver: mattn/go-sqlite3 v1.14.52 (cgo) with sqlite-vec v0.1.6 statically linked via github.com/asg017/sqlite-vec-go-bindings/cgo; SQLite builds require cgo and a C toolchain. The database/sql adapter itself stays driver-agnostic.
- PostgreSQL driver: jackc/pgx v5 (pinned) via pgxpool, plus pgx stdlib for *sql.DB adoption.
- Optional pure-Go SQLite: modernc.org/sqlite (pinned), without vector support.
- SQLite: >= 3.35 (RETURNING); the adapter checks the library version on connect.
- Security: identifiers always quoted, values always bound; raw SQL is an explicit escape hatch with a documented contract; parameter values not logged unless explicitly enabled.
- No hidden background goroutines, no implicit migrations at connect time, no unexplained global mutable state.
- Dependencies pinned in go.mod/go.sum.
- CI: GitHub Actions on free GitHub-hosted runners only (public repo): ubuntu-latest for most jobs, windows-latest for the SQLite/generator suite, PostgreSQL+pgvector as a `pgvector/pgvector` service container, SQLite+sqlite-vec via cgo on the runner; a macos-latest job builds and runs the SQLite suite to cover darwin/cgo.

Pinned versions, verified against the Go module proxy and the module sources on 2026-11-04:

| Module                                       | Pin      | Published  | Note                                                               |
| -------------------------------------------- | -------- | ---------- | ------------------------------------------------------------------ |
| github.com/jackc/pgx/v5                      | v5.11.0  | 2026-09-07 | v5.12.0 exists (2026-10-31) and is not used: too new for this plan |
| github.com/mattn/go-sqlite3                  | v1.14.52 | 2026-09-05 | bundles SQLite 3.53.4 (`sqlite3-binding.h`)                        |
| github.com/asg017/sqlite-vec-go-bindings/cgo | v0.1.6   | 2024-11-20 | `Auto()` calls `sqlite3_auto_extension`                            |
| modernc.org/sqlite                           | v1.60.1  | 2026-09-29 | bundles SQLite 3.54.1 in v1.61.1; v1.60.1 is pinned                |

Source facts relied on (read from the pinned module sources):

- pgx `QueryExecMode` is passed as the first variadic argument of `Conn.Query`/`Conn.Exec`; the default is `QueryExecModeCacheStatement`. Calls with zero arguments always use the simple protocol. `QueryExecModeSimpleProtocol` interpolates arguments client-side, quoted and escaped (conn.go: "QueryExecModeSimpleProtocol uses client side parameter interpolation. All values are quoted and escaped.").
- `pgxpool.Pool` has `Query`, `BeginTx(ctx, pgx.TxOptions)`, `CopyFrom(ctx, pgx.Identifier, []string, pgx.CopyFromSource)`, `SendBatch(ctx, *pgx.Batch)`, `Acquire`, `Stat() *pgxpool.Stat` with `AcquiredConns, IdleConns, TotalConns, MaxConns, AcquireCount, EmptyAcquireCount, EmptyAcquireWaitTime`.
- pgx transaction constants: `pgx.Serializable`, `pgx.RepeatableRead`, `pgx.ReadUncommitted` (`TxIsoLevel`); `pgx.ReadWrite`, `pgx.ReadOnly` (`TxAccessMode`). pgx stdlib exposes `OpenDBFromPool`; it has no `AcquireConn` at this version, so `FromSQLDB` pins through `database/sql`'s `*sql.Conn`.
- `*pgconn.PgError` fields `Code, Message, Detail, ConstraintName, TableName, ColumnName`. `PgError.Error()` returns `Severity + ": " + Message + " (SQLSTATE " + Code + ")"` and does not include `Detail`.
- mattn `sqlite3.Error` is a struct with a value receiver: `Code ErrNo`, `ExtendedCode ErrNoExtended`. Constants used: `ErrBusy`(5), `ErrLocked`(6), `ErrConstraintCheck`(275), `ErrConstraintForeignKey`(787), `ErrConstraintNotNull`(1299), `ErrConstraintPrimaryKey`(1555), `ErrConstraintUnique`(2067). mattn DSN keys: `_busy_timeout`, `_journal_mode`, `_synchronous`, `_foreign_keys`, `_txlock` (`deferred|immediate|exclusive`). `sqlite3.SQLiteDriver{ConnectHook func(*SQLiteConn) error}` is the per-connection hook. Load-extension support is compiled in unless the build tag `sqlite_omit_load_extension` is set (which adds `-DSQLITE_OMIT_LOAD_EXTENSION`).
- sqlite-vec `cgo.Auto()` calls `sqlite3_auto_extension` (process-wide, affects every connection opened later). `Cancel()` undoes it. The README says it does not work with modernc.
- modernc `*sqlite.Error` has a pointer receiver and `Code() int`. DSN params include `_txlock` and repeated `_pragma=name(value)`.

Open items that this plan does not resolve (recorded in the final report): PostgreSQL MERGE and MERGE-RETURNING version floors (the docs page fetched does not state them); the `testdb` return type.

## Task 1: testopen package with Postgres and Each (milestone M1)

Files:

- Create: `/Volumes/DATA/Development/momo/internal/testdb/testopen/testopen.go`
- Create: `/Volumes/DATA/Development/momo/internal/testdb/testopen/testopen_test.go`

`internal/testdb` (momo-01) returns connection strings only. `testopen` imports the drivers, so it is the one place that turns a URL into a `*momo.DB`. `testopen.Postgres(t)` builds its pool from `testdb.PostgresURL(t)`, so it inherits that test's schema (the `search_path` runtime parameter), and it never creates or drops schemas itself; a raw `testdb.PostgresURL(t)` call in the same test shares the schema. `SQLite(t)` opens `testdb.SQLitePath(t)`, and `MySQL84(t)`, `MySQL97(t)` and `MariaDB(t)` open `testdb.MySQL84URL(t)`, `testdb.MySQL97URL(t)` and `testdb.MariaDBURL(t)`, so they inherit the per-test database and never create or drop databases themselves; driver packages import `testdb`, and `testdb` never imports a driver, which keeps the import graph acyclic.

Interfaces (package `testopen`, exact):

```go
// Postgres opens a fresh *momo.DB on testdb.PostgresURL(t) through pgxdriver. opts are passed to
// momo.New, so options such as momo.WithHook and momo.WithArgLogging reach the database. The pool
// is closed by t.Cleanup. It fails the test when the URL is unset and MOMO_REQUIRE_INTEGRATION=1,
// and skips it otherwise (testdb does the check).
func Postgres(t *testing.T, opts ...momo.Option) *momo.DB

// Each runs fn once per dialect whose opener is registered in this build, as a subtest named
// after the dialect, each on a freshly opened database. Each passes opts to every opener.
func Each(t *testing.T, fn func(t *testing.T, db *momo.DB, d dialect.Name), opts ...momo.Option)

// openers is the build-time list of dialects. Each driver milestone appends its entry:
// M1 registers PostgreSQL (this task); M2 registers SQLite (task S0); M3 registers MySQL84,
// MySQL97 and MariaDB (momo-08). The list is not mutable at runtime.
var openers = []opener{{dialect.Postgres, Postgres}}
```

`Each` does not run subtests in parallel; each subtest owns its database.

- [ ] **Step 1: Write the failing test** `testopen_test.go`:

```go
package testopen

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/dialect"
)

func TestEachRunsPostgres(t *testing.T) {
    var ran []dialect.Name
    Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        ran = append(ran, d)
        rows, err := db.Driver().Query(context.Background(), "SELECT 1", nil)
        if err != nil {
            t.Fatal(err)
        }
        if err := rows.Close(); err != nil {
            t.Fatal(err)
        }
    })
    if len(ran) != len(openers) {
        t.Fatalf("Each ran %d dialects, registered %d", len(ran), len(openers))
    }
    if len(ran) == 0 || ran[0] != dialect.Postgres {
        t.Fatalf("Each ran %v, want postgres first", ran)
    }
}
```

Options test, same file:

```go
// countHook counts statements reaching the hook.
type countHook struct{ n int }

func (c *countHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
    c.n++
    return ctx
}

func (c *countHook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {}

func TestOptionsReachDB(t *testing.T) {
    hook := &countHook{}
    db := Postgres(t, momo.WithHook(hook))
    if _, err := momo.Exec(context.Background(), db, "SELECT 1"); err != nil {
        t.Fatal(err)
    }
    if hook.n != 1 {
        t.Fatalf("WithHook passed to Postgres did not reach the database: %d events", hook.n)
    }
}
```

Each-options test, same file:

```go
func TestEachPassesOptions(t *testing.T) {
    hook := &countHook{}
    ran := 0
    Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        ran++
        if _, err := momo.Exec(context.Background(), db, "SELECT 1"); err != nil {
            t.Fatal(err)
        }
    }, momo.WithHook(hook))
    if ran != len(openers) || hook.n != len(openers) {
        t.Fatalf("ran %d dialects, hook saw %d statements, want %d of each", ran, hook.n, len(openers))
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `MOMO_TEST_POSTGRES_URL=... go test ./internal/testdb/testopen/ -run 'TestEachRunsPostgres|TestOptionsReachDB|TestEachPassesOptions'` → `undefined: Each` and `too many arguments in call to Postgres` (and `too many arguments in call to Each` once Each exists without options). → `undefined: Each` and `too many arguments in call to Postgres`.

- [ ] **Step 3: Implement** `testopen.go`: `opener` struct (`name dialect.Name`, `open func(*testing.T, ...momo.Option) *momo.DB`), `Postgres` (`pgxpool.New(ctx, testdb.PostgresURL(t))`, so the pool inherits the per-test schema; it creates no schema itself; `t.Cleanup(pool.Close)`, `momo.New(pgxdriver.New(pool), opts...)`), `Each` (loops over `openers`, `t.Run(string(o.name), ...)`, calls `o.open(t, opts...)`), and the registry above.

- [ ] **Step 4: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test ./internal/testdb/testopen/ -race` → `ok`.

- [ ] **Step 5: Commit.** `feat(testopen): PostgreSQL opener and dialect-parameterized Each`

## Task 2: Optional driver capability interfaces in the root package

Files:

- Create: `/Volumes/DATA/Development/momo/driver_optional.go`
- Create: `/Volumes/DATA/Development/momo/driver_optional_test.go`

Interfaces (root package `momo`, exact):

```go
// PinnedConn is one database connection checked out of a driver's pool.
// Statements on it run on the same session. Begin starts a transaction on that
// session; Release returns the connection to the pool.
type PinnedConn interface {
    Query(ctx context.Context, sql string, args []any) (Rows, error)
    Exec(ctx context.Context, sql string, args []any) (int64, error)
    Begin(ctx context.Context, opts TxOptions) (DriverTx, error)
    Release() error
}

// Pinner is implemented by drivers that can pin one connection.
type Pinner interface {
    Pin(ctx context.Context) (PinnedConn, error)
}

// Copier is implemented by drivers with a native bulk-copy path.
type Copier interface {
    CopyFrom(ctx context.Context, table string, cols []string, rows [][]any) (int64, error)
}

// Batcher is implemented by drivers that send several statements in one round trip.
// Every statement is executed with Query; a statement without result rows yields
// a Rows whose Next returns false.
type Batcher interface {
    SendBatch(ctx context.Context, stmts []Compiled) ([]Rows, error)
}

// PoolStats is a snapshot of a driver's connection pool.
type PoolStats struct {
    MaxConns            int
    TotalConns          int
    AcquiredConns       int
    IdleConns           int
    AcquireCount        int64
    EmptyAcquireCount   int64
    EmptyAcquireWaitNS  int64
}

// PoolStater is implemented by drivers that expose pool statistics.
type PoolStater interface {
    PoolStats() PoolStats
}

// ServerVersioner is implemented by drivers that report the server version
// (PostgreSQL server_version_num, e.g. 170002). MERGE gates read it.
type ServerVersioner interface {
    ServerVersion(ctx context.Context) (int, error)
}

// Capabilities describes what a driver can execute beyond the dialect.
type Capabilities struct {
    Vectors bool // sqlite-vec or pgvector functions are available
    Batch   bool
    Copy    bool
}

// CapabilityReporter is implemented by drivers that report driver-level capabilities.
// Vector builders check Vectors and return ErrUnsupported when it is false.
type CapabilityReporter interface {
    Capabilities() Capabilities
}
```

- [ ] **Step 1: Write the failing test.** Create `driver_optional_test.go`:

```go
package momo

import "testing"

type fakeDriver struct{ Driver }

func (fakeDriver) PoolStats() PoolStats { return PoolStats{MaxConns: 4, AcquiredConns: 1} }
func (fakeDriver) Capabilities() Capabilities { return Capabilities{Vectors: true} }

func TestOptionalInterfacesAreSatisfiedByAdapters(t *testing.T) {
    var d any = fakeDriver{}
    if _, ok := d.(PoolStater); !ok {
        t.Fatal("fakeDriver must satisfy PoolStater")
    }
    if got := d.(CapabilityReporter).Capabilities(); !got.Vectors {
        t.Fatalf("Capabilities().Vectors = false, want true")
    }
    if got := d.(PoolStater).PoolStats().MaxConns; got != 4 {
        t.Fatalf("MaxConns = %d, want 4", got)
    }
}
```

- [ ] **Step 2: Run it and expect FAIL.** Command: `cd /Volumes/DATA/Development/momo && go test -run TestOptionalInterfacesAreSatisfiedByAdapters ./...` Expected: `undefined: PoolStater` (build failure before any test runs).

- [ ] **Step 3: Implement.** Create `driver_optional.go` containing exactly the types listed under Interfaces, with doc comments. Only the `context`, `Rows`, `Compiled` identifiers are used from the same package; no new imports.

- [ ] **Step 4: Run and expect PASS.** `go test -run TestOptionalInterfacesAreSatisfiedByAdapters -race ./...` → `ok`.

- [ ] **Step 5: Commit.** `git add driver_optional.go driver_optional_test.go && git commit -m "feat(driver): add optional driver capability interfaces"`

## Task 3: Error classification for PostgreSQL (pgx) with value redaction at the source

Files:

- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/classify.go`
- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/classify_test.go`

Interfaces:

```go
package pgxdriver

// ClassifyError maps *pgconn.PgError (found with errors.As) to *momo.Error.
func ClassifyError(err error) error
```

SQLSTATE mapping: `23505` → `ErrUniqueViolation`; `23503` → `ErrForeignKeyViolation`; `23514` → `ErrCheckViolation`; `23502` → `ErrNotNullViolation`; `40001` and `40P01` → `ErrSerialization`; `55P03` (lock not available, NOWAIT) → `ErrBusy`. `Table` = `TableName`, `Column` = `ColumnName`, `Constraint` = `ConstraintName`. `Err` is `driverError{cause: pgErr, msg: pgErr.Message}`: the wrapped message is the server message only, and `Detail` (which contains key values, e.g. `Key (email)=(a@b.c)`) is never copied into any string; it stays on the cause for `errors.As`.

- [ ] **Step 1: Write the failing test** `classify_test.go`:

```go
package pgxdriver

import (
    "errors"
    "strings"
    "testing"

    "github.com/jackc/pgx/v5/pgconn"
    "github.com/azrtydxb/momo"
)

func TestClassifyUniqueRedactsDetail(t *testing.T) {
    pe := &pgconn.PgError{
        Code:           "23505",
        Message:        `duplicate key value violates unique constraint "users_email_key"`,
        Detail:         "Key (email)=(secret@example.com) already exists.",
        TableName:      "users",
        ColumnName:     "email",
        ConstraintName: "users_email_key",
    }
    got := ClassifyError(pe)
    if !errors.Is(got, momo.ErrUniqueViolation) {
        t.Fatalf("not classified as unique violation: %v", got)
    }
    if strings.Contains(got.Error(), "secret@example.com") {
        t.Fatalf("Error() leaked the offending value: %q", got.Error())
    }
    var back *pgconn.PgError
    if !errors.As(got, &back) || back.Detail == "" {
        t.Fatalf("Detail not reachable through errors.As")
    }
    var me *momo.Error
    if !errors.As(got, &me) || me.Table != "users" || me.Constraint != "users_email_key" {
        t.Fatalf("structured fields missing: %+v", me)
    }
}

func TestClassifySerialization(t *testing.T) {
    if got := ClassifyError(&pgconn.PgError{Code: "40P01"}); !errors.Is(got, momo.ErrSerialization) {
        t.Fatalf("deadlock not ErrSerialization: %v", got)
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./driver/pgxdriver/ -run TestClassify` → `undefined: ClassifyError`.

- [ ] **Step 3: Implement** `classify.go` with a `switch pe.Code` per the mapping. Build the message as `pe.Message` only.

- [ ] **Step 4: Run and expect PASS.** `go test ./driver/pgxdriver/ -run TestClassify -race` → `ok`.

- [ ] **Step 5: Commit.** `feat(pgx): classify PostgreSQL errors without leaking detail`

## Task 4: pgx adapter core (pgxpool) with per-call exec mode

Files:

- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/driver.go`
- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/driver_test.go`
- Modify: `/Volumes/DATA/Development/momo/go.mod` (via `go get`)

Interfaces:

```go
type Option func(*driver)

// WithExecMode sets the pgx query execution mode for every statement and transaction this driver sends.
// Accepted: pgx.QueryExecModeExec (default: unnamed statements, server-side binding, PgBouncer-safe),
// pgx.QueryExecModeCacheStatement, pgx.QueryExecModeCacheDescribe, pgx.QueryExecModeDescribeExec.
// pgx.QueryExecModeSimpleProtocol is rejected: it interpolates arguments client-side, which breaks the
// constraint "values always bound". A rejected mode makes every Query, Exec and Begin return ErrUnsupported.
func WithExecMode(m pgx.QueryExecMode) Option

// New wraps a pgxpool. The caller owns the pool and closes it.
func New(pool *pgxpool.Pool, opts ...Option) momo.Driver
```

The driver type implements `momo.Driver` (`Dialect`, `Query`, `Exec`, `Begin`) and `ClassifyError` (Task 3). `Capabilities()` returns `momo.Capabilities{Vectors: true}` (the pgxdriver can execute vector SQL; momo-07 checks the pgvector extension itself). Each Query/Exec prepends the configured mode as the first argument (`pool.Query(ctx, sql, mode, args...)`). Dialect is `dialect.For(dialect.Postgres)`.

- [ ] **Step 1: Add dependencies.** `cd /Volumes/DATA/Development/momo && go get github.com/jackc/pgx/v5@v5.11.0 github.com/jackc/pgx/v5/pgxpool@v5.11.0 github.com/jackc/pgx/v5/stdlib@v5.11.0`. Expected: go.mod lists `github.com/jackc/pgx/v5 v5.11.0`.

- [ ] **Step 2: Write the failing tests** `driver_test.go`:

```go
package pgxdriver

import (
    "context"
    "errors"
    "testing"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/internal/testdb"
)

func openPool(t *testing.T) *pgxpool.Pool {
    t.Helper()
    pool, err := pgxpool.New(context.Background(), testdb.PostgresURL(t))
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(pool.Close)
    return pool
}

func TestExecModeExecIsApplied(t *testing.T) {
    d := New(openPool(t), WithExecMode(pgx.QueryExecModeExec))
    rows, err := d.Query(context.Background(), "SELECT $1::text", []any{"a'b"})
    if err != nil {
        t.Fatal(err)
    }
    defer rows.Close()
    var got string
    if !rows.Next() {
        t.Fatal("no row")
    }
    if err := rows.Scan(&got); err != nil {
        t.Fatal(err)
    }
    if got != "a'b" {
        t.Fatalf("got %q", got)
    }
}

func TestSimpleProtocolRejected(t *testing.T) {
    d := New(openPool(t), WithExecMode(pgx.QueryExecModeSimpleProtocol))
    if _, err := d.Query(context.Background(), "SELECT $1::int", []any{1}); !errors.Is(err, momo.ErrUnsupported) {
        t.Fatalf("simple protocol err = %v, want ErrUnsupported", err)
    }
}

func TestDefaultExecModeNoNamedStatements(t *testing.T) {
    d := New(openPool(t))
    ctx := context.Background()
    p, err := d.(momo.Pinner).Pin(ctx)
    if err != nil {
        t.Fatal(err)
    }
    defer p.Release()
    rows, err := p.Query(ctx, "SELECT $1::int", []any{1})
    if err != nil {
        t.Fatal(err)
    }
    rows.Close()
    rows, err = p.Query(ctx, "SELECT count(*) FROM pg_prepared_statements", nil)
    if err != nil {
        t.Fatal(err)
    }
    defer rows.Close()
    rows.Next()
    var n int
    if err := rows.Scan(&n); err != nil {
        t.Fatal(err)
    }
    if n != 0 {
        t.Fatalf("default mode left %d named statements; PgBouncer transaction pooling would break", n)
    }
}
```

- [ ] **Step 3: Run and expect FAIL.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run 'ExecMode|SimpleProtocol|DefaultExecMode'` → `undefined: New`.

- [ ] **Step 4: Implement** `driver.go`: `mode` defaults to `pgx.QueryExecModeExec`; `WithExecMode` sets `mode`, and for SimpleProtocol sets `modeErr = momo.ErrUnsupported` wrapped in `*momo.Error{Kind: momo.ErrUnsupported, Op: "build", Err: errors.New("simple protocol interpolates arguments client-side")}`. `Query`, `Exec` and `Begin` return `modeErr` first when set. The mode prepend is `args = append([]any{d.mode}, args...)`. `Query` returns `pgxRows{pgx.Rows}`, a wrapper whose `Close() error` calls pgx's `Close()` and returns nil: `pgx.Rows.Close()` has no return value, and `momo.Rows` requires `Close() error`, so pgx rows do not satisfy `momo.Rows` directly.

- [ ] **Step 5: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run 'ExecMode|SimpleProtocol|DefaultExecMode' -race` → `ok`.

- [ ] **Step 6: Commit.** `feat(pgx): pgxpool driver, exec mode defaults to PgBouncer-safe Exec`

## Task 5: Pool acquire tracer feeding momo.RecordPoolWait

Files:

- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/tracer.go`
- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/tracer_test.go` (package `pgxdriver_test`)
- Modify: `/Volumes/DATA/Development/momo/internal/testdb/testopen/testopen.go` (`Postgres` builds its pool with `NewPool`)

Dependency: momo-09 defines `func RecordPoolWait(ctx context.Context, d time.Duration)` and `QueryEvent.PoolWait`. This task calls the former and its test reads the latter, so it lands after momo-09's recorder task. Until then it does not compile.

Interfaces:

```go
// InstrumentPool installs the pool acquire tracer on cfg. It is set as cfg.ConnConfig.Tracer and
// delegates pgx query tracing to any tracer that was set before. Call it before pgxpool.NewWithConfig.
func InstrumentPool(cfg *pgxpool.Config)

// NewPool parses url, installs the tracer with InstrumentPool and opens the pool.
func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error)
```

Behaviour (pgxpool v5.11.0 selects an acquire tracer from `ConnConfig.Tracer` when it implements `pgxpool.AcquireTracer`, and passes to `TraceAcquireEnd` the context that `TraceAcquireStart` returned): `TraceAcquireStart` returns the context with the current time stored under an unexported key. `TraceAcquireEnd` reads that time and calls `momo.RecordPoolWait(ctx, time.Since(start))`, so the wait is attributed to the statement whose context is being acquired for. An acquire that finds an idle connection records nothing, so `PoolWait == 0` means no wait; the one exception is a concurrent idle hit, which may record its own small, real elapsed time (see the comment in `tracer.go`).

- [ ] **Step 1: Write the failing test** `tracer_test.go`:

```go
package pgxdriver_test

import (
    "context"
    "sync/atomic"
    "testing"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/driver/pgxdriver"
    "github.com/azrtydxb/momo/internal/testdb"
)

// poolWaitHook keeps the largest PoolWait it sees.
type poolWaitHook struct{ maxNS atomic.Int64 }

func (h *poolWaitHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
    return ctx
}

func (h *poolWaitHook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
    for {
        cur := h.maxNS.Load()
        if int64(e.PoolWait) <= cur || h.maxNS.CompareAndSwap(cur, int64(e.PoolWait)) {
            return
        }
    }
}

func TestPoolWaitRecordedWhenExhausted(t *testing.T) {
    ctx := context.Background()
    cfg, err := pgxpool.ParseConfig(testdb.PostgresURL(t))
    if err != nil {
        t.Fatal(err)
    }
    cfg.MaxConns = 1
    pgxdriver.InstrumentPool(cfg)
    pool, err := pgxpool.NewWithConfig(ctx, cfg)
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(pool.Close)
    d := pgxdriver.New(pool)
    hook := &poolWaitHook{}
    db := momo.New(d, momo.WithHook(hook))
    pc, err := d.(momo.Pinner).Pin(ctx) // holds the only connection
    if err != nil {
        t.Fatal(err)
    }
    done := make(chan error, 1)
    go func() {
        _, err := momo.Exec(ctx, db, "SELECT 1") // the second query now waits for the pool
        done <- err
    }()
    time.Sleep(100 * time.Millisecond)
    if err := pc.Release(); err != nil {
        t.Fatal(err)
    }
    if err := <-done; err != nil {
        t.Fatal(err)
    }
    if got := time.Duration(hook.maxNS.Load()); got < 50*time.Millisecond {
        t.Fatalf("PoolWait = %v, want the blocked query to report its wait of about 100ms", got)
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run TestPoolWaitRecordedWhenExhausted` → `undefined: pgxdriver.InstrumentPool`.

- [ ] **Step 3: Mutation check after Step 4.** Remove the `InstrumentPool` call from the test, run again. Expected FAIL `PoolWait = 0s, want the blocked query to report its wait of about 100ms`. Restore.

- [ ] **Step 4: Implement** `tracer.go`:

```go
type acquireStartKey struct{}

type poolTracer struct{ next pgx.QueryTracer }

func (p *poolTracer) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
    return context.WithValue(ctx, acquireStartKey{}, time.Now())
}

func (p *poolTracer) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireEndData) {
    if start, ok := ctx.Value(acquireStartKey{}).(time.Time); ok {
        momo.RecordPoolWait(ctx, time.Since(start))
    }
}

func (p *poolTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
    if p.next != nil {
        return p.next.TraceQueryStart(ctx, conn, data)
    }
    return ctx
}

func (p *poolTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
    if p.next != nil {
        p.next.TraceQueryEnd(ctx, conn, data)
    }
}

func InstrumentPool(cfg *pgxpool.Config) {
    cfg.ConnConfig.Tracer = &poolTracer{next: cfg.ConnConfig.Tracer}
}

func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
    cfg, err := pgxpool.ParseConfig(url)
    if err != nil {
        return nil, err
    }
    InstrumentPool(cfg)
    return pgxpool.NewWithConfig(ctx, cfg)
}
```

Change `testopen.Postgres` to `pool, err := pgxdriver.NewPool(ctx, testdb.PostgresURL(t))`, keeping its `t.Cleanup(pool.Close)`.

- [ ] **Step 5: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run TestPoolWaitRecordedWhenExhausted -race -count=3` → `ok` three times.

- [ ] **Step 6: Commit.** `feat(pgx): pool acquire tracer reports pool wait to momo.RecordPoolWait`

## Task 6: Transactions with isolation, read-only access and savepoints on PostgreSQL

Files:

- Modify: `/Volumes/DATA/Development/momo/driver/pgxdriver/driver.go`
- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/tx_test.go`

Interfaces: `func (d *driver) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error)`.

Mapping `sql.IsolationLevel` → `pgx.TxIsoLevel`: `LevelDefault` → empty (server default); `LevelReadUncommitted` → `pgx.ReadUncommitted`; `LevelReadCommitted` → `pgx.ReadCommitted`; `LevelRepeatableRead` → `pgx.RepeatableRead`; `LevelSerializable` → `pgx.Serializable`; any other value → `momo.ErrUnsupported`. `ReadOnly` → `pgx.ReadOnly`, else `pgx.ReadWrite`. Savepoints are issued by the root `Tx.Savepoint` through `Exec`, so no extra code is needed here.

- [ ] **Step 1: Write the failing test** `tx_test.go`:

```go
package pgxdriver

import (
    "context"
    "database/sql"
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestTransactionsIsolationReachesServer(t *testing.T) {
    d := New(openPool(t))
    tx, err := d.Begin(context.Background(), momo.TxOptions{Isolation: sql.LevelRepeatableRead})
    if err != nil {
        t.Fatal(err)
    }
    defer tx.Rollback(context.Background())
    rows, err := tx.Query(context.Background(), "SHOW transaction_isolation", nil)
    if err != nil {
        t.Fatal(err)
    }
    defer rows.Close()
    rows.Next()
    var lvl string
    if err := rows.Scan(&lvl); err != nil {
        t.Fatal(err)
    }
    if lvl != "repeatable read" {
        t.Fatalf("transaction_isolation = %q", lvl)
    }
}

func TestUnknownIsolationIsUnsupported(t *testing.T) {
    d := New(openPool(t))
    _, err := d.Begin(context.Background(), momo.TxOptions{Isolation: sql.IsolationLevel(99)})
    if !errors.Is(err, momo.ErrUnsupported) {
        t.Fatalf("err = %v, want ErrUnsupported", err)
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run 'Transactions|UnknownIsolation'` → the first test fails with `transaction_isolation = "read committed"` only if Begin ignores the level; before implementation Begin is absent, so the expected failure is `driver.Begin undefined` at compile time.

- [ ] **Step 3: Implement** `Begin`: call `d.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: iso, AccessMode: mode})` and wrap the returned `pgx.Tx` in a `dtx` struct implementing `momo.DriverTx` (`Query`, `Exec`, `Commit`, `Rollback`) that forwards to the pgx transaction, applying the same exec-mode prepend as Task 4.

- [ ] **Step 4: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run 'Transactions|UnknownIsolation' -race` → `ok`.

- [ ] **Step 5: Commit.** `feat(pgx): map transaction isolation and access mode`

## Task 7: Pinned connections and pool statistics on PostgreSQL

Files:

- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/pin.go`
- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/pin_test.go`
- Create: `/Volumes/DATA/Development/momo/onconn.go` (package `momo`, root)
- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/onconn_test.go` (package `pgxdriver_test`)
- Modify: `/Volumes/DATA/Development/momo/db.go` (`Begin` guard)

Interfaces:

```go
// Pin acquires one pool connection; statements through the returned PinnedConn share its session.
func (d *driver) Pin(ctx context.Context) (momo.PinnedConn, error)

// OnConn (root package momo, onconn.go) returns a handle whose statements all run on pc, a connection
// pinned with Pin. It is a copy of db: it keeps db's hooks, error classifier and options (WithHook,
// WithArgLogging, WithClock, WithInterceptor, WithFieldHooks, ...), and drops db's replicas. Begin on
// it returns ErrUnsupported with Op "begin": a pinned connection is not a pool, so transactions are
// not started on it. Non-transactional Go migrations (momo-04, momo-11) use it.
func (db *DB) OnConn(pc PinnedConn) *DB

// pinned.Query returns the pgxRows wrapper from Task 4, so every Rows satisfies momo.Rows.
// PoolStats reports pgxpool statistics.
func (d *driver) PoolStats() momo.PoolStats
```

`pinned` wraps `*pgxpool.Conn`. `Begin` starts a pgx transaction on that connection through the same `DriverTx` wrapper the pool driver uses (Task 6), so a transaction on a pinned connection ends on the same session. `Release` calls the pool release once; a second call returns nil. `PoolStats` copies `pool.Stat()` into `momo.PoolStats`, with `EmptyAcquireWaitNS = stat.EmptyAcquireWaitTime().Nanoseconds()`.

- [ ] **Step 1: Write the failing test** `pin_test.go`:

```go
package pgxdriver

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestPinnedConn(t *testing.T) {
    pool := openPool(t)
    d := New(pool)
    ctx := context.Background()
    p, err := d.(momo.Pinner).Pin(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if _, err := p.Exec(ctx, "SET application_name = 'momo-pin'", nil); err != nil {
        t.Fatal(err)
    }
    rows, err := p.Query(ctx, "SHOW application_name", nil)
    if err != nil {
        t.Fatal(err)
    }
    rows.Next()
    var name string
    if err := rows.Scan(&name); err != nil {
        t.Fatal(err)
    }
    rows.Close()
    if name != "momo-pin" {
        t.Fatalf("session state lost: application_name = %q", name)
    }
    if st := d.(momo.PoolStater).PoolStats(); st.AcquiredConns < 1 {
        t.Fatalf("AcquiredConns = %d while pinned", st.AcquiredConns)
    }
    tx, err := p.Begin(ctx, momo.TxOptions{})
    if err != nil {
        t.Fatal(err)
    }
    if _, err := tx.Exec(ctx, "SELECT 1", nil); err != nil {
        t.Fatal(err)
    }
    if err := tx.Rollback(ctx); err != nil {
        t.Fatal(err)
    }
    if err := p.Release(); err != nil {
        t.Fatal(err)
    }
    if st := d.(momo.PoolStater).PoolStats(); st.AcquiredConns != 0 {
        t.Fatalf("AcquiredConns = %d after release", st.AcquiredConns)
    }
}
```

OnConn test, `driver/pgxdriver/onconn_test.go` (external test package, so it can import `testopen`, which imports this package):

```go
package pgxdriver_test

import (
    "context"
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/internal/testdb/testopen"
)

type countHook struct{ n int }

func (c *countHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
    c.n++
    return ctx
}

func (c *countHook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {}

func TestOnConnSessionAndHooks(t *testing.T) {
    hook := &countHook{}
    db := testopen.Postgres(t, momo.WithHook(hook))
    ctx := context.Background()
    pc, err := db.Driver().(momo.Pinner).Pin(ctx)
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() {
        if err := pc.Release(); err != nil {
            t.Error(err)
        }
    })
    oc := db.OnConn(pc)
    if _, err := momo.Exec(ctx, oc, "SET application_name = 'momo-onconn'"); err != nil {
        t.Fatal(err)
    }
    rows, err := pc.Query(ctx, "SHOW application_name", nil)
    if err != nil {
        t.Fatal(err)
    }
    if !rows.Next() {
        t.Fatalf("SHOW returned no row: %v", rows.Err())
    }
    var name string
    if err := rows.Scan(&name); err != nil {
        t.Fatal(err)
    }
    if err := rows.Close(); err != nil {
        t.Fatal(err)
    }
    if name != "momo-onconn" {
        t.Fatalf("statement through OnConn did not set session state on the pinned connection: %q", name)
    }
    before := hook.n
    if _, err := momo.Exec(ctx, oc, "SELECT 1"); err != nil {
        t.Fatal(err)
    }
    if hook.n != before+1 {
        t.Fatalf("hook registered on db fired %d times for an OnConn statement, want 1", hook.n-before)
    }
    if _, err := oc.Begin(ctx, momo.TxOptions{}); !errors.Is(err, momo.ErrUnsupported) {
        t.Fatalf("Begin on a pinned handle err = %v, want ErrUnsupported", err)
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run TestPinnedConn` → `panic: interface conversion: *pgxdriver.driver is not momo.Pinner: missing method Pin`; `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run TestOnConnSessionAndHooks` → `db.OnConn undefined`.

- [ ] **Step 3: Implement** `pin.go` per Interfaces, `onconn.go` in the root (`func (db *DB) OnConn(pc PinnedConn) *DB`: `nd := new(DB); *nd = *db; nd.ex = execer{db: nd, q: pc, d: db.ex.d, classify: db.ex.classify}; nd.replicas = nil; return nd`), and the `db.go` guard at the start of `Begin`: `if _, ok := db.ex.q.(Driver); !ok { return nil, &Error{Kind: ErrUnsupported, Op: "begin", Err: errors.New("transactions are not available on a pinned connection")} }`. `OnConn` copies every configuration field, so hooks, options and classifier carry over without listing them.

- [ ] **Step 4: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run TestPinnedConn -race` → `ok`.

- [ ] **Step 5: Commit.** `feat(pgx): pinned connections and pool stats`

## Task 8: COPY and batch paths on PostgreSQL

Files:

- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/bulk.go`
- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/bulk_test.go`
- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/bulk_tx_test.go` (package `pgxdriver_test`)
- Create: `/Volumes/DATA/Development/momo/driver_access.go` (package `momo`, root)
- Modify: `/Volumes/DATA/Development/momo/driver/pgxdriver/driver.go` (the transaction wrapper `dtx` gains `CopyFrom` and `SendBatch`)

Interfaces:

```go
// CopyFrom implements momo.Copier with pgx's COPY protocol.
func (d *driver) CopyFrom(ctx context.Context, table string, cols []string, rows [][]any) (int64, error)

// SendBatch implements momo.Batcher: all statements are queued into one pgx.Batch.
func (d *driver) SendBatch(ctx context.Context, stmts []momo.Compiled) ([]momo.Rows, error)
```

Transactions are Copiers and Batchers too: the transaction wrapper `dtx` forwards `CopyFrom` to `pgx.Tx.CopyFrom` and `SendBatch` to `pgx.Tx.SendBatch`, so work inside `db.Tx` uses the same bulk paths as work on the pool. The root package finds the right object for any handle:

```go
// CopierOf returns the Copier behind q: the driver for a *DB, the transaction for a *Tx.
func CopierOf(q Querier) (Copier, bool)

// BatcherOf returns the Batcher behind q: the driver for a *DB, the transaction for a *Tx.
func BatcherOf(q Querier) (Batcher, bool)
```

`driver_access.go` implements both with a type switch on `*DB` (`db.ex.q`) and `*Tx` (`tx.dtx`), so that momo-05's COPY insert works inside a transaction.

**Batch buffering and exec mode (review 063).** `SendBatch` buffers every statement result eagerly and without bound before returning (documented in ARCHITECTURE). It honours the driver exec mode: under the default mode it sends unnamed extended-protocol statements through pgconn (no prepared statements, PgBouncer-safe), because `pgx.Batch` always uses the connection default mode.

`CopyFrom` splits `table` on `.` into `pgx.Identifier` parts (schema, table) so schema-qualified tables work. `SendBatch` returns one `momo.Rows` per statement: `pgx.BatchResults.Query()` for each, wrapped in `pgxRows` (Task 4); a statement that returns no rows yields a `Rows` whose `Next` is false and whose `Close` drains the result.

- [ ] **Step 1: Write the failing test** `bulk_test.go`:

```go
package pgxdriver

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestCopyInsertMatchesRows(t *testing.T) {
    d := New(openPool(t)).(momo.Copier)
    ctx := context.Background()
    if _, err := d.(momo.Driver).Exec(ctx, "CREATE TEMP TABLE t_copy (id bigint, name text)", nil); err != nil {
        t.Fatal(err)
    }
    n, err := d.CopyFrom(ctx, "t_copy", []string{"id", "name"}, [][]any{{int64(1), "a"}, {int64(2), "b"}})
    if err != nil {
        t.Fatal(err)
    }
    if n != 2 {
        t.Fatalf("copied %d, want 2", n)
    }
}

func TestBatchRunsAllStatementsInOneRoundTrip(t *testing.T) {
    d := New(openPool(t)).(momo.Batcher)
    out, err := d.SendBatch(context.Background(), []momo.Compiled{
        {SQL: "SELECT 1", Args: nil},
        {SQL: "SELECT $1::int", Args: []any{2}},
    })
    if err != nil {
        t.Fatal(err)
    }
    if len(out) != 2 {
        t.Fatalf("got %d result sets, want 2", len(out))
    }
}
```

Transaction test, `driver/pgxdriver/bulk_tx_test.go`:

```go
package pgxdriver_test

import (
    "context"
    "errors"
    "fmt"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/internal/testdb/testopen"
)

var errRollback = errors.New("roll back on purpose")

func TestCopyAndBatchInTransaction(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    if _, err := momo.Exec(ctx, db, "CREATE TABLE tx_copy (id bigint, name text)"); err != nil {
        t.Fatal(err)
    }
    err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
        c, ok := momo.CopierOf(tx)
        if !ok {
            return errors.New("transaction is not a Copier")
        }
        n, err := c.CopyFrom(ctx, "tx_copy", []string{"id", "name"}, [][]any{{int64(1), "a"}, {int64(2), "b"}})
        if err != nil {
            return err
        }
        if n != 2 {
            return fmt.Errorf("copied %d rows, want 2", n)
        }
        b, ok := momo.BatcherOf(tx)
        if !ok {
            return errors.New("transaction is not a Batcher")
        }
        rs, err := b.SendBatch(ctx, []momo.Compiled{{SQL: "SELECT count(*) FROM tx_copy"}})
        if err != nil {
            return err
        }
        if len(rs) != 1 {
            return fmt.Errorf("batch returned %d result sets, want 1", len(rs))
        }
        if !rs[0].Next() {
            return errors.New("batch count returned no row")
        }
        var inTx int
        if err := rs[0].Scan(&inTx); err != nil {
            return err
        }
        if err := rs[0].Close(); err != nil {
            return err
        }
        if inTx != 2 {
            return fmt.Errorf("batch inside the transaction saw %d rows, want 2", inTx)
        }
        return errRollback
    })
    if !errors.Is(err, errRollback) {
        t.Fatalf("Tx err = %v, want the rollback sentinel", err)
    }
    rows, err := db.Driver().Query(ctx, "SELECT count(*) FROM tx_copy", nil)
    if err != nil {
        t.Fatal(err)
    }
    if !rows.Next() {
        t.Fatalf("count returned no row: %v", rows.Err())
    }
    var after int
    if err := rows.Scan(&after); err != nil {
        t.Fatal(err)
    }
    if err := rows.Close(); err != nil {
        t.Fatal(err)
    }
    if after != 0 {
        t.Fatalf("rolled-back COPY left %d rows", after)
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run 'CopyAndBatchInTransaction'` → `undefined: momo.CopierOf`. The earlier tests in this task (`CopyInsert`, `Batch`) are run in the same step: `go test ./driver/pgxdriver/ -run 'CopyInsert|Batch'`.

- [ ] **Step 2b: Run and expect FAIL for the pool path.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run 'CopyInsert|Batch'` → `interface conversion ... is not momo.Copier`.

- [ ] **Step 3: Implement** `bulk.go` as specified. Keep the `pgx.Batch` creation local to `SendBatch`; close all `BatchResults` on error. Add `CopyFrom` and `SendBatch` to `dtx` in `driver.go` (forwarding to `pgx.Tx`), and write `driver_access.go` (`CopierOf`, `BatcherOf`).

- [ ] **Step 4: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run 'CopyInsert|Batch' -race` → `ok`.

- [ ] **Step 5: Commit.** `feat(pgx): COPY and batch paths`

## Task 9: database/sql adoption: FromSQLDB and FromConnConfig (pgx stdlib)

Files:

- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/sqldb.go`
- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/sqldb_test.go`
- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/sqldb_adapter_test.go` (the S-88 acceptance test, external package `pgxdriver_test`, because it imports `testopen`, which imports `pgxdriver`)

Interfaces:

```go
// FromSQLDB adopts an existing *sql.DB opened with the pgx stdlib driver. The execution mode is the one
// set in that connection's pgx config; FromSQLDB cannot change it.
func FromSQLDB(db *sql.DB) momo.Driver

// FromConnConfig opens a *sql.DB from cfg with stdlib.OpenDB and adopts it. cfg.DefaultQueryExecMode
// is the execution mode of every statement. opts is accepted for signature parity with New; the
// database/sql path uses only error classification, so opts have no effect here.
func FromConnConfig(cfg *pgx.ConnConfig, opts ...Option) (*sql.DB, momo.Driver)
```

Pin uses `db.Conn(ctx)` wrapped as `momo.PinnedConn`. `PoolStats` maps `db.Stats()`: `OpenConnections` to TotalConns, `InUse` to AcquiredConns, `Idle` to IdleConns, `MaxOpenConnections` to MaxConns, `WaitCount` to AcquireCount, `WaitDuration` nanoseconds to EmptyAcquireWaitNS. Begin uses `db.BeginTx(ctx, &sql.TxOptions{Isolation, ReadOnly})`; the pinned connection's `Begin` uses `conn.BeginTx(ctx, &sql.TxOptions{Isolation, ReadOnly})` on the `*sql.Conn` from `Pin`, and its `Release` calls `conn.Close()` (which returns the connection to the pool). Errors pass through unwrapped (database/sql does not wrap driver errors), so `ClassifyError` from Task 3 sees `*pgconn.PgError`. The driver returned by `FromSQLDB` also implements `Capabilities()` returning `momo.Capabilities{Vectors: true}`, as `New` does, because S-88 requires vector support shared with the native adapter. It implements neither `momo.Copier` nor `momo.Batcher`; that difference is the only one allowed (see `allowedDiffs` in Step 2b).

- [ ] **Step 1: Write the failing tests** `sqldb_test.go`:

```go
package pgxdriver

import (
    "context"
    "database/sql"
    "testing"

    "github.com/jackc/pgx/v5"
    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/internal/testdb"
)

func TestFromSQLDBPinsAndCounts(t *testing.T) {
    db, err := sql.Open("pgx", testdb.PostgresURL(t))
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { db.Close() })
    d := FromSQLDB(db)
    ctx := context.Background()
    p, err := d.(momo.Pinner).Pin(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if st := d.(momo.PoolStater).PoolStats(); st.AcquiredConns != 1 {
        t.Fatalf("AcquiredConns = %d while pinned, want 1", st.AcquiredConns)
    }
    if err := p.Release(); err != nil {
        t.Fatal(err)
    }
}

func TestFromConnConfigExecMode(t *testing.T) {
    cfg, err := pgx.ParseConfig(testdb.PostgresURL(t))
    if err != nil {
        t.Fatal(err)
    }
    cfg.DefaultQueryExecMode = pgx.QueryExecModeExec
    db, drv := FromConnConfig(cfg)
    t.Cleanup(func() { db.Close() })
    ctx := context.Background()
    rows, err := drv.Query(ctx, "SELECT $1::int", []any{1})
    if err != nil {
        t.Fatal(err)
    }
    if err := rows.Close(); err != nil {
        t.Fatal(err)
    }
    conn, err := db.Conn(ctx)
    if err != nil {
        t.Fatal(err)
    }
    defer conn.Close()
    var x int
    if err := conn.QueryRowContext(ctx, "SELECT $1::int", 1).Scan(&x); err != nil {
        t.Fatal(err)
    }
    var n int
    if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM pg_prepared_statements").Scan(&n); err != nil {
        t.Fatal(err)
    }
    if n != 0 {
        t.Fatalf("exec mode left %d named statements on the session", n)
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run 'FromSQLDB|FromConnConfig'` → `undefined: FromSQLDB`.

- [ ] **Step 2b: Write the S-88 acceptance test** `driver/pgxdriver/sqldb_adapter_test.go`. It runs the shared PostgreSQL suite (`suite.RunAll`, momo-05 Task 35) through `*sql.DB`, then compares `suite.CapsOf` for both handles. `CapsOf` is used instead of `CapabilitiesOf` because `CapabilitiesOf` leaves the driver-level fields (`Copy`, `Batch`, `Vectors`) false, so it would hide the differences this test exists to catch. The comparison goes through reflection, so a new capability field is compared without editing the test. This step depends on momo-05 Task 35 for `suite`.

```go
//go:build integration

package pgxdriver_test

import (
    "database/sql"
    "reflect"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/driver/pgxdriver"
    "github.com/azrtydxb/momo/internal/suite"
    "github.com/azrtydxb/momo/internal/testdb"
    "github.com/azrtydxb/momo/internal/testdb/testopen"
    _ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

// allowedDiffs is the capability matrix for the database/sql adapter. A capability may differ from the
// native pgx adapter only when it is listed here with its reason. Each reason cites the plan.
var allowedDiffs = map[string]string{
    "Copy":  "momo-03 Task 9 gives FromSQLDB no COPY path; CopyFrom is Task 8, on the native pool and transactions only",
    "Batch": "momo-03 Task 9 gives FromSQLDB no batch path; SendBatch is Task 8, on the native pool and transactions only",
}

func TestPostgresSQLDBAdapter(t *testing.T) {
    sqldb, err := sql.Open("pgx", testdb.PostgresURL(t))
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { sqldb.Close() })

    viaSQL := momo.New(pgxdriver.FromSQLDB(sqldb))
    native := testopen.Postgres(t)

    suite.RunAll(t, viaSQL, suite.CapsOf(viaSQL))

    got := reflect.ValueOf(suite.CapsOf(viaSQL))
    want := reflect.ValueOf(suite.CapsOf(native))
    differs := map[string]bool{}
    for i := 0; i < got.NumField(); i++ {
        f := got.Field(i)
        name := got.Type().Field(i).Name
        if f.Kind() != reflect.Bool || f.Bool() == want.Field(i).Bool() {
            continue
        }
        differs[name] = true
        if _, ok := allowedDiffs[name]; !ok {
            t.Errorf("capability %s differs: database/sql=%v native=%v; list it in allowedDiffs with a reason",
                name, f.Bool(), want.Field(i).Bool())
        }
    }
    for name := range allowedDiffs {
        if !differs[name] {
            t.Errorf("allowedDiffs lists %s, but it no longer differs; remove the entry", name)
        }
    }
}
```

- [ ] **Step 2c: Run and expect FAIL.** `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=... go test -tags integration -run TestPostgresSQLDBAdapter ./driver/pgxdriver` → `undefined: pgxdriver.FromSQLDB`. Until momo-05 Task 35 lands, the same run also reports `undefined: suite.RunAll`.

- [ ] **Step 3: Implement** `sqldb.go` per Interfaces, including the `Capabilities()` method on the `FromSQLDB` driver (Vectors true, as `New` reports).

- [ ] **Step 4: Mutation check.** In `TestFromConnConfigExecMode` change the mode line to `pgx.QueryExecModeCacheStatement`, run; expect FAIL `exec mode left 1 named statements on the session`. Restore `QueryExecModeExec`.

- [ ] **Step 5: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run 'FromSQLDB|FromConnConfig' -race` → `ok`.

- [ ] **Step 5b: Run and expect PASS (S-88).** `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=... go test -tags integration -run TestPostgresSQLDBAdapter ./driver/pgxdriver` → `ok`. Before momo-05 Task 35 lands this cannot pass.

- [ ] **Step 6: Commit.** `feat(pgx): database/sql adoption with explicit exec mode via FromConnConfig`

## Task 10: PgBouncer transaction-mode compatibility with QueryExecModeExec

Files:

- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/pgbouncer_test.go`
- Modify: `/Volumes/DATA/Development/momo/internal/testdb/compose.yaml` (adds the `pgbouncer` service)
- Create: `/Volumes/DATA/Development/momo/internal/testdb/pgbouncer/pgbouncer.ini`
- Create: `/Volumes/DATA/Development/momo/internal/testdb/pgbouncer/userlist.txt`

Interfaces: `testdb.PgBouncerURL(t testing.TB) string` (contract name; the momo-01 testdb package returns connection strings only). It returns the DSN of the PgBouncer port for the same database as `MOMO_TEST_POSTGRES_URL`, with the per-test `search_path` of `testdb.PostgresURL(t)` appended; it fails the test when the URL is unset and `MOMO_REQUIRE_INTEGRATION=1`, and skips otherwise.

Pinned container: `edoburu/pgbouncer:v1.25.2-p0` (published 2026-06-10, verified on Docker Hub during planning).

`internal/testdb/compose.yaml` gains this service under `services:`:

```yaml
services:
  pgbouncer:
    image: edoburu/pgbouncer:v1.25.2-p0
    volumes:
      - ./pgbouncer/pgbouncer.ini:/etc/pgbouncer/pgbouncer.ini:ro
      - ./pgbouncer/userlist.txt:/etc/pgbouncer/userlist.txt:ro
    ports:
      - "6432:6432"
    depends_on:
      - postgres
```

`internal/testdb/pgbouncer/pgbouncer.ini`:

```ini
[databases]
postgres = host=postgres port=5432 dbname=postgres

[pgbouncer]
listen_addr = 0.0.0.0
listen_port = 6432
auth_type = plain
auth_file = /etc/pgbouncer/userlist.txt
pool_mode = transaction
default_pool_size = 4
max_client_conn = 200
max_prepared_statements = 0
track_extra_parameters = search_path
```

`track_extra_parameters = search_path` (PgBouncer 1.20 or later) is required: `testdb.PgBouncerURL(t)` carries the per-test schema as a `search_path` startup parameter, and PgBouncer must apply it to whichever server connection it hands the client. The `[databases]` entry must route to the same database as `MOMO_TEST_POSTGRES_URL`, because that URL's connection creates the per-test schema.

`internal/testdb/pgbouncer/userlist.txt` (test credentials only): `"momo" "momo"`.

Reconciled 2026-11-05: mounting `pgbouncer.ini` and `userlist.txt` does not work here. Docker Desktop cannot share the checkout path (`/Volumes/DATA`), and GitHub Actions service containers start before checkout. The service is therefore configured through the edoburu image environment in both `internal/testdb/compose.yaml` and `.github/workflows/ci.yml`: `POOL_MODE=transaction`, `DEFAULT_POOL_SIZE=4`, `MAX_CLIENT_CONN=200`, `MAX_PREPARED_STATEMENTS=0`, `TRACK_EXTRA_PARAMETERS=search_path`, `IGNORE_STARTUP_PARAMETERS=extra_float_digits`. The two files above are not created. The image's generated ini has no `track_extra_parameters` line and the image's entrypoint has no variable for it, so the pooler does not track the per-test `search_path`: a server connection opened for a client carries it, a reused one keeps the value of the client that opened it. The suite phase of `pgbouncer_test.go` therefore asserts a precondition (probe table plus every backend's `search_path`) and fails with a hint when it does not hold; the workload phase schema-qualifies its tables.

Two phases. Phase 1 runs eight concurrent workers over `onePass` (transactions, reads, batches), so statements hop server connections. Phase 2 runs `suite.RunAll` (momo-05 Task 35) through the same pooler with `suite.CapsOf(db)`, exercising the full query and write surface under transaction pooling. Phase 2 runs after phase 1 has finished, because the suite creates its own tables.

- [ ] **Step 1: Write the failing test** `pgbouncer_test.go`:

```go
package pgxdriver

import (
    "context"
    "errors"
    "fmt"
    "sync"
    "testing"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/internal/suite"
    "github.com/azrtydxb/momo/internal/testdb"
)

const pgbWorkers, pgbIters = 8, 25

func TestPgBouncerTransactionMode(t *testing.T) {
    ctx := context.Background()
    pool, err := pgxpool.New(ctx, testdb.PgBouncerURL(t))
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(pool.Close)
    d := New(pool, WithExecMode(pgx.QueryExecModeExec))
    if _, err := d.Exec(ctx, "DROP TABLE IF EXISTS pgb_items", nil); err != nil {
        t.Fatal(err)
    }
    if _, err := d.Exec(ctx, "CREATE TABLE pgb_items (id bigserial primary key, worker int not null, n int not null)", nil); err != nil {
        t.Fatal(err)
    }
    errs := make(chan error, pgbWorkers*pgbIters)
    var wg sync.WaitGroup
    for w := 0; w < pgbWorkers; w++ {
        wg.Add(1)
        go func(w int) {
            defer wg.Done()
            for i := 0; i < pgbIters; i++ {
                if err := onePass(ctx, d, w, i); err != nil {
                    errs <- fmt.Errorf("worker %d iteration %d: %w", w, i, err)
                    return
                }
            }
        }(w)
    }
    wg.Wait()
    close(errs)
    for err := range errs {
        t.Error(err)
    }
    if t.Failed() {
        return
    }
    rows, err := d.Query(ctx, "SELECT count(*) FROM pgb_items", nil)
    if err != nil {
        t.Fatal(err)
    }
    defer rows.Close()
    rows.Next()
    var total int
    if err := rows.Scan(&total); err != nil {
        t.Fatal(err)
    }
    if total != pgbWorkers*pgbIters {
        t.Fatalf("rows = %d, want %d", total, pgbWorkers*pgbIters)
    }
    // Phase 2: the shared suite through the pooler.
    db := momo.New(d)
    suite.RunAll(t, db, suite.CapsOf(db))
}

func onePass(ctx context.Context, d momo.Driver, w, i int) error {
    tx, err := d.Begin(ctx, momo.TxOptions{})
    if err != nil {
        return err
    }
    if _, err := tx.Exec(ctx, "INSERT INTO pgb_items (worker, n) VALUES ($1, $2)", []any{w, i}); err != nil {
        return errors.Join(err, tx.Rollback(ctx))
    }
    mine, err := countWorker(ctx, tx, w)
    if err != nil {
        return errors.Join(err, tx.Rollback(ctx))
    }
    if mine < 1 {
        return errors.Join(fmt.Errorf("worker %d sees %d rows after its insert", w, mine), tx.Rollback(ctx))
    }
    if err := tx.Commit(ctx); err != nil {
        return err
    }
    _, err = d.(momo.Batcher).SendBatch(ctx, []momo.Compiled{
        {SQL: "UPDATE pgb_items SET n = n WHERE worker = $1", Args: []any{w}},
        {SQL: "SELECT 1", Args: nil},
    })
    return err
}

// countWorker reads one count inside tx. The rows are closed and their Close error is returned.
func countWorker(ctx context.Context, tx momo.DriverTx, w int) (n int, err error) {
    rows, err := tx.Query(ctx, "SELECT count(*) FROM pgb_items WHERE worker = $1", []any{w})
    if err != nil {
        return 0, err
    }
    defer func() { err = errors.Join(err, rows.Close()) }()
    if !rows.Next() {
        return 0, errors.Join(errors.New("count returned no row"), rows.Err())
    }
    if err := rows.Scan(&n); err != nil {
        return 0, err
    }
    return n, nil
}
```

- [ ] **Step 2: Run the mutation check.** Change `WithExecMode(pgx.QueryExecModeExec)` to `WithExecMode(pgx.QueryExecModeCacheStatement)`, run `MOMO_TEST_PGBOUNCER_URL=... go test ./driver/pgxdriver/ -run TestPgBouncerTransactionMode -race`. Expected FAIL containing `prepared statement` and `does not exist` for at least one worker. Restore `QueryExecModeExec`.

- [ ] **Step 3: Implement** the testdb helper `PgBouncer` (momo-01 file, added here), the compose service and the two config files exactly as shown above.

- [ ] **Step 4: Run and expect PASS.** `docker compose -f internal/testdb/compose.yaml up -d pgbouncer` then `MOMO_TEST_PGBOUNCER_URL=... go test ./driver/pgxdriver/ -run TestPgBouncerTransactionMode -race -count=3` → `ok` three times.

- [ ] **Step 5: Commit.** `test(pgx): PgBouncer transaction mode with concurrent workers and exec mode`

## Task 11: Read-replica routing with a unit of work and AllowStaleReads (S-40)

Files:

- Create: `/Volumes/DATA/Development/momo/replica.go`
- Create: `/Volumes/DATA/Development/momo/replica_test.go`
- Modify: `/Volumes/DATA/Development/momo/db.go` (after momo-01 creates it)
- Modify: `/Volumes/DATA/Development/momo/driver_optional.go` is not touched.

Interfaces (root package `momo`):

```go
// WithReplicas registers read-replica drivers. Replicas are used only through DB.Replica.
func WithReplicas(replicas ...Driver) Option

// Replica returns a read handle whose reads go to replica i, unless the context is a unit of work
// that has written: then reads go to the primary. Writes and transactions always go to the primary.
// It returns ErrUnsupported when no replica is configured.
func (db *DB) Replica(i int) (*DB, error)

// NewUnitOfWork returns a context in which every write made through any handle is recorded.
// A Replica handle reads from the primary once a write has been recorded in that context.
func NewUnitOfWork(ctx context.Context) context.Context

// AllowStaleReads returns a context that opts reads of a Replica handle back to the replica even
// after a write in the same unit of work.
func AllowStaleReads(ctx context.Context) context.Context
```

OnConn copies *DB; the copy must nil the replicas field.

Implementation: `uowState{written atomic.Bool}` is stored under an unexported key by `NewUnitOfWork`; `AllowStaleReads` stores a `staleOK` key. `routingDriver{primary, replica Driver}` implements `Driver`: `Dialect` returns the primary's; `Query` goes to the primary when `uowWritten(ctx) && !staleOK(ctx)` and to the replica otherwise; `Exec` and `Begin` always go to the primary. Every non-select statement executed by `execer.exec` (and therefore by Tx) calls `markWrite(ctx)` first. Replicas do not chain.

- [ ] **Step 1: Write the failing test** `replica_test.go`:

```go
package momo

import (
    "context"
    "testing"
)

type countingDriver struct {
    Driver
    reads  *int
    writes *int
}

func (c countingDriver) Query(ctx context.Context, sql string, args []any) (Rows, error) {
    *c.reads++
    return c.Driver.Query(ctx, sql, args)
}

func (c countingDriver) Exec(ctx context.Context, sql string, args []any) (int64, error) {
    if c.writes != nil {
        *c.writes++
    }
    return c.Driver.Exec(ctx, sql, args)
}

func readOnce(t *testing.T, ctx context.Context, e *execer) {
    t.Helper()
    rows, err := e.q.Query(ctx, "SELECT 1", nil)
    if err != nil {
        t.Fatal(err)
    }
    rows.Close()
}

func TestReadReplicaRouting(t *testing.T) {
    var pReads, pWrites, rReads int
    prim := countingDriver{Driver: stubDriver{}, reads: &pReads, writes: &pWrites}
    rep := countingDriver{Driver: stubDriver{}, reads: &rReads}
    db := New(prim, WithReplicas(rep))
    rd, err := db.Replica(0)
    if err != nil {
        t.Fatal(err)
    }
    ctx := context.Background()

    t.Run("read without a unit of work uses the replica", func(t *testing.T) {
        before := rReads
        readOnce(t, ctx, rd.conn())
        if rReads != before+1 {
            t.Fatalf("replica reads %d -> %d", before, rReads)
        }
    })

    t.Run("read after a write in the same unit of work uses the primary", func(t *testing.T) {
        uow := NewUnitOfWork(ctx)
        if _, err := Exec(uow, db, "INSERT INTO t VALUES (1)"); err != nil {
            t.Fatal(err)
        }
        beforeR, beforeP := rReads, pReads
        readOnce(t, uow, rd.conn())
        if rReads != beforeR || pReads != beforeP+1 {
            t.Fatalf("replica reads %d -> %d, primary reads %d -> %d", beforeR, rReads, beforeP, pReads)
        }
    })

    t.Run("AllowStaleReads opts out after a write", func(t *testing.T) {
        uow := AllowStaleReads(NewUnitOfWork(ctx))
        if _, err := Exec(uow, db, "INSERT INTO t VALUES (2)"); err != nil {
            t.Fatal(err)
        }
        before := rReads
        readOnce(t, uow, rd.conn())
        if rReads != before+1 {
            t.Fatalf("stale read did not reach the replica: %d -> %d", before, rReads)
        }
    })

    t.Run("transactions never read from the replica", func(t *testing.T) {
        before := rReads
        err := db.Tx(ctx, TxOptions{}, func(tx *Tx) error {
            rows, err := tx.conn().q.Query(ctx, "SELECT 2", nil)
            if err != nil {
                return err
            }
            return rows.Close()
        })
        if err != nil {
            t.Fatal(err)
        }
        if rReads != before {
            t.Fatalf("a read inside a transaction reached the replica")
        }
    })
}

func TestReplicaUnsupportedWithoutReplicas(t *testing.T) {
    if _, err := New(stubDriver{}).Replica(0); !errors.Is(err, ErrUnsupported) {
        t.Fatalf("err = %v, want ErrUnsupported", err)
    }
}
```

The file also imports `"errors"`. `stubDriver` is a Driver whose `Dialect()` returns `dialect.For(dialect.SQLite)`, `Query` returns a `Rows` whose `Next` is false, `Exec` returns `0, nil`, and `Begin` returns a `DriverTx` forwarding to the same stub.

- [ ] **Step 2: Run and expect FAIL.** `go test -run 'ReadReplica|ReplicaUnsupported' .` → `undefined: WithReplicas`.

- [ ] **Step 3: Implement** `replica.go` and the `db.go` hook: `execer.exec` calls `markWrite(ctx)` when `op != "select"`; `Replica(i)` builds the routing DB described above.

- [ ] **Step 4: Mutation check.** Remove the `markWrite` call from `execer.exec`, run `go test -run TestReadReplicaRouting .`. Expected FAIL `replica reads ... primary reads ...`. Restore.

- [ ] **Step 5: Run and expect PASS.** `go test -run 'ReadReplica|ReplicaUnsupported' -race .` → `ok`.

- [ ] **Step 6: Commit.** `feat(core): explicit read replicas with unit-of-work routing and AllowStaleReads`

## Task 12: Iterator closes the cursor on early exit (S-4 cursor guarantee)

Files:

- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/iter_test.go`

Interfaces used (root, from momo-01 spike `select.go`): `SelectQuery[T].Each(ctx, fn) error`, `SelectQuery[T].Iter(ctx) iter.Seq2[T, error]`.

Test models: a hand-written `Meta[T]` fixture in `testdata`-free test file: `type row struct{ ID int64 }` with `func (*row) MomoMeta() *momo.Meta[row]` returning Columns `[{Name:"id"}]`, Ptr `&m.ID`, Table `"iter_rows"`. The test creates `iter_rows` with 100 rows.

- [ ] **Step 1: Write the failing test** `iter_test.go` with three subtests: early `break` from `Iter`, callback error from `Each`, and a cancelled context mid-iteration; each asserts `PoolStats().AcquiredConns == 0` afterwards:

```go
package pgxdriver

import (
    "context"
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
)

type iterRow struct{ ID int64 }

func (*iterRow) MomoMeta() *momo.Meta[iterRow] {
    return &momo.Meta[iterRow]{
        Table:   "iter_rows",
        Columns: []momo.ColumnMeta{{Name: "id"}},
        Ptr:     func(m *iterRow, i int) any { return &m.ID },
        Value:   func(m *iterRow, i int) any { return m.ID },
    }
}

func seedIter(t *testing.T, d momo.Driver) {
    ctx := context.Background()
    if _, err := d.Exec(ctx, "CREATE TABLE iter_rows (id bigint)", nil); err != nil {
        t.Fatal(err)
    }
    if _, err := d.Exec(ctx, "INSERT INTO iter_rows SELECT generate_series(1,100)", nil); err != nil {
        t.Fatal(err)
    }
}

func TestIterClosesCursor(t *testing.T) {
    pool := openPool(t)
    drv := New(pool)
    seedIter(t, drv)
    db := momo.New(drv)
    q := momo.Select[iterRow](db)
    ctx := context.Background()
    stats := func() int32 { return drv.(momo.PoolStater).PoolStats().AcquiredConns }

    t.Run("break", func(t *testing.T) {
        for _, err := range q.Iter(ctx) {
            if err != nil {
                t.Fatal(err)
            }
            break
        }
        if n := stats(); n != 0 {
            t.Fatalf("cursor left open after break: %d acquired", n)
        }
    })
    t.Run("callback error", func(t *testing.T) {
        sentinel := errors.New("stop")
        err := q.Each(ctx, func(*iterRow) error { return sentinel })
        if !errors.Is(err, sentinel) {
            t.Fatalf("err = %v", err)
        }
        if n := stats(); n != 0 {
            t.Fatalf("cursor left open after callback error: %d", n)
        }
    })
    t.Run("cancelled", func(t *testing.T) {
        cctx, cancel := context.WithCancel(ctx)
        seen := 0
        err := q.Each(cctx, func(*iterRow) error {
            seen++
            if seen == 3 {
                cancel()
            }
            return nil
        })
        if !errors.Is(err, context.Canceled) {
            t.Fatalf("Each after cancel err = %v, want context.Canceled", err)
        }
        if n := stats(); n != 0 {
            t.Fatalf("cursor left open after cancel: %d", n)
        }
    })
}
```

- [ ] **Step 2: Run and expect FAIL.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run TestIterClosesCursor` → fails with `cursor left open after break: 1 acquired` if the iterator does not close on break; if the spike already closes, the failure is the compile error on `momo.Select`'s generic constraint (`PT Modeler[T]` requires `*iterRow` which satisfies it) — the expected message is the first of those two.

- [ ] **Step 3: Implement** in root `select.go` (momo-01 owns it; this task edits it after momo-01 lands): `Iter` and `Each` `defer r.Close()` on every return path, including the `yield` returning false and `ctx.Err()` checks between rows; assert `r.Err()` after the loop.

- [ ] **Step 4: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run TestIterClosesCursor -race` → `ok`.

- [ ] **Step 5: Commit.** `fix(core): iterators always close the cursor`

## Task 13: Pool statistics track acquisition (S-90)

Files:

- Modify: `/Volumes/DATA/Development/momo/driver/pgxdriver/pin_test.go` (adds one test)

Scope: pool statistics only. Bounded memory during iteration belongs to momo-09 (S-56) and is not tested here.

- [ ] **Step 1: Write the failing test**, appended to `pin_test.go`:

```go
func TestPoolStatsReflectAcquisition(t *testing.T) {
    pool := openPool(t)
    d := New(pool)
    ctx := context.Background()
    before := d.(momo.PoolStater).PoolStats().AcquireCount
    p, err := d.(momo.Pinner).Pin(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if err := p.Release(); err != nil {
        t.Fatal(err)
    }
    if after := d.(momo.PoolStater).PoolStats().AcquireCount; after != before+1 {
        t.Fatalf("AcquireCount %d -> %d, want +1", before, after)
    }
}
```

- [ ] **Step 2: Run and expect FAIL** if the `AcquireCount` mapping from Task 7 is missing: `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run TestPoolStatsReflectAcquisition` → `AcquireCount 0 -> 0, want +1`.

- [ ] **Step 3: Implement** nothing new if Task 7 maps `AcquireCount` from `pool.Stat().AcquireCount()`.

- [ ] **Step 4: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run TestPoolStatsReflectAcquisition -race` → `ok`.

- [ ] **Step 5: Commit.** `test(pgx): pool statistics track acquisition`

## Task 14: ServerVersion for MERGE gating

Files:

- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/version.go`
- Create: `/Volumes/DATA/Development/momo/driver/pgxdriver/version_test.go`

Interfaces: `func (d *driver) ServerVersion(ctx context.Context) (int, error)` returns `current_setting('server_version_num')` (for example 170002). It satisfies the root `momo.ServerVersioner` interface from Task 2.

- [ ] **Step 1: Write the failing test** `version_test.go`:

```go
package pgxdriver

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestServerVersion(t *testing.T) {
    pool := openPool(t)
    ctx := context.Background()
    d := New(pool)
    got, err := d.(momo.ServerVersioner).ServerVersion(ctx)
    if err != nil {
        t.Fatal(err)
    }
    var want int
    if err := pool.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&want); err != nil {
        t.Fatal(err)
    }
    if got != want {
        t.Fatalf("ServerVersion = %d, want %d", got, want)
    }
    if got < 150000 {
        t.Fatalf("server %d is below PostgreSQL 15; MERGE is unavailable", got)
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run TestServerVersion` → `d.(momo.ServerVersioner) ... missing method ServerVersion` as a panic.

- [ ] **Step 3: Implement** `version.go`: `pool.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&v)`.

- [ ] **Step 4: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test ./driver/pgxdriver/ -run TestServerVersion -race` → `ok` against `pgvector/pgvector:pg17` (172xxx-range value >= 150000).

- [ ] **Step 5: Commit.** `feat(pgx): ServerVersion for MERGE gating`

## Task 15: testopen SQLite opener, its Each entry and the options path (milestone M2)

Files:

- Modify: `/Volumes/DATA/Development/momo/internal/testdb/testopen/testopen.go` (adds `SQLite` and appends the registry entry)
- Modify: `/Volumes/DATA/Development/momo/internal/testdb/testopen/testopen_test.go` (replaces `TestEachRunsPostgres`, adds the SQLite tests)

Interfaces (added to package `testopen`):

```go
// SQLite opens a fresh *momo.DB on testdb.SQLitePath(t) through sqlitedriver (mattn, sqlite-vec).
// opts are passed to momo.New, as for Postgres. The *sql.DB is closed by t.Cleanup.
func SQLite(t *testing.T, opts ...momo.Option) *momo.DB
```

Registry after this task: `var openers = []opener{{dialect.Postgres, Postgres}, {dialect.SQLite, SQLite}}`. From this point every test written with `testopen.Each` runs on SQLite as well, which is the M2 parity coverage.

- [ ] **Step 1: Write the failing tests.** Replace `TestEachRunsPostgres` (Task 1) with the following; `countHook` is the type defined in Task 1's test file:

```go
package testopen

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/dialect"
)

func TestEachRunsBothDialects(t *testing.T) {
    var ran []dialect.Name
    Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        ran = append(ran, d)
        if _, err := momo.Exec(context.Background(), db, "SELECT 1"); err != nil {
            t.Fatal(err)
        }
    })
    if len(ran) != 2 || ran[0] != dialect.Postgres || ran[1] != dialect.SQLite {
        t.Fatalf("Each ran %v, want [postgres sqlite]", ran)
    }
}

func TestOptionsReachSQLite(t *testing.T) {
    hook := &countHook{}
    db := SQLite(t, momo.WithHook(hook))
    if _, err := momo.Exec(context.Background(), db, "SELECT 1"); err != nil {
        t.Fatal(err)
    }
    if hook.n != 1 {
        t.Fatalf("WithHook passed to SQLite did not reach the database: %d events", hook.n)
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `MOMO_TEST_POSTGRES_URL=... go test ./internal/testdb/testopen/ -run 'TestEachRunsBothDialects|TestOptionsReachSQLite'` → `Each ran [postgres], want [postgres sqlite]` and `too many arguments in call to SQLite`.

- [ ] **Step 3: Implement** `SQLite`: `sqldb, drv, err := sqlitedriver.Open(testdb.SQLitePath(t), sqlitedriver.Config{})` (Task 17 of this plan), `t.Cleanup(func() { sqldb.Close() })`, `return momo.New(drv, opts...)`. Append `{dialect.SQLite, SQLite}` to `openers`.

- [ ] **Step 4: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test ./internal/testdb/testopen/ -race` → `ok`.

- [ ] **Step 5: Commit.** `feat(testopen): SQLite opener and Each entry`

## Task 16: Error classification for SQLite (mattn) with redaction-safe causes

Files:

- Create: `/Volumes/DATA/Development/momo/driver/sqlitedriver/classify.go`
- Create: `/Volumes/DATA/Development/momo/driver/sqlitedriver/classify_test.go`

Interfaces:

```go
package sqlitedriver

// ClassifyError maps a mattn sqlite3.Error to *momo.Error. Other errors are returned unchanged.
func ClassifyError(err error) error
```

Mapping (extended code → kind): `ErrConstraintUnique`, `ErrConstraintPrimaryKey` → `momo.ErrUniqueViolation`; `ErrConstraintForeignKey` → `momo.ErrForeignKeyViolation`; `ErrConstraintCheck` → `momo.ErrCheckViolation`; `ErrConstraintNotNull` → `momo.ErrNotNullViolation`; primary code `ErrBusy` or `ErrLocked` → `momo.ErrBusy`. Constraint text is parsed from the SQLite message `UNIQUE constraint failed: table.column` into `Table` and `Column`; SQLite messages carry names, never values.

The returned `*momo.Error` has `Err` set to `driverError{cause: err, msg: err.Error()}`, where `driverError` is an unexported type with `Error() string` returning `msg` and `Unwrap() error` returning `cause`, so `errors.As(err, &sqlite3.Error{})` still succeeds.

- [ ] **Step 1: Write the failing test** `classify_test.go`:

```go
package sqlitedriver

import (
    "errors"
    "testing"

    "github.com/mattn/go-sqlite3"
    "github.com/azrtydxb/momo"
)

func TestClassifyUniqueKeepsCause(t *testing.T) {
    in := sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintUnique}
    got := ClassifyError(in)
    if !errors.Is(got, momo.ErrUniqueViolation) {
        t.Fatalf("errors.Is(ErrUniqueViolation) = false for %v", got)
    }
    var se sqlite3.Error
    if !errors.As(got, &se) || se.ExtendedCode != sqlite3.ErrConstraintUnique {
        t.Fatalf("driver error not reachable via errors.As: %v", got)
    }
}

func TestClassifyBusy(t *testing.T) {
    got := ClassifyError(sqlite3.Error{Code: sqlite3.ErrBusy, ExtendedCode: sqlite3.ErrBusy})
    if !errors.Is(got, momo.ErrBusy) {
        t.Fatalf("SQLITE_BUSY not classified as momo.ErrBusy: %v", got)
    }
}

func TestClassifyPassesThroughUnknown(t *testing.T) {
    plain := errors.New("boom")
    if got := ClassifyError(plain); got != plain {
        t.Fatalf("unknown error changed: %v", got)
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./driver/sqlitedriver/ -run TestClassify` → `undefined: ClassifyError`.

- [ ] **Step 3: Implement** `classify.go` per the mapping above. Use `errors.As(err, &se)` with `var se sqlite3.Error`. Compute the kind with a `switch se.ExtendedCode`, falling back to `switch se.Code` for `ErrBusy`/`ErrLocked`. Parse the column with `strings.TrimPrefix` after the last `": "` in the message, and split on `.` into table and column.

- [ ] **Step 4: Run and expect PASS.** `go test ./driver/sqlitedriver/ -run TestClassify -race` → `ok`.

- [ ] **Step 5: Commit.** `feat(sqlite): classify mattn sqlite3 errors`

## Task 17: SQLite adapter core (mattn + sqlite-vec) with recommended configuration

Files:

- Create: `/Volumes/DATA/Development/momo/driver/sqlitedriver/open.go`
- Create: `/Volumes/DATA/Development/momo/driver/sqlitedriver/open_test.go`
- Modify: `/Volumes/DATA/Development/momo/go.mod` (via `go get`)

Interfaces:

```go
package sqlitedriver

// Config holds the recommended SQLite settings (S-57). Zero values select the defaults.
type Config struct {
    BusyTimeout  time.Duration // default 5 * time.Second
    Synchronous  string        // "NORMAL" (default), "FULL", "EXTRA", "OFF"
    TxLock       string        // "immediate" (default), "deferred", "exclusive"
    MaxOpenConns int           // default 1 (single-writer guidance); 0 means 1
}

// Open opens a SQLite database with mattn/go-sqlite3 and sqlite-vec. The caller owns
// the returned *sql.DB. The connect hook verifies sqlite_version() >= 3.35.0 and that
// load_extension is unavailable (see checkLoadExtensionError in Task 20).
func Open(dsn string, cfg Config) (*sql.DB, momo.Driver, error)

// checkVersion returns an error unless v is at least 3.35.0.
func checkVersion(v string) error
```

`Open` also sets `journal_mode=WAL` through the DSN. Driver registration uses `registerOnce sync.Once`: it registers `"momo-sqlite3"` with `&sqlite3.SQLiteDriver{ConnectHook: connectHook}` and calls `sqlite_vec.Auto()` in the same once. This is the only process-global side effect (sqlite-vec's `Auto` calls `sqlite3_auto_extension`).

DSN construction appends to the caller DSN with `url.Values`: `_busy_timeout=<ms>`, `_journal_mode=WAL`, `_synchronous=<value>`, `_foreign_keys=on`, `_txlock=<value>`.

The driver type implements `Dialect`, `Query`, `Exec`, `Begin`, `Capabilities` (`Vectors: true`). `Begin` enforces the isolation rules listed in Task 18. `Pin`, `PoolStats` and `ClassifyError` come in Tasks 13 and 2.

- [ ] **Step 1: Write the failing test** `open_test.go`:

```go
package sqlitedriver

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/internal/testdb"
)

func TestCheckVersion(t *testing.T) {
    for v, ok := range map[string]bool{"3.34.9": false, "3.35.0": true, "3.53.4": true, "2.8.17": false} {
        err := checkVersion(v)
        if (err == nil) != ok {
            t.Fatalf("checkVersion(%q) err=%v, want ok=%v", v, err, ok)
        }
    }
}

func TestSQLiteRecommendedConfig(t *testing.T) {
    db, _, err := Open(testdb.SQLitePath(t), Config{})
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { db.Close() })
    ctx := context.Background()
    var mode string
    var sync, fk, busy int
    if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
        t.Fatal(err)
    }
    if err := db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&sync); err != nil {
        t.Fatal(err)
    }
    if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
        t.Fatal(err)
    }
    if err := db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy); err != nil {
        t.Fatal(err)
    }
    if mode != "wal" || sync != 1 || fk != 1 || busy != 5000 {
        t.Fatalf("journal=%s synchronous=%d foreign_keys=%d busy_timeout=%d", mode, sync, fk, busy)
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./driver/sqlitedriver/ -run 'CheckVersion|RecommendedConfig'` → `undefined: checkVersion` and `undefined: Open`.

- [ ] **Step 3: Implement** `open.go` per Interfaces. The connect hook runs `SELECT sqlite_version()`, passes it to `checkVersion`, then runs the `load_extension` probe through `checkLoadExtensionError` (Task 20) on every new connection. `Open` sets `db.SetMaxOpenConns(n)` with `n = cfg.MaxOpenConns`, mapping 0 to 1.

- [ ] **Step 4: Run and expect PASS.** `go test ./driver/sqlitedriver/ -run 'CheckVersion|RecommendedConfig' -race` → `ok`.

- [ ] **Step 5: Commit.** `feat(sqlite): open mattn database with recommended configuration`

## Task 18: SQLite transaction isolation and read-only transactions

Files:

- Create: `/Volumes/DATA/Development/momo/driver/sqlitedriver/driver_test.go`
- Modify: `/Volumes/DATA/Development/momo/driver/sqlitedriver/open.go` (Begin)

Rules (implemented in `Begin`): `Isolation` default or `sql.LevelSerializable` is accepted (SQLite is serializable); any other level returns `momo.ErrUnsupported`. `ReadOnly: true` runs `PRAGMA query_only=ON` on a dedicated `*sql.Conn` before `BeginTx`; the wrapping transaction resets it with `PRAGMA query_only=OFF` after Commit or Rollback, then returns the connection to the pool. Without the reset, a pooled connection would stay read-only.

- [ ] **Step 1: Write the failing tests** `driver_test.go`:

```go
package sqlitedriver

import (
    "context"
    "database/sql"
    "errors"
    "strings"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/internal/testdb"
)

func TestSQLiteIsolationRules(t *testing.T) {
    _, drv, err := Open(testdb.SQLitePath(t), Config{})
    if err != nil {
        t.Fatal(err)
    }
    ctx := context.Background()
    if _, err := drv.Begin(ctx, momo.TxOptions{Isolation: sql.LevelReadUncommitted}); !errors.Is(err, momo.ErrUnsupported) {
        t.Fatalf("read uncommitted err = %v, want ErrUnsupported", err)
    }
    tx, err := drv.Begin(ctx, momo.TxOptions{Isolation: sql.LevelSerializable})
    if err != nil {
        t.Fatal(err)
    }
    if err := tx.Rollback(ctx); err != nil {
        t.Fatal(err)
    }
}

func TestSQLiteReadOnlyTransaction(t *testing.T) {
    db, drv, err := Open(testdb.SQLitePath(t), Config{})
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { db.Close() })
    ctx := context.Background()
    if _, err := drv.Exec(ctx, "CREATE TABLE ro (x int)", nil); err != nil {
        t.Fatal(err)
    }
    tx, err := drv.Begin(ctx, momo.TxOptions{ReadOnly: true})
    if err != nil {
        t.Fatal(err)
    }
    _, err = tx.Exec(ctx, "INSERT INTO ro VALUES (1)", nil)
    if err == nil || !strings.Contains(err.Error(), "readonly") {
        t.Fatalf("write inside read-only transaction err = %v", err)
    }
    if err := tx.Rollback(ctx); err != nil {
        t.Fatal(err)
    }
    if _, err := drv.Exec(ctx, "INSERT INTO ro VALUES (2)", nil); err != nil {
        t.Fatalf("query_only leaked to the pooled connection: %v", err)
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./driver/sqlitedriver/ -run 'IsolationRules|ReadOnlyTransaction'` → `read-only ... ` case fails with `write inside read-only transaction err = <nil>` (ReadOnly is not yet enforced; before Begin supports it the failure is the same message).

- [ ] **Step 3: Implement** the ReadOnly branch in `Begin` per the rules above.

- [ ] **Step 4: Mutation check.** Remove the `PRAGMA query_only=OFF` reset, run `go test ./driver/sqlitedriver/ -run ReadOnlyTransaction`. Expected FAIL `query_only leaked to the pooled connection`. Restore.

- [ ] **Step 5: Run and expect PASS.** `go test ./driver/sqlitedriver/ -run 'IsolationRules|ReadOnlyTransaction' -race` → `ok`.

- [ ] **Step 6: Commit.** `feat(sqlite): read-only transactions via query_only with reset on finish`

## Task 19: SQLite pinned connections, pool statistics and busy mapping

Files:

- Create: `/Volumes/DATA/Development/momo/driver/sqlitedriver/pin_test.go`
- Modify: `/Volumes/DATA/Development/momo/driver/sqlitedriver/driver.go`

Interfaces: `Pin(ctx) (momo.PinnedConn, error)` via `db.Conn(ctx)`, with `Begin(ctx, momo.TxOptions) (momo.DriverTx, error)` on the pinned connection via `conn.BeginTx` and `Release() error` via `conn.Close()`, as in Task 9; `PoolStats()` from `db.Stats()` with the same mapping as Task 9.

- [ ] **Step 1: Write the failing test** `pin_test.go`:

```go
package sqlitedriver

import (
    "context"
    "errors"
    "testing"
    "time"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/internal/testdb"
)

func TestBusyIsStructuredError(t *testing.T) {
    dsn := testdb.SQLitePath(t)
    db1, d1, err := Open(dsn, Config{BusyTimeout: 50 * time.Millisecond, MaxOpenConns: 1})
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { db1.Close() })
    db2, d2, err := Open(dsn, Config{BusyTimeout: 50 * time.Millisecond, MaxOpenConns: 1})
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { db2.Close() })
    ctx := context.Background()
    if _, err := db1.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS t_busy (x int)"); err != nil {
        t.Fatal(err)
    }
    tx, err := d1.Begin(ctx, momo.TxOptions{})
    if err != nil {
        t.Fatal(err)
    }
    defer tx.Rollback(ctx)
    if _, err := tx.Exec(ctx, "INSERT INTO t_busy VALUES (1)", nil); err != nil {
        t.Fatal(err)
    }
    _, err = d2.Exec(ctx, "INSERT INTO t_busy VALUES (2)", nil)
    if !errors.Is(err, momo.ErrBusy) {
        t.Fatalf("second writer err = %v, want momo.ErrBusy", err)
    }
}

func TestSQLitePinnedAndStats(t *testing.T) {
    db, d, err := Open(testdb.SQLitePath(t), Config{})
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { db.Close() })
    ctx := context.Background()
    p, err := d.(momo.Pinner).Pin(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if st := d.(momo.PoolStater).PoolStats(); st.AcquiredConns != 1 {
        t.Fatalf("AcquiredConns = %d", st.AcquiredConns)
    }
    if err := p.Release(); err != nil {
        t.Fatal(err)
    }
}
```

The `Config` used in `TestBusyIsStructuredError` ensures the second writer's `_txlock=immediate` BEGIN waits `BusyTimeout` and then fails with `SQLITE_BUSY`, which Task 16 classifies as `momo.ErrBusy`.

- [ ] **Step 2: Run and expect FAIL.** `go test ./driver/sqlitedriver/ -run 'Busy|Pinned'` → `d.(momo.Pinner)` panics or the busy error is `*sqlite3.Error`, not `momo.ErrBusy`.

- [ ] **Step 3: Implement** Pin, PoolStats and the `ClassifyError` hookup in `driver.go`/`pin.go`. Make sure `Exec` and `Query` route errors through `ClassifyError` (the momo root also wraps them; both paths must agree).

- [ ] **Step 4: Run and expect PASS.** `go test ./driver/sqlitedriver/ -run 'Busy|Pinned' -race` → `ok`.

- [ ] **Step 5: Commit.** `feat(sqlite): pinned connections, pool stats, structured busy error`

## Task 20: SQLite load_extension is refused on every connection (S-45)

Files:

- Create: `/Volumes/DATA/Development/momo/driver/sqlitedriver/extension_test.go`
- Modify: `/Volumes/DATA/Development/momo/driver/sqlitedriver/open.go` (connect hook)

Behavior: when SQLite extension loading is disabled, the SQL function `load_extension` fails with the message `not authorized`. That message is the only proof of disabled loading, because a missing extension file also makes `load_extension` fail (with `no such file`), so the check must read the message. The connect hook runs this probe on every new connection, so a connection that can load extensions is refused when it opens.

Interfaces: `func checkLoadExtensionError(err error) error` returns nil only when `err` contains `not authorized`; nil input and any other error text yield an error naming `load_extension is enabled`.

- [ ] **Step 1: Write the failing tests** `extension_test.go`:

```go
package sqlitedriver

import (
    "context"
    "database/sql"
    "errors"
    "strings"
    "testing"

    "github.com/azrtydxb/momo/internal/testdb"
)

func TestLoadExtensionProbeClassifiesErrors(t *testing.T) {
    if err := checkLoadExtensionError(errors.New("not authorized")); err != nil {
        t.Fatalf("disabled loading reported as enabled: %v", err)
    }
    if err := checkLoadExtensionError(errors.New("x.so: cannot open shared object file: No such file or directory")); err == nil {
        t.Fatal("a missing-file error must not count as disabled loading")
    }
    if err := checkLoadExtensionError(nil); err == nil {
        t.Fatal("success must count as enabled loading")
    }
}

func TestSQLiteNoLoadExtension(t *testing.T) {
    db, _, err := Open(testdb.SQLitePath(t), Config{MaxOpenConns: 2})
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { db.Close() })
    ctx := context.Background()
    c1, err := db.Conn(ctx)
    if err != nil {
        t.Fatal(err)
    }
    defer c1.Close()
    c2, err := db.Conn(ctx)
    if err != nil {
        t.Fatal(err)
    }
    defer c2.Close()
    for i, c := range []*sql.Conn{c1, c2} {
        _, err := c.ExecContext(ctx, "SELECT load_extension('momo_probe')")
        if err == nil || !strings.Contains(err.Error(), "not authorized") {
            t.Fatalf("connection %d: load_extension error = %v, want SQLite's disabled message", i, err)
        }
        var v string
        if err := c.QueryRowContext(ctx, "SELECT vec_version()").Scan(&v); err != nil {
            t.Fatalf("connection %d: sqlite-vec unavailable: %v", i, err)
        }
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./driver/sqlitedriver/ -run 'LoadExtension'` → `undefined: checkLoadExtensionError`.

- [ ] **Step 3: Implement** `checkLoadExtensionError` and call it from the connect hook: `_, err := conn.Exec("SELECT load_extension('momo_probe')", nil)` then `if e := checkLoadExtensionError(err); e != nil { return e }`. `conn` is the `*sqlite3.SQLiteConn` the hook receives, so the probe runs on each physical connection.

- [ ] **Step 4: Run and expect PASS.** `go test ./driver/sqlitedriver/ -run 'LoadExtension' -race` → `ok`.

- [ ] **Step 5: Commit.** `test(sqlite): refuse load_extension on every connection by its disabled message`

## Task 21: Structured error round trip for SQLite (classification in use)

Files:

- Create: `/Volumes/DATA/Development/momo/driver/sqlitedriver/errors_test.go`

- [ ] **Step 1: Write the failing test:**

```go
package sqlitedriver

import (
    "context"
    "errors"
    "testing"

    "github.com/mattn/go-sqlite3"
    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/internal/testdb"
)

func TestErrorClassificationSQLite(t *testing.T) {
    db, d, err := Open(testdb.SQLitePath(t), Config{})
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { db.Close() })
    ctx := context.Background()
    if _, err := db.ExecContext(ctx, "CREATE TABLE u (id integer primary key, email text unique not null)"); err != nil {
        t.Fatal(err)
    }
    if _, err := db.ExecContext(ctx, "INSERT INTO u (email) VALUES ('a@x')"); err != nil {
        t.Fatal(err)
    }
    _, err = d.Exec(ctx, "INSERT INTO u (email) VALUES ('a@x')", nil)
    if !errors.Is(err, momo.ErrUniqueViolation) {
        t.Fatalf("unique: %v", err)
    }
    var se sqlite3.Error
    if !errors.As(err, &se) || se.ExtendedCode != sqlite3.ErrConstraintUnique {
        t.Fatalf("cause not reachable: %v", err)
    }
    _, err = d.Exec(ctx, "INSERT INTO u (email) VALUES (NULL)", nil)
    if !errors.Is(err, momo.ErrNotNullViolation) {
        t.Fatalf("not null: %v", err)
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./driver/sqlitedriver/ -run ErrorClassificationSQLite` → `not null: <nil>` or raw `*sqlite3.Error` until the driver routes Exec through `ClassifyError`.

- [ ] **Step 3: Implement** the `ClassifyError` wiring in `driver.go`'s `Exec`/`Query` error returns (Task 16 defines the mapping).

- [ ] **Step 4: Run and expect PASS.** `go test ./driver/sqlitedriver/ -race` → `ok`.

- [ ] **Step 5: Commit.** `test(sqlite): error classification end to end`

## Task 22: Pure-Go SQLite adapter (modernc, no cgo, no vectors)

Files:

- Create: `/Volumes/DATA/Development/momo/driver/sqlitepure/open.go`
- Create: `/Volumes/DATA/Development/momo/driver/sqlitepure/open_test.go`
- Modify: `/Volumes/DATA/Development/momo/go.mod` (via `go get modernc.org/sqlite@v1.60.1`)

Interfaces: `func Open(dsn string, cfg sqlitedriver.Config) (*sql.DB, momo.Driver, error)` with the same `Config` type imported from `driver/sqlitedriver`. Driver name `"sqlite"` (modernc registers it). DSN built with `_pragma=busy_timeout(<ms>)`, `_pragma=journal_mode(WAL)`, `_pragma=synchronous(NORMAL)`, `_pragma=foreign_keys(1)`, `_txlock=immediate`, appended to the caller DSN with `url.Values` style. `Capabilities{Vectors: false}`. Vector operations are rejected: `Capabilities` reports false and vector builders (plan momo-04) return `ErrUnsupported` for it.

Error classification: modernc `*sqlite.Error` (pointer receiver) with `Code()`; map `Code()&0xff == 5` (SQLITE_BUSY) and `== 6` (SQLITE_LOCKED) → `ErrBusy`; `Code() == 2067 || 1555` → `ErrUniqueViolation`; `787` → `ErrForeignKeyViolation`; `275` → `ErrCheckViolation`; `1299` → `ErrNotNullViolation`. `errors.As(err, &me)` with `var me *sqlite.Error`.

- [ ] **Step 1: Write the failing test** `open_test.go`:

```go
package sqlitepure

import (
    "context"
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/internal/testdb"
)

func TestPureSQLiteCapabilitiesAndErrors(t *testing.T) {
    _, d, err := Open(testdb.SQLitePath(t), sqlitedriver.Config{})
    if err != nil {
        t.Fatal(err)
    }
    if d.(momo.CapabilityReporter).Capabilities().Vectors {
        t.Fatal("pure-Go driver must report Vectors=false")
    }
    ctx := context.Background()
    if _, err := d.Exec(ctx, "CREATE TABLE u (email text unique)", nil); err != nil {
        t.Fatal(err)
    }
    if _, err := d.Exec(ctx, "INSERT INTO u VALUES ('a')", nil); err != nil {
        t.Fatal(err)
    }
    if _, err := d.Exec(ctx, "INSERT INTO u VALUES ('a')", nil); !errors.Is(err, momo.ErrUniqueViolation) {
        t.Fatalf("err = %v", err)
    }
}
```

(The file must import `sqlitedriver "github.com/azrtydxb/momo/driver/sqlitedriver"` as well.)

- [ ] **Step 2: Run and expect FAIL.** `go test ./driver/sqlitepure/ -run Pure` → `undefined: Open`.

- [ ] **Step 3: Implement** `open.go` and `classify.go` (pure classification as above), with the `driver` methods shared by reusing `sqlitedriver`'s exported `Config` only. Do not import `driver/sqlitedriver` for code beyond `Config`, so the cgo package is not linked (verified in Task 23).

- [ ] **Step 4: Run and expect PASS.** `go test ./driver/sqlitepure/ -race` → `ok`.

- [ ] **Step 5: Commit.** `feat(sqlite): pure-Go modernc adapter without vector support`

## Task 23: Pure-Go build proof (S-89)

Files:

- Create: `/Volumes/DATA/Development/momo/driver/sqlitepure/nocgo_test.go`
- Modify: `/Volumes/DATA/Development/momo/driver/sqlitepure/classify.go` (adds the `vec_` rule)

- [ ] **Step 1: Write the failing test** `nocgo_test.go`:

```go
package sqlitepure

import (
    "context"
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/driver/sqlitedriver"
    "github.com/azrtydxb/momo/internal/testdb"
)

func TestSQLitePureGo(t *testing.T) {
    _, d, err := Open(testdb.SQLitePath(t), sqlitedriver.Config{})
    if err != nil {
        t.Fatal(err)
    }
    ctx := context.Background()
    if _, err := d.Exec(ctx, "CREATE TABLE p (x int)", nil); err != nil {
        t.Fatal(err)
    }
    if _, err := d.Exec(ctx, "SELECT vec_version()", nil); !errors.Is(err, momo.ErrUnsupported) {
        t.Fatalf("vector function err = %v, want ErrUnsupported", err)
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `CGO_ENABLED=0 go test ./driver/sqlitepure/ -run TestSQLitePureGo` → `vec_version: no such function` (a plain error, not `ErrUnsupported`).

- [ ] **Step 3: Implement** the rule in `classify.go`: a message containing `no such function: vec_` maps to `momo.ErrUnsupported` with `Op` set by the caller. This is the pure driver's only message-based rule and is documented in the file comment.

- [ ] **Step 4: Run and expect PASS.** `CGO_ENABLED=0 go test ./driver/sqlitepure/ -race` → `ok`; `CGO_ENABLED=0 go list -deps ./driver/sqlitepure | grep -c mattn` prints `0`.

- [ ] **Step 5: Commit.** `test(sqlite): pure-Go build has no cgo and reports vectors unsupported`

## Task 24: SQLite round trip through the public driver interface

Files:

- Create: `/Volumes/DATA/Development/momo/driver/driver_roundtrip_test.go` (package `driver_test`)

- [ ] **Step 1: Write the failing test:**

```go
package driver_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/driver/sqlitedriver"
    "github.com/azrtydxb/momo/internal/testdb"
)

func TestRoundTripSQLite(t *testing.T) {
    _, d, err := sqlitedriver.Open(testdb.SQLitePath(t), sqlitedriver.Config{})
    if err != nil {
        t.Fatal(err)
    }
    ctx := context.Background()
    if _, err := d.Exec(ctx, "CREATE TABLE r (id integer primary key, v text)", nil); err != nil {
        t.Fatal(err)
    }
    if _, err := d.Exec(ctx, "INSERT INTO r (v) VALUES (?)", []any{"x"}); err != nil {
        t.Fatal(err)
    }
    rows, err := d.Query(ctx, "SELECT v FROM r", nil)
    if err != nil {
        t.Fatal(err)
    }
    defer rows.Close()
    if !rows.Next() {
        t.Fatal("no row")
    }
    var v string
    if err := rows.Scan(&v); err != nil || v != "x" {
        t.Fatalf("v=%q err=%v", v, err)
    }
    _ = momo.New(d)
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./driver/ -run TestRoundTripSQLite` → `undefined: sqlitedriver.Open` before Task 17 lands; after Task 17 the expected failure is a Scan mismatch if the raw `?` placeholder path is wrong.

- [ ] **Step 3: Implement** nothing new if Tasks 11–12 pass; fix the `?` path in the driver's `Query`/`Exec` only if this test shows a mismatch.

- [ ] **Step 4: Run and expect PASS.** `go test ./driver/ -race` → `ok`.

- [ ] **Step 5: Commit.** `test(driver): sqlite round trip through the driver interface`

## Task 25: Document process-wide effects and hardening of the cgo SQLite driver

Files:

- Create: `/Volumes/DATA/Development/momo/driver/sqlitedriver/doc.go`
- Create: `/Volumes/DATA/Development/momo/driver/sqlitedriver/doc_test.go`

`doc.go` states: the package requires cgo and a C toolchain; the first `Open` calls `sqlite_vec.Auto()`, which registers sqlite-vec process-wide through `sqlite3_auto_extension`; recommended hardening is building with `-tags sqlite_omit_load_extension`; `Config` defaults.

- [ ] **Step 1: Write the failing test** `doc_test.go`:

```go
package sqlitedriver

import (
    "os"
    "strings"
    "testing"
)

func TestDocStatesProcessGlobalEffects(t *testing.T) {
    b, err := os.ReadFile("doc.go")
    if err != nil {
        t.Fatal(err)
    }
    s := string(b)
    for _, want := range []string{"process-wide", "sqlite_omit_load_extension", "cgo"} {
        if !strings.Contains(s, want) {
            t.Fatalf("doc.go does not mention %q", want)
        }
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./driver/sqlitedriver/ -run TestDocStates` → `open doc.go: no such file or directory`.

- [ ] **Step 3: Implement** `doc.go` with the package comment containing those three phrases.

- [ ] **Step 4: Run and expect PASS.** `go test ./driver/sqlitedriver/ -run TestDocStates -race` → `ok`.

- [ ] **Step 5: Commit.** `docs(sqlite): document cgo, process-wide extension registration and hardening`

## Task 26: Final gate for the drivers plan

Files: none created. This task runs the gate.

- [ ] **Step 1: Run the cgo suite.** `go test -race ./driver/...` with `MOMO_TEST_POSTGRES_URL`, `MOMO_TEST_PGBOUNCER_URL` set → `ok` for each package.
- [ ] **Step 2: Run the pure-Go suite.** `CGO_ENABLED=0 go test ./driver/sqlitepure/...` → `ok`.
- [ ] **Step 3: Check the import graph.** `go list -deps ./driver/pgxdriver ./driver/sqlitepure | grep -E 'mattn|sqlite-vec' | wc -l` → `0`.
- [ ] **Step 4: Commit nothing.** This task has no code change; record the outputs in the PR description.

## Spec coverage

| Spec id                                      | Tasks                                                                                                          |
| -------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| S-7                                          | 6 (PostgreSQL transactions, isolation, savepoints), 18 (SQLite isolation and read-only)                        |
| S-8                                          | 3 (PostgreSQL classification), 16 (SQLite classification), 21 (SQLite error round trip)                        |
| S-9                                          | 4, 8, 9 (PostgreSQL pgx, COPY and batch on pools and transactions, database/sql), 17 (SQLite via database/sql) |
| S-18 pool wait                               | 5 (pool acquire tracer feeds `momo.RecordPoolWait`)                                                            |
| S-30                                         | 4 (exec mode), 10 (PgBouncer transaction mode)                                                                 |
| S-40                                         | 11 (replicas, unit of work, AllowStaleReads)                                                                   |
| S-45                                         | 17 (open), 20 (load_extension refused on every connection), 25 (hardening doc)                                 |
| S-57                                         | 17 (WAL, busy_timeout, synchronous), 19 (busy mapping)                                                         |
| S-88                                         | 9                                                                                                              |
| S-89                                         | 22 (modernc adapter), 23 (no-cgo build)                                                                        |
| S-90                                         | 7 (PostgreSQL pinned, OnConn), 13 (pool stats acquisition), 19 (SQLite pinned)                                 |
| TestIterClosesCursor                         | 12                                                                                                             |
| SQLite >= 3.35 check                         | 17                                                                                                             |
| TestErrorClassification (drivers)            | 3, 16, 21                                                                                                      |
| TestTransactions (driver)                    | 6, 18                                                                                                          |
| TestSQLiteRecommendedConfig                  | 17, 19                                                                                                         |
| TestSQLiteNoLoadExtension                    | 20                                                                                                             |
| TestPgBouncerTransactionMode                 | 10                                                                                                             |
| TestPinnedConn                               | 7                                                                                                              |
| TestPoolWaitRecordedWhenExhausted            | 5                                                                                                              |
| TestCopyAndBatchInTransaction                | 8                                                                                                              |
| TestSQLitePureGo                             | 23                                                                                                             |
| TestPostgresSQLDBAdapter                     | 9                                                                                                              |
| MERGE version gate input                     | 14                                                                                                             |
| testopen.Each (options) and dialect registry | 1 (Postgres, Each with options), 15 (SQLite entry and options)                                                 |
| Final gate                                   | 26                                                                                                             |

## Dependencies on other plans

- momo-01-foundation provides: `go.mod`; `internal/testdb` URL helpers `PostgresURL`, `PostgresAppRoleURL`, `PgBouncerURL`, `SQLitePath` (strings only); `driver.go`, `errors.go` sentinels (`ErrBusy`, `ErrUnsupported`, ...), `db.go`, `select.go` with `Iter` and `Each`; the `dialect` package; `Compiled` and `Rows`; `compose.yaml` with a `pgbouncer` service; the root unit fixture `unitDocument`.
- momo-09 defines `momo.RecordPoolWait` and `QueryEvent.PoolWait`. Task 5 calls the former and its test reads the latter, so Task 5 lands after momo-09's recorder task.
- momo-05 consumes `testopen.Each` from Task 1 (PostgreSQL) and Task 15 (SQLite), and `momo.CopierOf` from Task 8. Task 10 phase 2 calls `suite.RunAll` and `suite.CapsOf` from momo-05; Task 10 closes after momo-05 Task 35 exists.
- momo-08 appends its MySQL84, MySQL97 and MariaDB entries to `testopen`'s registry.
