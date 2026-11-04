# momo-12-performance — implementation plan

Status: draft
Spec: .procoder/specs/momo.md

Milestone: M4 (after M1 to M3). Benchmarks run on PostgreSQL first; SQLite or MySQL/MariaDB benchmark rows are added only when `testopen.Each` reports that dialect in this build.

## Goal

Publish reproducible performance evidence for Momo: a separate `bench/` Go module that measures ORM-only microbenchmarks and real-database workloads against handwritten pgx and database/sql, GORM, Ent and Bun with p50/p95/p99, equivalence checks, recall-matched vector benchmarks, bulk insert path comparisons, a benchstat allocation gate in CI, and measured budgets in docs/PERFORMANCE.md.

## Architecture

`bench/` is its own module (`github.com/azrtydxb/momo/bench`) so GORM, Ent and Bun never enter the library's import graph. A `bench/workload` package defines each operation once in terms of an `Impl` interface with canonical result encodings; adapters under `bench/adapters/` implement it for handwritten pgx, handwritten database/sql, Momo, GORM, Ent and Bun on the same PostgreSQL+pgvector server with identical pool settings. `make bench` runs the full suite on a dedicated machine ten times per benchmark, captures the environment and pinned versions, and writes raw output under `bench/results/`; `make bench-smoke` runs one iteration in CI. Allocation counts, which are deterministic across runs, are gated in CI with benchstat against a committed baseline; timings are judged only on the dedicated machine.

## Constraints

- Performance claims without reproducible evidence; zero-allocation promises (out of scope: spec). Published budgets must come from the stored raw results.
- CI: GitHub Actions on free GitHub-hosted runners only (public repo): ubuntu-latest for most jobs, windows-latest for the SQLite/generator suite, PostgreSQL+pgvector as a `pgvector/pgvector` service container, SQLite+sqlite-vec via cgo on the runner; a macos-latest job builds and runs the SQLite suite to cover darwin/cgo. No self-hosted or paid larger runners. Benchmarks published as release evidence are run on a documented dedicated machine, because shared runners are too noisy for regression gating; CI only smoke-runs benchmarks.
- Generator and migration tooling dependencies (golang.org/x/tools etc.) stay out of the application runtime import graph; benchmark comparison ORMs live in a separate Go module.
- Go: minimum 1.26 in go.mod; CI tests 1.26 and 1.27. Dependencies pinned in go.mod/go.sum.
- PostgreSQL driver: jackc/pgx v5 (pinned) via pgxpool, plus pgx stdlib for *sql.DB adoption.
- Generated execution path performs no struct-tag inspection, no reflection over models and no name-based field resolution per query.
- No hidden background goroutines, no implicit migrations at connect time, no unexplained global mutable state.
- PostgreSQL bind-parameter limit 65535 (bulk insert chunking and eager loading use it).
- Benchmark equivalence rule (spec S-19): each compared ORM returns identical rows per workload; a workload whose SQL semantics differ between ORMs fails `TestBenchEquivalence`.
- Regression policy and budgets (spec S-19): reproducible commands, environment, raw results, repeated runs, noise-aware regression policy, measured budgets; generated-code size and compile cost.

## Version record (verified 2026-11-04)

| Module or image                          | Pinned version                                                                           | Released                    |
| ---------------------------------------- | ---------------------------------------------------------------------------------------- | --------------------------- |
| github.com/jackc/pgx/v5                  | v5.12.0                                                                                  | 2026-10-31                  |
| github.com/go-sql-driver/mysql           | v1.10.1                                                                                  | 2026-09-02                  |
| github.com/mattn/go-sqlite3              | v1.14.52                                                                                 | 2026-09-05                  |
| github.com/asg017/sqlite-vec-go-bindings | v0.1.6 (cgo subpackage)                                                                  | 2024-11-20                  |
| modernc.org/sqlite                       | v1.61.1                                                                                  | 2026-10-31                  |
| gorm.io/gorm                             | v1.31.2                                                                                  | 2026-06-22                  |
| gorm.io/driver/postgres                  | v1.6.3                                                                                   | 2026-09-14                  |
| entgo.io/ent                             | v0.14.6                                                                                  | 2026-03-17                  |
| github.com/uptrace/bun                   | v1.3.0                                                                                   | 2026-10-01                  |
| github.com/uptrace/bun/dialect/pgdialect | v1.3.0                                                                                   | 2026-10-01                  |
| github.com/stretchr/testify              | v1.12.1                                                                                  | 2026-08-17                  |
| golang.org/x/perf/cmd/benchstat          | v0.0.0-20261009192801-be2c69fb417e                                                       | 2026-10-09                  |
| go.yaml.in/yaml/v3                       | latest stable at pinning time (verify with `go list -m -versions go.yaml.in/yaml/v3`)    |                             |
| Go toolchain                             | 1.27.3 (CI matrix 1.26 and 1.27)                                                         |                             |
| pgvector image                           | `pgvector/pgvector:0.8.7-pg17`                                                           | pgvector 0.8.7 (2026-10-01) |
| actions/checkout                         | `actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1` (pinned in momo-01) |                             |
| actions/setup-go                         | `actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0` (pinned in momo-01) |                             |

Notes: gorm.io/driver/sqlite v1.6.0 and gorm.io/driver/mysql v1.6.0 are not used by this plan. Bun runs on `*sql.DB` from pgx stdlib with `pgdialect` (the same driver stack as Momo's `FromSQLDB` adapter), not on `bun/driver/pgdriver`, so the comparison isolates the ORM layer. The sqlite-vec module version pins to tag v0.1.6; whether the bundled sqlite-vec is v0.1.6 is UNVERIFIED and is checked in Task 9 by printing `vec_version()`.

## Task 1: bench module, pinned dependencies and environment capture

Covers: S-19 (reproducible environment capture, pinned versions). Acceptance: `make bench` reproduces published results and `bench/results` records environment and pinned versions (checked by Task 11); this task adds `TestEnvCapture`.

Files:

- Create `bench/go.mod`: module `github.com/azrtydxb/momo/bench`, `go 1.26`, `replace github.com/azrtydxb/momo => ../`, requires for the versions in the version record.
- Create `bench/env/env.go` and `bench/env/env_test.go`.
- Create `bench/cmd/envcapture/main.go`: prints JSON from `env.Capture`.

Interfaces (exact):

```go
// bench/env
package env

type Environment struct {
	CapturedAtUTC  string            `json:"captured_at_utc"`
	GoVersion      string            `json:"go_version"`
	GOOS           string            `json:"goos"`
	GOARCH         string            `json:"goarch"`
	NumCPU         int               `json:"num_cpu"`
	GOMAXPROCS     int               `json:"gomaxprocs"`
	CPUModel       string            `json:"cpu_model"`
	Kernel         string            `json:"kernel"`
	Hostname       string            `json:"hostname"`
	VCSRevision    string            `json:"vcs_revision"`
	VCSModified    bool              `json:"vcs_modified"`
	ModuleVersions map[string]string `json:"module_versions"`
	PostgresVersion string           `json:"postgres_version"` // from SHOW server_version; empty when no DSN
	PgvectorVersion string           `json:"pgvector_version"` // from pg_extension.extversion; empty when no DSN
}

// Capture reads the environment. CPU model: /proc/cpuinfo "model name" on linux,
// `sysctl -n machdep.cpu.brand_string` on darwin, `wmic cpu get name` on windows.
// Postgres fields are filled by CapturePostgres when BENCH_PG_URL is set.
func Capture() (Environment, error)
func CapturePostgres(ctx context.Context, dsn string, e *Environment) error
```

- [ ] Step 1. Write `bench/env/env_test.go`:

```go
package env

import (
	"runtime"
	"testing"
)

func TestEnvCapture(t *testing.T) {
	e, err := Capture()
	if err != nil {
		t.Fatal(err)
	}
	if e.GoVersion == "" || e.GOOS != runtime.GOOS || e.GOARCH != runtime.GOARCH {
		t.Fatalf("toolchain fields wrong: %+v", e)
	}
	if e.NumCPU < 1 || e.GOMAXPROCS < 1 {
		t.Fatalf("cpu fields wrong: %+v", e)
	}
	if e.CapturedAtUTC == "" {
		t.Fatal("captured_at_utc empty")
	}
	if _, ok := e.ModuleVersions["github.com/jackc/pgx/v5"]; !ok {
		t.Fatalf("pinned module pgx/v5 missing from module_versions: %v", e.ModuleVersions)
	}
}
```

Expect FAIL with `undefined: Capture`.

- [ ] Step 2. Implement `env.go`: `Capture` uses `runtime`, `runtime/debug.ReadBuildInfo` (module versions from `bi.Deps` and the main module; VCS fields from `bi.Settings` keys `vcs.revision` and `vcs.modified`), `os.Hostname`, and the CPU command for the platform. `CapturePostgres` runs `SHOW server_version` and `SELECT extversion FROM pg_extension WHERE extname = 'vector'` through `pgx.Connect`. Run `cd bench && go test ./env -run TestEnvCapture -count=1`; expect `ok`.
- [ ] Step 3. Implement `bench/cmd/envcapture/main.go`, which calls `env.Capture()`, then `CapturePostgres` when `BENCH_PG_URL` is set, and prints indented JSON. Run `cd bench && go run ./cmd/envcapture | head -5`; expect a JSON object beginning with `{`.
- [ ] Step 4. Commit: `feat(bench): add bench module with pinned dependencies and environment capture`.

## Task 2: Workload definitions, canonical results and the Impl contract

Covers: S-19 (equivalence rules: identical rows per workload). Acceptance: `TestBenchEquivalence` is completed in Task 5 once all adapters exist; this task adds `TestCanonical` and `TestWorkloadSQLSemantics`.

Files:

- Create `bench/workload/model.go`: `Customer`, `Order`, `CustomerWithOrders`.
- Create `bench/workload/impl.go`: `Impl` interface, `Canonical` encoder.
- Create `bench/workload/seed.go`: deterministic seed (`math/rand` with seed 42), `Seed(ctx context.Context, pool *pgxpool.Pool, customers, ordersPerCustomer int) error`.
- Create `bench/workload/sql.go`: the exact SQL strings every handwritten or semantic reference uses.
- Create `bench/workload/canonical_test.go`: `TestCanonical`.

Interfaces (exact):

```go
package workload

type Customer struct {
	ID        int64
	Email     string
	Country   string
	Status    string
	CreatedAt time.Time // stored as timestamptz; canonical form truncates to microseconds in UTC
}
type Order struct {
	ID         int64
	CustomerID int64
	AmountCents int64
	Status     string
	CreatedAt  time.Time
}
type CustomerWithOrders struct {
	Customer Customer
	Orders   []Order // ordered by ID ASC
}

// Impl is implemented by every adapter. Each method has one SQL meaning, defined in sql.go.
type Impl interface {
	Name() string
	CustomerByID(ctx context.Context, id int64) (Customer, error)                       // ErrNotFound-equivalent: returns (Customer{}, ErrNoRow)
	RecentOrders(ctx context.Context, country string, limit int) ([]Order, error)       // ORDER BY o.created_at DESC, o.id DESC
	InsertOrder(ctx context.Context, o Order) (int64, error)                            // returns generated id
	SetCustomerStatus(ctx context.Context, id int64, status string) (int64, error)      // rows affected
	CustomersWithOrders(ctx context.Context, country string, limit int) ([]CustomerWithOrders, error) // customers ORDER BY id ASC LIMIT limit; orders per customer ORDER BY id ASC
	BulkInsertOrders(ctx context.Context, rows []Order) (int64, error)                  // rows affected
	VectorTopK(ctx context.Context, query []float32, k int) ([]int64, error)            // optional: adapters without vectors return ErrNotSupported
}

// ErrNoRow is returned by CustomerByID when no row matches; adapters map their own not-found
// error to it (for example momo.ErrNotFound, gorm.ErrRecordNotFound, pgx.ErrNoRows).
var ErrNoRow = errors.New("workload: no row")
var ErrNotSupported = errors.New("workload: operation not supported by this implementation")

// Canonical returns a stable textual form of rows: one line per row, fields separated by |,
// NULL as <nil>, times in UTC with microseconds, float64 with %.9g.
func Canonical[T any](rows []T) []string
```

- [ ] Step 1. Write `bench/workload/canonical_test.go`:

```go
package workload

import (
	"testing"
	"time"
)

func TestCanonical(t *testing.T) {
	ts := time.Date(2026, 11, 4, 12, 0, 0, 123456789, time.FixedZone("CET", 3600))
	got := Canonical([]Order{{ID: 2, CustomerID: 7, AmountCents: 1250, Status: "paid", CreatedAt: ts}})
	want := "2|7|1250|paid|2026-11-04T11:00:00.123456Z"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("Canonical = %q, want [%q]", got, want)
	}
}

func TestWorkloadSQLSemantics(t *testing.T) {
	for name, q := range map[string]string{
		"RecentOrders":           sqlRecentOrders,
		"CustomersWithOrders":    sqlCustomersWithOrders,
	} {
		if !containsAll(q, "ORDER BY") {
			t.Errorf("%s has no ORDER BY; equivalence needs deterministic ordering", name)
		}
	}
}
```

`containsAll` is a helper using `strings.Contains`. Expect FAIL with `undefined: Canonical`.

- [ ] Step 2. Implement `impl.go`, `model.go`, `sql.go` (constants `sqlRecentOrders`, `sqlCustomersWithOrders` and every other statement, each with explicit ORDER BY and LIMIT placeholders), and `seed.go`. `Canonical` formats each row through a `fmt.Stringer`-free encoder: reflect-free by requiring the row to implement `canonicalRow() []string` (generated by hand in `model.go` for the three types). Run `cd bench && go test ./workload -count=1`; expect `ok`.
- [ ] Step 3. Commit: `feat(bench): define workload contract and canonical result encoding`.

## Task 3: Handwritten pgx and database/sql adapters

Covers: S-19 (handwritten pgx/database-sql as the reference implementations). Acceptance: part of `TestBenchEquivalence` (Task 5).

Files:

- Create `bench/adapters/pgxhand/pgxhand.go`: `New(pool *pgxpool.Pool) workload.Impl`.
- Create `bench/adapters/sqlhand/sqlhand.go`: `New(db *sql.DB) workload.Impl` on pgx stdlib.
- Create `bench/adapters/pool.go` (package `adapters`): the shared pool limits and the two constructors below.

Interfaces (exact):

```go
// bench/adapters/pool.go (package adapters)
const (
	poolMaxConns        = 16
	poolMinConns        = 16
	poolMaxConnLifetime = time.Hour
)

// NewPool opens a pgxpool on url with MaxConns and MinConns 16, MaxConnLifetime 1h and
// QueryExecModeCacheStatement, and pings it. It fails on an empty url.
func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	if url == "" {
		return nil, errors.New("adapters: the PostgreSQL URL is empty (set BENCH_PG_URL)")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = poolMaxConns
	cfg.MinConns = poolMinConns
	cfg.MaxConnLifetime = poolMaxConnLifetime
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheStatement
	return pgxpool.NewWithConfig(ctx, cfg)
}

// ApplySQLDBSettings applies the same limits to a *sql.DB: MaxOpenConns and MaxIdleConns 16, ConnMaxLifetime 1h.
func ApplySQLDBSettings(db *sql.DB) {
	db.SetMaxOpenConns(poolMaxConns)
	db.SetMaxIdleConns(poolMaxConns)
	db.SetConnMaxLifetime(poolMaxConnLifetime)
}
```

Interfaces (exact): both adapters implement `workload.Impl`; `VectorTopK` runs `SELECT id FROM vec_items ORDER BY emb <=> $1 LIMIT $2` with `pgvector` text literal encoding via `vector.Vector.Value()` from the root module (plan momo-07 `vector` package) and `hnsw.ef_search` applied in a transaction through `db.WithSearch` (which calls `Tx.SetLocal`, rendering `set_config`) when the caller passes `EfSearch` through the adapter constructor option `WithEfSearch(int)`.

- [ ] Step 1. Write `bench/adapters/pgxhand/pgxhand_test.go` (`//go:build integration`, package `pgxhand_test`). A golden test seeds a fixed dataset, runs the read operations through the handwritten adapter, and compares the canonical lines with `bench/workload/testdata/golden/pgx.txt`. `UPDATE_GOLDEN=1` writes the file, so the first review step can commit it:

```go
//go:build integration

package pgxhand_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/bench/adapters"
	"github.com/azrtydxb/momo/bench/adapters/pgxhand"
	"github.com/azrtydxb/momo/bench/workload"
)

const goldenPath = "../../workload/testdata/golden/pgx.txt"

func TestPgxhandGolden(t *testing.T) {
	ctx := context.Background()
	pool, err := adapters.NewPool(ctx, os.Getenv("BENCH_PG_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := workload.Seed(ctx, pool, 200, 5); err != nil {
		t.Fatal(err)
	}
	impl := pgxhand.New(pool)

	customer, err := impl.CustomerByID(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	recent, err := impl.RecentOrders(ctx, "DE", 5)
	if err != nil {
		t.Fatal(err)
	}
	withOrders, err := impl.CustomersWithOrders(ctx, "DE", 3)
	if err != nil {
		t.Fatal(err)
	}

	var lines []string
	lines = append(lines, "customer:")
	lines = append(lines, workload.Canonical([]workload.Customer{customer})...)
	lines = append(lines, "recent:")
	lines = append(lines, workload.Canonical(recent)...)
	lines = append(lines, "with-orders:")
	for _, cw := range withOrders {
		lines = append(lines, workload.Canonical([]workload.Customer{cw.Customer})...)
		lines = append(lines, workload.Canonical(cw.Orders)...)
	}
	got := strings.Join(lines, "\n") + "\n"

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with UPDATE_GOLDEN=1 once): %v", err)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch:\n--- got\n%s--- want\n%s", got, want)
	}
}
```

`adapters.NewPool(ctx, url)` is the shared pool constructor in `bench/adapters/pool.go` (Task 3 Step 2) with the settings in `PoolConfig`; `workload.Seed` is in `bench/workload/seed.go`, which takes the pool. Expect FAIL with `undefined: pgxhand.New` before Step 2. Run `cd bench && BENCH_PG_URL=$URL MOMO_REQUIRE_INTEGRATION=1 UPDATE_GOLDEN=1 go test -tags integration ./adapters/pgxhand -count=1` once, review the golden file, then run without `UPDATE_GOLDEN`; expect `ok`.

- [ ] Step 2. Implement both adapters with the SQL strings from `bench/workload/sql.go`. `InsertOrder` uses `INSERT ... RETURNING id`. `BulkInsertOrders` uses one `INSERT ... VALUES` per 2000 rows (3 parameters each, below the 65535 limit). Run `BENCH_PG_URL=$URL MOMO_REQUIRE_INTEGRATION=1 go test -tags integration ./adapters/pgxhand ./adapters/sqlhand -count=1` from `bench/`; expect `ok`.
- [ ] Step 3. Commit: `feat(bench): add handwritten pgx and database/sql reference adapters`.

## Task 4: Momo and GORM adapters

Covers: S-19 (Momo and GORM in the comparison). Acceptance: part of `TestBenchEquivalence` (Task 5).

Files:

- Create `bench/models/momo/customer.go` and `bench/models/momo/order.go`: Momo models with `momo.Model` marker and tags (the generated `momo_gen.go` and subpackages come from `momo generate` run by `make bench-generate`).
- Create `bench/adapters/momo/momo.go`: `New(db *momo.DB) workload.Impl`, using the generated query API from plan momo-02 (`Select`, `Insert`, `Update`, `Load` relation loader from plan momo-06).
- Create `bench/adapters/gormad/gorm.go`: `New(db *gorm.DB) workload.Impl`, with `gorm.Config{SkipDefaultTransaction: true, PrepareStmt: false}` so GORM does not wrap single writes in implicit transactions (Momo does not do so either).
- Create `bench/adapters/gormad/gorm_test.go` (`//go:build integration`).
- Create `bench/adapters/gormad/open.go` (package `gormad`): `Open`, the GORM opener that the GORM adapter test and the equivalence test share. It imports `_ "github.com/jackc/pgx/v5/stdlib"` for the `pgx` database/sql driver.

Interfaces (exact):

```go
// bench/adapters/gormad/open.go (package gormad)
func Open(tb testing.TB, url string) *gorm.DB {
	tb.Helper()
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			tb.Error(err)
		}
	})
	adapters.ApplySQLDBSettings(sqlDB)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		tb.Fatal(err)
	}
	return db
}
```

Interfaces (exact): `momo.New(drv)` from the contract wraps `pgxdriver.New(pool)`; `GORM` uses `gorm.io/driver/postgres` opened with `postgres.New(postgres.Config{Conn: sqlDB})` where `sqlDB` is the pgx stdlib `*sql.DB` with the pool settings from Task 3. `CustomersWithOrders` for GORM uses `Preload("Orders", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") })` with `Where country = ? ORDER BY id ASC LIMIT ?` on the parent query.

- [ ] Step 1. Write `bench/adapters/gormad/gorm_test.go` (`//go:build integration`, package `gormad_test`). Three checks: a missing customer maps to `workload.ErrNoRow`, the recent-orders order matches the SQL contract, and a 1000-row bulk insert affects 1000 rows:

```go
//go:build integration

package gormad_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/azrtydxb/momo/bench/adapters"
	"github.com/azrtydxb/momo/bench/adapters/gormad"
	"github.com/azrtydxb/momo/bench/workload"
)


func TestGormAdapter(t *testing.T) {
	ctx := context.Background()
	gdb := gormad.Open(t, os.Getenv("BENCH_PG_URL"))
	pool, err := adapters.NewPool(ctx, os.Getenv("BENCH_PG_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := workload.Seed(ctx, pool, 50, 4); err != nil {
		t.Fatal(err)
	}
	impl := gormad.New(gdb)

	t.Run("missing customer maps to ErrNoRow", func(t *testing.T) {
		if _, err := impl.CustomerByID(ctx, 1<<40); !errors.Is(err, workload.ErrNoRow) {
			t.Fatalf("err = %v, want workload.ErrNoRow", err)
		}
	})

	t.Run("recent orders are newest first with id tie-break", func(t *testing.T) {
		orders, err := impl.RecentOrders(ctx, "DE", 20)
		if err != nil {
			t.Fatal(err)
		}
		for i := 1; i < len(orders); i++ {
			prev, cur := orders[i-1], orders[i]
			if prev.CreatedAt.Before(cur.CreatedAt) || (prev.CreatedAt.Equal(cur.CreatedAt) && prev.ID < cur.ID) {
				t.Fatalf("order %d (%v, id %d) sorts before %d (%v, id %d)", i-1, prev.CreatedAt, prev.ID, i, cur.CreatedAt, cur.ID)
			}
		}
	})

	t.Run("bulk insert of 1000 rows affects 1000 rows", func(t *testing.T) {
		rows := make([]workload.Order, 1000)
		for i := range rows {
			rows[i] = workload.Order{CustomerID: 1, AmountCents: int64(i), Status: "new", CreatedAt: time.Unix(1_700_000_000+int64(i), 0).UTC()}
		}
		n, err := impl.BulkInsertOrders(ctx, rows)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1000 {
			t.Fatalf("rows affected = %d, want 1000", n)
		}
	})
}
```

`adapters.ApplySQLDBSettings(*sql.DB)` applies the shared pool settings from `bench/adapters/pool.go`. Expect FAIL with `undefined: gormad.New` before Step 2. Run `cd bench && BENCH_PG_URL=$URL MOMO_REQUIRE_INTEGRATION=1 go test -tags integration ./adapters/gormad -count=1`; expect `ok`.

- [ ] Step 2. Implement both adapters. The Momo adapter uses `Select[momodel.Customer] on db`, `Insert[momodel.Order] on db.Models(...)`, and `Update` with the typed `Status.Set(...)`. Run `BENCH_PG_URL=$URL MOMO_REQUIRE_INTEGRATION=1 go test -tags integration ./adapters/momo ./adapters/gormad -count=1`; expect `ok`.
- [ ] Step 3. Commit: `feat(bench): add Momo and GORM adapters`.

## Task 5: Ent and Bun adapters and TestBenchEquivalence

Covers: S-19 (Ent and Bun in the comparison; equivalence rules). Acceptance: `TestBenchEquivalence`.

Files:

- Create `bench/adapters/ent/schema/customer.go`, `bench/adapters/ent/schema/order.go` (Ent schema definitions), generated code under `bench/adapters/ent/ent/` by `go generate ./adapters/ent` with `//go:generate go run entgo.io/ent/cmd/ent@v0.14.6 generate ./schema` (exact version pin), committed.
- Create `bench/adapters/ent/ent.go`: `New(client *ent.Client) workload.Impl`.
- Create `bench/adapters/bunad/bun.go`: `New(db *bun.DB) workload.Impl` using `pgdialect` on the pgx stdlib `*sql.DB`.
- Create `bench/equivalence_test.go` (`//go:build integration`): `TestBenchEquivalence`.

Interfaces (exact):

```go
// bench/equivalence_test.go
func TestBenchEquivalence(t *testing.T) // seeds a database, runs every Impl through every read workload
// and asserts workload.Canonical equality against the pgxhand reference. Writes are compared through a
// follow-up CustomerByID and a row count query. VectorTopK is compared only among adapters that report it.
```

Ent's `CustomersWithOrders` uses `WithOrders(func(q *ent.OrderQuery) { q.Order(ent.Asc(order.FieldID)) })` with `Where(customer.CountryEQ(...))` and `Order(ent.Asc(customer.FieldID)).Limit(limit)`. Bun uses `Relation("Orders", func(q *bun.SelectQuery) *bun.SelectQuery { return q.Order("id ASC") })` only when the adapter loads via `Relation`, with the parent query ordered by id ASC and limited.

- Create `bench/impls_test.go` (package `bench`): `openAllImpls`, which opens the six adapters on one pool. The equivalence test and the micro benchmarks both use it, so every comparison runs the same constructors.

Interfaces (exact):

```go
// bench/impls_test.go (package bench)
// openAllImpls returns the six adapters in this order: pgxhand (the reference), sqlhand, momo, gorm, ent, bun.
func openAllImpls(tb testing.TB, url string, pool *pgxpool.Pool) []workload.Impl {
	tb.Helper()
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			tb.Error(err)
		}
	})
	adapters.ApplySQLDBSettings(sqlDB)
	entClient := ent.NewClient(entad.Driver(entsql.OpenDB("postgres", sqlDB)))
	return []workload.Impl{
		pgxhand.New(pool),
		sqlhand.New(sqlDB),
		momoad.New(momo.New(pgxdriver.New(pool))),
		gormad.New(gormad.Open(tb, url)),
		ent.New(entClient),
		bunad.New(bun.NewDB(sqlDB, pgdialect.New())),
	}
}
```

- [ ] Step 1. Write `TestBenchEquivalence` in `bench/equivalence_test.go` (`//go:build integration`, package `bench`). It seeds one database, takes the handwritten pgx adapter as the reference, and checks that every other adapter returns identical canonical rows for every read, and that writes leave identical state:

```go
//go:build integration

package bench

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo/bench/adapters"
	"github.com/azrtydxb/momo/bench/workload"
)

// flattenWithOrders encodes customers with their orders in one canonical list.
func flattenWithOrders(cs []workload.CustomerWithOrders) []string {
	var out []string
	for _, cw := range cs {
		out = append(out, workload.Canonical([]workload.Customer{cw.Customer})...)
		out = append(out, workload.Canonical(cw.Orders)...)
	}
	return out
}

func TestBenchEquivalence(t *testing.T) {
	ctx := context.Background()
	url := os.Getenv("BENCH_PG_URL")
	pool, err := adapters.NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := workload.Seed(ctx, pool, 200, 5); err != nil {
		t.Fatal(err)
	}
	impls := openAllImpls(t, url, pool)
	ref, others := impls[0], impls[1:]
	for _, impl := range others {
		impl := impl
		t.Run(impl.Name(), func(t *testing.T) {
			for _, country := range []string{"DE", "FR"} {
				want, err := ref.RecentOrders(ctx, country, 20)
				if err != nil {
					t.Fatal(err)
				}
				got, err := impl.RecentOrders(ctx, country, 20)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(workload.Canonical(got), workload.Canonical(want)) {
					t.Fatalf("RecentOrders(%s) differs from pgx reference", country)
				}
			}

			wantCW, err := ref.CustomersWithOrders(ctx, "DE", 10)
			if err != nil {
				t.Fatal(err)
			}
			gotCW, err := impl.CustomersWithOrders(ctx, "DE", 10)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(flattenWithOrders(gotCW), flattenWithOrders(wantCW)) {
				t.Fatal("CustomersWithOrders differs from pgx reference")
			}

			// Writes: the same status change must be visible through the reference.
			n, err := impl.SetCustomerStatus(ctx, 2, "vip")
			if err != nil {
				t.Fatal(err)
			}
			if n != 1 {
				t.Fatalf("SetCustomerStatus affected %d rows, want 1", n)
			}
			c, err := ref.CustomerByID(ctx, 2)
			if err != nil {
				t.Fatal(err)
			}
			if c.Status != "vip" {
				t.Fatalf("reference sees status %q after %s write, want vip", c.Status, impl.Name())
			}
			if _, err := ref.SetCustomerStatus(ctx, 2, "active"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
```

`gormad.Open(t, url)` (`bench/adapters/gormad/open.go`) returns the `*gorm.DB` used in the GORM adapter test. `bunad`, `ent`, `momoad` and `sqlhand` expose `New(...)`. The VectorTopK comparison runs in `bench/vector`; it is not repeated here. Expect FAIL with `undefined: sqlhand.New` before Step 2 of Task 5. Run `cd bench && BENCH_PG_URL=$URL MOMO_REQUIRE_INTEGRATION=1 go test -tags integration ./ -run TestBenchEquivalence -count=1`; expect `ok` with every subtest passing.

- [ ] Step 2. Implement the Ent and Bun adapters. Run `BENCH_PG_URL=$URL MOMO_REQUIRE_INTEGRATION=1 go test -tags integration ./ -run TestBenchEquivalence -count=1`; expect `ok` with every subtest passing. A failing subtest prints the two canonical diffs; fix the adapter, not the expectation, unless the SQL meaning in `sql.go` is wrong, in which case change `sql.go` and every adapter together.
- [ ] Step 3. Commit: `feat(bench): add Ent and Bun adapters and equivalence test`.

## Task 6: ORM-only microbenchmarks with p50, p95 and p99

Covers: S-19 (ORM-only microbenchmarks; p50/p95/p99). Acceptance: part of `make bench` (Task 11).

Files:

- Create `bench/stats/percentile.go` and `bench/stats/percentile_test.go`: `TestPercentiles`.
- Create `bench/micro_test.go`: benchmarks for each operation and implementation.

Interfaces (exact):

```go
// bench/stats
// Percentile returns the nearest-rank percentile p (0 < p <= 100) of samples (copied and sorted).
func Percentile(samples []time.Duration, p float64) time.Duration
// Report sets p50_ns, p95_ns and p99_ns metrics on b from samples collected during the timed loop.
func Report(b *testing.B, samples []time.Duration)
```

Benchmarks in `micro_test.go` (each `b.Run(impl.Name()+"/"+op, ...)` over all six impls): `CustomerByID`, `RecentOrders` (limit 20), `InsertOrder`, `SetCustomerStatus`, `CustomersWithOrders` (limit 100), `BulkInsertOrders` (1000 rows). Each loop records `time.Since` per call into a preallocated `[]time.Duration` of length `b.N` and calls `stats.Report` after `b.StopTimer()`. Each benchmark calls `b.ReportAllocs()`.

- [ ] Step 1. Write `bench/stats/percentile_test.go`:

```go
package stats

import (
	"testing"
	"time"
)

func TestPercentiles(t *testing.T) {
	var s []time.Duration
	for i := 100; i >= 1; i-- {
		s = append(s, time.Duration(i))
	}
	cases := map[float64]time.Duration{50: 50, 95: 95, 99: 99, 100: 100}
	for p, want := range cases {
		if got := Percentile(s, p); got != want {
			t.Errorf("p%v = %v, want %v", p, got, want)
		}
	}
	if len(s) != 100 || s[0] != 100 {
		t.Fatal("Percentile must not reorder the caller's slice")
	}
}
```

Expect FAIL with `undefined: Percentile`.

- [ ] Step 2. Implement `percentile.go` (nearest rank: index = ceil(p/100 * n) - 1 on a sorted copy). Run `cd bench && go test ./stats -count=1`; expect `ok`.
- [ ] Step 3. Write `micro_test.go` (package `bench`) with the six operations over all six adapters. Each timed loop records one duration per call; `stats.Report` publishes p50, p95 and p99 after the loop:

```go
package bench

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/azrtydxb/momo/bench/adapters"
	"github.com/azrtydxb/momo/bench/stats"
	"github.com/azrtydxb/momo/bench/workload"
)

// benchImpls opens the six adapters on the shared pool; each benchmark calls it once.
func benchImpls(b *testing.B) []workload.Impl {
	b.Helper()
	url := os.Getenv("BENCH_PG_URL")
	pool, err := adapters.NewPool(context.Background(), url)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(pool.Close)
	return openAllImpls(b, url, pool)
}

func timed(b *testing.B, op func(i int) error) {
	b.Helper()
	samples := make([]time.Duration, 0, b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		if err := op(i); err != nil {
			b.Fatal(err)
		}
		samples = append(samples, time.Since(start))
	}
	b.StopTimer()
	stats.Report(b, samples)
}

func BenchmarkMicro(b *testing.B) {
	ctx := context.Background()
	for _, impl := range benchImpls(b) {
		impl := impl
		b.Run(impl.Name()+"/CustomerByID", func(b *testing.B) {
			timed(b, func(i int) error {
				_, err := impl.CustomerByID(ctx, int64(i%200+1))
				return err
			})
		})
		b.Run(impl.Name()+"/RecentOrders", func(b *testing.B) {
			timed(b, func(int) error {
				_, err := impl.RecentOrders(ctx, "DE", 20)
				return err
			})
		})
		b.Run(impl.Name()+"/InsertOrder", func(b *testing.B) {
			timed(b, func(i int) error {
				_, err := impl.InsertOrder(ctx, workload.Order{CustomerID: 1, AmountCents: int64(i), Status: "new", CreatedAt: time.Unix(1_700_000_000, 0).UTC()})
				return err
			})
		})
		b.Run(impl.Name()+"/SetCustomerStatus", func(b *testing.B) {
			timed(b, func(i int) error {
				_, err := impl.SetCustomerStatus(ctx, int64(i%200+1), "active")
				return err
			})
		})
		b.Run(impl.Name()+"/CustomersWithOrders", func(b *testing.B) {
			timed(b, func(int) error {
				_, err := impl.CustomersWithOrders(ctx, "DE", 100)
				return err
			})
		})
		b.Run(impl.Name()+"/BulkInsertOrders", func(b *testing.B) {
			rows := make([]workload.Order, 1000)
			for i := range rows {
				rows[i] = workload.Order{CustomerID: 1, AmountCents: int64(i), Status: "new", CreatedAt: time.Unix(1_700_000_000, 0).UTC()}
			}
			timed(b, func(int) error {
				_, err := impl.BulkInsertOrders(ctx, rows)
				return err
			})
		})
	}
}
```

`openAllImpls` is the shared constructor list used by the equivalence test (`bench/impls_test.go`), so each benchmark builds the same six adapters. Run `cd bench && BENCH_PG_URL=$URL go test -run '^$' -bench 'BenchmarkMicro' -benchtime=1x ./...`; expect each sub-benchmark to print a line containing `p99_ns`.

- [ ] Step 4. Commit: `feat(bench): add ORM microbenchmarks with percentile reporting`.

## Task 7: Real-database mixed workload with concurrency

Covers: S-19 (real-database workloads). Acceptance: part of `make bench` (Task 11).

Files:

- Create `bench/loadgen/loadgen.go`: `Run(ctx, impl workload.Impl, cfg Config) (Result, error)`.
- Create `bench/cmd/loadgen/main.go`: flags `-impl`, `-concurrency`, `-ops`, `-read-ratio`, `-seed`, `-out` (JSON lines).
- Create `bench/loadgen/loadgen_test.go` (`//go:build integration`): `TestLoadgenOutputSchema`.

Interfaces (exact):

```go
type Config struct {
	Concurrency int     // goroutines; default 8
	Ops         int     // total operations; default 20000
	ReadRatio   float64 // fraction of CustomerByID/RecentOrders/CustomersWithOrders calls; default 0.8
	Seed        int64   // default 42; drives key choice and the write payloads
}
type Result struct {
	Impl         string        `json:"impl"`
	Concurrency  int           `json:"concurrency"`
	Ops          int           `json:"ops"`
	ReadRatio    float64       `json:"read_ratio"`
	Throughput   float64       `json:"ops_per_second"`
	P50NS        int64         `json:"p50_ns"`
	P95NS        int64         `json:"p95_ns"`
	P99NS        int64         `json:"p99_ns"`
	Errors       int           `json:"errors"`
	PoolWaitNS   int64         `json:"pool_wait_ns"` // from pgxpool Stat().EmptyAcquireWaitTime when available, else 0
	StartedAtUTC string        `json:"started_at_utc"`
}
```

Pool wait is reported only for the native pgx adapter; the field is 0 for database/sql adapters.

- [ ] Step 1. Write `TestLoadgenOutputSchema` (integration, package `loadgen_test`, file `bench/loadgen/loadgen_integration_test.go`). It runs the generator on the handwritten pgx adapter and checks the JSON keys and the zero error count:

```go
//go:build integration

package loadgen_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/azrtydxb/momo/bench/adapters"
	"github.com/azrtydxb/momo/bench/adapters/pgxhand"
	"github.com/azrtydxb/momo/bench/loadgen"
	"github.com/azrtydxb/momo/bench/workload"
)

func TestLoadgenOutputSchema(t *testing.T) {
	ctx := context.Background()
	pool, err := adapters.NewPool(ctx, os.Getenv("BENCH_PG_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := workload.Seed(ctx, pool, 200, 5); err != nil {
		t.Fatal(err)
	}
	res, err := loadgen.Run(ctx, pgxhand.New(pool), loadgen.Config{Ops: 200, Concurrency: 4})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"impl", "ops_per_second", "p50_ns", "p95_ns", "p99_ns", "errors", "started_at_utc"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("output lacks key %q: %s", key, raw)
		}
	}
	if res.Errors != 0 {
		t.Fatalf("errors = %d, want 0", res.Errors)
	}
}
```

Expect FAIL with `undefined: Run` before Step 2. Run `cd bench && BENCH_PG_URL=$URL MOMO_REQUIRE_INTEGRATION=1 go test -tags integration ./loadgen -run TestLoadgenOutputSchema -count=1`; expect `ok`.

- [ ] Step 2. Implement `loadgen.go` (a fixed number of goroutines pulling operations from a channel seeded by `rand.New(rand.NewSource(seed))` per goroutine; latencies recorded per operation into per-goroutine slices merged after `wg.Wait()`) and `cmd/loadgen/main.go`. Run the Step 1 command; expect `ok`.
- [ ] Step 3. Commit: `feat(bench): add mixed real-database workload generator`.

## Task 8: Bulk insert paths: chunked VALUES, COPY and unnest

Covers: S-52 (PostgreSQL bulk insert via unnest array parameters, benchmarked against chunked VALUES and COPY), S-5 and S-9 (bulk path chunking and COPY, owned by plan momo-05 for the VALUES and COPY builders). Acceptance: `TestUnnestBulkInsert`.

Files:

- Create `insert_unnest.go` (root package `momo`): `func (q InsertQuery[T]) Unnest() InsertQuery[T]` and the statement renderer `renderUnnestInsert`.
- Create `insert_unnest_integration_test.go` (root, `//go:build integration`): `TestUnnestBulkInsert`.
- Create `bench/bulk_test.go`: `BenchmarkBulkInsert` with sub-benchmarks `values`, `copy`, `unnest` for 1000, 10000 and 40000 rows.

Interfaces (exact; the contract does not yet name these, see the final report):

```go
// insert_unnest.go (package momo)
// Unnest switches a multi-row insert to one statement that passes one array parameter per column:
//   INSERT INTO t (c1, c2, ...) SELECT * FROM unnest($1::bigint[], $2::text[], ...)
// PostgreSQL only; other dialects return ErrUnsupported. Column array types come from the
// schema column types recorded in Meta[T].TableSchema. Generated keys come back through RETURNING.
func (q InsertQuery[T]) Unnest() InsertQuery[T]
```

Array values are built column by column as `[]any`; pgx encodes each `[]any` against the array OID the statement declares, so the element types never pass through text.

- [ ] Step 1. Write `TestUnnestBulkInsert` (integration, package `momo_test`, file `insert_unnest_integration_test.go`). It inserts 40000 rows of three columns through three paths into one table, taking a snapshot after each: chunked VALUES, unnest (which must send exactly one statement), and COPY through the driver's `Copier`. The three snapshots must be identical:

```go
//go:build integration

package momo_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
)

type bulkRow struct {
	ID int64
	A  int64
	B  string
}

func (r *bulkRow) MomoMeta() *momo.Meta[bulkRow] {
	return &momo.Meta[bulkRow]{
		Table:   "bulk_rows",
		PK:      []int{0},
		Columns: []momo.ColumnMeta{{Name: "id"}, {Name: "a"}, {Name: "b"}},
		Ptr: func(r *bulkRow, i int) any {
			return [...]any{&r.ID, &r.A, &r.B}[i]
		},
		Value: func(r *bulkRow, i int) any {
			return [...]any{r.ID, r.A, r.B}[i]
		},
	}
}

// statementCounter counts the statements that reach the driver.
type statementCounter struct{ n int }

func (c *statementCounter) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context {
	c.n++
	return ctx
}

func (c *statementCounter) AfterQuery(context.Context, *momo.QueryEvent) {}

// snapshot returns the table's rows ordered by id.
func snapshot(t *testing.T, pool *pgxpool.Pool) [][3]any {
	t.Helper()
	rows, err := pool.Query(context.Background(), "SELECT id, a, b FROM bulk_rows ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out [][3]any
	for rows.Next() {
		var id, a int64
		var b string
		if err := rows.Scan(&id, &a, &b); err != nil {
			t.Fatal(err)
		}
		out = append(out, [3]any{id, a, b})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUnnestBulkInsert(t *testing.T) {
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "CREATE TABLE bulk_rows (id bigint PRIMARY KEY, a bigint NOT NULL, b text NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	truncate := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, "TRUNCATE bulk_rows"); err != nil {
			t.Fatal(err)
		}
	}

	rows := make([]bulkRow, 40000)
	for i := range rows {
		rows[i] = bulkRow{ID: int64(i + 1), A: int64(i * 3), B: "row"}
	}
	drv := pgxdriver.New(pool)

	// Path 1: chunked VALUES.
	if _, err := momo.Insert[bulkRow, *bulkRow](momo.New(drv)).Models(rows).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	valuesSnap := snapshot(t, pool)
	truncate()

	// Path 2: unnest, one statement.
	counter := &statementCounter{}
	if _, err := momo.Insert[bulkRow, *bulkRow](momo.New(drv, momo.WithHook(counter))).Unnest().Models(rows).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if counter.n != 1 {
		t.Fatalf("unnest path sent %d statements, want 1", counter.n)
	}
	if got := snapshot(t, pool); !reflect.DeepEqual(got, valuesSnap) {
		t.Fatal("unnest rows differ from the VALUES rows")
	}
	truncate()

	// Path 3: COPY through the driver's Copier.
	copier, ok := drv.(momo.Copier)
	if !ok {
		t.Fatal("pgx driver does not implement momo.Copier")
	}
	copyRows := make([][]any, len(rows))
	for i, r := range rows {
		copyRows[i] = []any{r.ID, r.A, r.B}
	}
	if _, err := copier.CopyFrom(ctx, "bulk_rows", []string{"id", "a", "b"}, copyRows); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(t, pool); !reflect.DeepEqual(got, valuesSnap) {
		t.Fatal("COPY rows differ from the VALUES rows")
	}
	if len(valuesSnap) != len(rows) {
		t.Fatalf("VALUES path wrote %d rows, want %d", len(valuesSnap), len(rows))
	}
}
```

Expect FAIL with `undefined: Unnest` before Step 2. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test -tags integration . -run TestUnnestBulkInsert -count=1`; expect `ok`.

- [ ] Step 2. Implement `renderUnnestInsert` and `Unnest`. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test -tags integration . -run TestUnnestBulkInsert -count=1`; expect `ok`.
- [ ] Step 3. Write `bench/bulk_test.go` (package `bench`): `BenchmarkBulkInsert` with sub-benchmarks `values`, `copy` and `unnest` at 1000, 10000 and 40000 rows. Each iteration truncates the table outside the timer:

```go
package bench

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
)

func BenchmarkBulkInsert(b *testing.B) {
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(testdb.PostgresURL(b))
	if err != nil {
		b.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "CREATE TABLE IF NOT EXISTS bulk_rows (id bigint PRIMARY KEY, a bigint NOT NULL, b text NOT NULL)"); err != nil {
		b.Fatal(err)
	}
	drv := pgxdriver.New(pool)
	for _, size := range []int{1000, 10000, 40000} {
		rows := make([]bulkRowBench, size)
		for i := range rows {
			rows[i] = bulkRowBench{ID: int64(i + 1), A: int64(i), B: "row"}
		}
		copyRows := make([][]any, size)
		for i, r := range rows {
			copyRows[i] = []any{r.ID, r.A, r.B}
		}
		for _, path := range []string{"values", "copy", "unnest"} {
			b.Run(fmt.Sprintf("%s/%d", path, size), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					if _, err := pool.Exec(ctx, "TRUNCATE bulk_rows"); err != nil {
						b.Fatal(err)
					}
					b.StartTimer()
					switch path {
					case "values":
						_, err = momo.Insert[bulkRowBench, *bulkRowBench](momo.New(drv)).Models(rows).Exec(ctx)
					case "unnest":
						_, err = momo.Insert[bulkRowBench, *bulkRowBench](momo.New(drv)).Unnest().Models(rows).Exec(ctx)
					case "copy":
						_, err = drv.(momo.Copier).CopyFrom(ctx, "bulk_rows", []string{"id", "a", "b"}, copyRows)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
```

The model type is declared in the same file, as the bench package cannot import the root test package:

```go
type bulkRowBench struct {
	ID int64
	A  int64
	B  string
}

func (r *bulkRowBench) MomoMeta() *momo.Meta[bulkRowBench] {
	return &momo.Meta[bulkRowBench]{
		Table:   "bulk_rows",
		PK:      []int{0},
		Columns: []momo.ColumnMeta{{Name: "id"}, {Name: "a"}, {Name: "b"}},
		Ptr:     func(r *bulkRowBench, i int) any { return [...]any{&r.ID, &r.A, &r.B}[i] },
		Value:   func(r *bulkRowBench, i int) any { return [...]any{r.ID, r.A, r.B}[i] },
	}
}
```

Run `cd bench && BENCH_PG_URL=$URL go test -run '^$' -bench BenchmarkBulkInsert -benchtime=1x ./...`; expect nine result lines.

- [ ] Step 4. Commit: `feat(momo): add unnest bulk insert path verified against VALUES and COPY`.

## Task 9: Recall-matched vector benchmarks

Covers: S-19 (recall-matched vector benchmarks), S-15 (approximate search comparison against exact search from plan momo-07). Acceptance: part of `make bench` (Task 11).

Files:

- Create `bench/vector/dataset.go`: seeded dataset generator (`math/rand`, seed 7, normalized float32 vectors).
- Create `bench/vector/recall.go`: exact top-k via pgvector exact search (`WithSearch(SearchSettings{ExactSearch: true})` on PostgreSQL), recall@k.
- Create `bench/vector/sweep.go`: sweep `hnsw.ef_search` in {10, 20, 40, 80, 160, 320, 640} and choose the smallest value whose recall@10 is at least 0.95.
- Create `bench/vector/recall_test.go`: `TestRecallMatchedSweep` (small fixed dataset: 2000 vectors, dimension 32) and `BenchmarkVectorTopK` for momo and raw pgx at the chosen setting.

Interfaces (exact):

```go
package vector

type Dataset struct {
	Dim     int
	Vectors [][]float32
	Queries [][]float32
}
func Generate(n, queries, dim int, seed int64) Dataset
// Recall returns the fraction of approximate ids that appear in the exact ids, averaged over queries.
func Recall(exact, approx [][]int64) float64
// Sweep returns the smallest ef_search from the list whose mean recall@k is at least target,
// together with the recall achieved. If no value reaches target it returns the largest value and its recall.
func Sweep(ctx context.Context, db workloadDB, ds Dataset, k int, target float64, efs []int) (ef int, recall float64, err error)
```

`workloadDB` is the minimal query interface both the momo and raw pgx paths satisfy (`QueryIDs(ctx, sql string, args ...any) ([]int64, error)` and `WithSearchEf(ctx, ef int, fn func() error) error`); the implementer defines it in `bench/vector/sweep.go`.

- [ ] Step 1. Write `TestRecallMatchedSweep` (integration, package `vector`, file `bench/vector/recall_integration_test.go`). It loads 2000 seeded vectors of dimension 32 with an HNSW index, runs `Sweep` with target recall 0.95 and k=10, and checks the recall and that two runs choose the same `ef_search`. The adapter runs on a one-connection pool, so the session setting it sets is the one every query sees:

```go
//go:build integration

package vector

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// pgSweepDB adapts a one-connection pool to the workloadDB interface used by Sweep.
type pgSweepDB struct {
	pool *pgxpool.Pool
}

func (d pgSweepDB) QueryIDs(ctx context.Context, sql string, args ...any) ([]int64, error) {
	rows, err := d.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (d pgSweepDB) WithSearchEf(ctx context.Context, ef int, fn func() error) error {
	if _, err := d.pool.Exec(ctx, "SELECT set_config('hnsw.ef_search', $1, false)", itoa(ef)); err != nil {
		return err
	}
	ferr := fn()
	if _, err := d.pool.Exec(ctx, "SELECT set_config('hnsw.ef_search', '40', false)"); err != nil && ferr == nil {
		return err
	}
	return ferr
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func TestRecallMatchedSweep(t *testing.T) {
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(os.Getenv("BENCH_PG_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ds := Generate(2000, 50, 32, 7)
	for _, stmt := range []string{
		"CREATE EXTENSION IF NOT EXISTS vector",
		"DROP TABLE IF EXISTS recall_items",
		"CREATE TABLE recall_items (id bigint PRIMARY KEY, emb vector(32) NOT NULL)",
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	for i, v := range ds.Vectors {
		text, err := vectorText(v)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO recall_items (id, emb) VALUES ($1, $2)", int64(i+1), text); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, "CREATE INDEX recall_items_emb ON recall_items USING hnsw (emb vector_l2_ops)"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE recall_items"); err != nil {
		t.Fatal(err)
	}

	db := pgSweepDB{pool: pool}
	efs := []int{10, 20, 40, 80, 160, 320, 640}
	ef1, recall, err := Sweep(ctx, db, ds, 10, 0.95, efs)
	if err != nil {
		t.Fatal(err)
	}
	if recall < 0.95 {
		t.Fatalf("recall@10 = %.3f at ef_search %d, want at least 0.95", recall, ef1)
	}
	ef2, _, err := Sweep(ctx, db, ds, 10, 0.95, efs)
	if err != nil {
		t.Fatal(err)
	}
	if ef1 != ef2 {
		t.Fatalf("sweep is not deterministic: %d then %d", ef1, ef2)
	}
}
```

`vectorText(v)` returns the pgvector text literal (`vector.Vector(v).Value()`), `vectorText` lives in `bench/vector/dataset.go`. Expect FAIL with `undefined: Generate` before Step 2. Run `cd bench && MOMO_REQUIRE_INTEGRATION=1 BENCH_PG_URL=$URL go test -tags integration ./vector -run TestRecallMatchedSweep -count=1`; expect `ok`.

- [ ] Step 2. Implement `dataset.go`, `recall.go`, `sweep.go`. Run `cd bench && MOMO_REQUIRE_INTEGRATION=1 BENCH_PG_URL=$URL go test -tags integration ./vector -run TestRecallMatchedSweep -count=1`; expect `ok`.
- [ ] Step 3. Implement `BenchmarkVectorTopK` in `bench/vector/topk_test.go` (package `vector`): two sub-benchmarks, `momo` and `pgxraw`, each at the `ef_search` that `Sweep` chose on the full dataset. The dataset size comes from `BENCH_VECTOR_N` (default 100000, dimension 384). Each sub-benchmark reports p50, p95 and p99 and the recall achieved:

```go
package vector

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo/bench/stats"
	"github.com/azrtydxb/momo/vector"
)

func datasetSize(b *testing.B) int {
	b.Helper()
	if s := os.Getenv("BENCH_VECTOR_N"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			b.Fatal(err)
		}
		return n
	}
	return 100000
}

func BenchmarkVectorTopK(b *testing.B) {
	ctx := context.Background()
	n := datasetSize(b)
	cfg, err := pgxpool.ParseConfig(os.Getenv("BENCH_PG_URL"))
	if err != nil {
		b.Fatal(err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(pool.Close)
	ds := Generate(n, 100, 384, 7)
	loadItems(b, ctx, pool, ds) // creates topk_items with an HNSW index; defined in topk_setup_test.go
	db := pgSweepDB{pool: pool}
	ef, recall, err := Sweep(ctx, db, ds, 10, 0.95, []int{10, 20, 40, 80, 160, 320, 640})
	if err != nil {
		b.Fatal(err)
	}
	for _, name := range []string{"momo", "pgxraw"} {
		b.Run(name, func(b *testing.B) {
			samples := make([]time.Duration, 0, b.N)
			b.ReportAllocs()
			b.ResetTimer()
			err := db.WithSearchEf(ctx, ef, func() error {
				for i := 0; i < b.N; i++ {
					q := vector.Vector(ds.Queries[i%len(ds.Queries)])
					start := time.Now()
					if _, err := db.QueryIDs(ctx, topKSQL(name), q); err != nil {
						return err
					}
					samples = append(samples, time.Since(start))
				}
				return nil
			})
			if err != nil {
				b.Fatal(err)
			}
			b.StopTimer()
			stats.Report(b, samples)
			b.ReportMetric(recall, "recall@10")
		})
	}
}
```

`loadItems` (inserting the dataset into `topk_items` with the HNSW index) and `topKSQL(name)` (`SELECT id FROM topk_items ORDER BY emb <-> $2 LIMIT 10` for `pgxraw`, and the same statement produced by the momo query builder for `momo`) are defined in `bench/vector/topk_setup_test.go`. Run `cd bench && BENCH_VECTOR_N=2000 BENCH_PG_URL=$URL go test -run '^$' -bench BenchmarkVectorTopK -benchtime=1x ./vector`; expect two result lines, each with `recall@10`.

- [ ] Step 4. Commit: `feat(bench): add recall-matched vector benchmarks`.

## Task 10: Generated-code size and compile cost

Covers: S-19 (generated-code size and compile cost). Acceptance: part of `make bench` (Task 11).

Files:

- Create `bench/codesize/codesize.go`: measures generated file bytes and compile time for `bench/models/momo` (the generated files from `momo generate`).
- Create `bench/codesize/codesize_test.go`: `TestCodeSizeReport`.

Interfaces (exact):

```go
package codesize

type Report struct {
	GeneratedFiles      int   `json:"generated_files"`
	GeneratedBytes      int64 `json:"generated_bytes"`
	GeneratedLines      int   `json:"generated_lines"`
	ModelPackageCompileMS int64 `json:"model_package_compile_ms"` // median of 5 `go build` runs after a content nonce change
	BinaryBytesDelta    int64 `json:"binary_bytes_delta"`       // examples binary with generated code minus the same binary without the models import
}
func Measure(ctx context.Context, moduleDir string) (Report, error)
```

- [ ] Step 1. Write `TestCodeSizeReport` in `bench/codesize/codesize_test.go` (package `codesize`). It measures the generated package and checks that every generated file carries the generator header:

```go
package codesize

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeSizeReport(t *testing.T) {
	dir, err := filepath.Abs("../models/momo")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Measure(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if rep.GeneratedFiles == 0 {
		t.Fatal("no generated files found")
	}
	if rep.GeneratedBytes == 0 {
		t.Fatal("generated files are empty")
	}
	if rep.ModelPackageCompileMS == 0 {
		t.Fatal("compile time was not measured")
	}
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(string(raw), "// Code generated by momo") && !strings.Contains(string(raw), "DO NOT EDIT") {
			t.Errorf("%s: generated header lacks DO NOT EDIT", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
```

Expect FAIL with `undefined: Measure` before Step 2. Run `cd bench && go test ./codesize -count=1`; expect `ok`.

- [ ] Step 2. Implement `Measure`: walk the directory for files whose first line starts with `// Code generated by momo`; compile time is the median of five `go build ./bench/models/momo` runs, each preceded by appending a unique comment nonce to a copy of the package in a temporary directory created with `os.MkdirTemp` under the scratch area (not the repository). Run `cd bench && go test ./codesize -count=1`; expect `ok`.
- [ ] Step 3. Commit: `feat(bench): measure generated code size and compile cost`.

## Task 11: Reproducible `make bench`, results layout and docs/PERFORMANCE.md

Covers: S-19 (reproducible commands, environment, raw results, repeated runs, noise-aware regression policy, measured budgets). Acceptance: `make bench` reproduces published results; `bench/results` contains raw output, environment and pinned versions.

Files:

- Modify the root `Makefile` (created by momo-01): add targets `bench`, `bench-smoke`, `bench-generate`, `bench-baseline`.
- Create `bench/scripts/run.sh`: runs the suite for `full` (dedicated machine) or `smoke` (CI).
- Create `bench/results/README.txt`: the directory layout (produced files and their meaning). Not a report; it describes the layout only.
- Create `bench/scripts/run_test.go`: `TestBenchResultsLayout`.
- Create `docs/PERFORMANCE.md`: methodology, environment, version table (copied from `versions.txt` of the reference run), budget table, regression policy, reproduction steps.

Interfaces (exact):

```make
bench:            ## full run on the dedicated machine: BENCH_COUNT=10
	./bench/scripts/run.sh full
bench-smoke:      ## CI smoke: one iteration per benchmark
	./bench/scripts/run.sh smoke
bench-generate:   ## regenerate bench/models/momo with momo generate
	cd bench && go run ../cmd/momo generate ./models/momo
bench-baseline:   ## write bench/baseline/allocs.txt from the allocation gate runs
	./bench/scripts/run.sh baseline
```

`run.sh` writes to `bench/results/<UTC timestamp>-<goos>-<goarch>/`: `env.json` (from `bench/cmd/envcapture`), `versions.txt` (`cd bench && go list -m all`), `raw.txt` (`go test -run '^$' -bench . -benchmem -count $BENCH_COUNT ./...` with `BENCH_PG_URL` exported), and `summary.txt` (`benchstat raw.txt`). `smoke` uses `-count 1 -benchtime 1x`. The run never writes outside `bench/results`.

Regression policy (written into `docs/PERFORMANCE.md`): a timing regression is flagged when benchstat reports a change above 10% with p < 0.05 on the dedicated machine across two consecutive full runs; allocation regressions are flagged above 5% with p < 0.05 in CI (Task 12); budgets are the median of the reference run plus 10% and are reviewed on every full run.

- [ ] Step 1. Write `bench/scripts/run_test.go`:

```go
package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBenchResultsLayout(t *testing.T) {
	out := t.TempDir()
	cmd := exec.Command("bash", "run.sh", "smoke")
	cmd.Env = append(os.Environ(), "BENCH_RESULTS_DIR="+out, "BENCH_VECTOR_N=2000")
	cmd.Dir = "."
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run.sh smoke: %v\n%s", err, b)
	}
	matches, err := filepath.Glob(filepath.Join(out, "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("want one run directory, got %v", matches)
	}
	for _, f := range []string{"env.json", "versions.txt", "raw.txt", "summary.txt"} {
		if _, err := os.Stat(filepath.Join(matches[0], f)); err != nil {
			t.Fatalf("missing %s: %v", f, err)
		}
	}
}
```

Expect FAIL with `run.sh: No such file or directory`.

- [ ] Step 2. Write `run.sh` (bash, `set -euo pipefail`, honours `BENCH_RESULTS_DIR`, default `bench/results`) and the Makefile targets above. The timestamp directory name is `date -u +%Y%m%dT%H%M%SZ`. Run `cd bench/scripts && go test -count=1 .`; expect `ok`.
- [ ] Step 3. Run `make bench` on the dedicated machine once with `BENCH_PG_URL` set to the local `pgvector/pgvector:0.8.7-pg17` service. Commit the resulting `bench/results/<run>/` directory (`env.json`, `versions.txt`, `raw.txt`, `summary.txt`) in a separate commit with the message body listing the machine from `env.json`.
- [ ] Step 4. Write `docs/PERFORMANCE.md` with: the methodology (identical pool settings `MaxConns=16`; GORM `SkipDefaultTransaction`; Bun on pgx stdlib; no ORM caches; `ANALYZE` after seeding); the version table; the budget table filled from the committed run (the S-18 `BenchmarkSelectNoHooks` budget is recorded in this file when the S-18 owner adds it); the regression policy above; and the reproduction commands `make db-up` and `make bench`. Add one row `| [Performance](PERFORMANCE.md) | methodology, pinned versions, budgets and regression policy |` to the page index in `docs/README.md`. Run `go test ./internal/repocheck -count=1` (momo-01 docs checks); expect `ok`.
- [ ] Step 5. Commit: `feat(bench): add reproducible make bench with results layout and performance policy`.

## Task 12: Allocation gate in CI with benchstat

Covers: S-58 (benchstat allocation gate in CI; timing regressions judged only on the dedicated machine). Acceptance: `TestBenchGateConfig`.

Files:

- Create `bench/alloc_test.go`: `BenchmarkAllocs_*` for each impl and operation (`CustomerByID`, `InsertOrder`, `SetCustomerStatus`, `RecentOrders` at limit 20), each with `b.ReportAllocs()`.
- Create `bench/cmd/benchgate/main.go` and `bench/cmd/benchgate/gate_test.go`: `TestBenchGate`.
- Create `bench/baseline/allocs.txt`: written by `make bench-baseline` on the CI runner, committed in a separate PR.
- Create `.github/workflows/bench-gate.yml`.
- Create `bench/gate_config_test.go`: `TestBenchGateConfig`.

Interfaces (exact):

```go
// bench/cmd/benchgate: reads benchstat text output on stdin or from a file argument;
// exits 1 when any allocs/op row has a positive delta above -max-percent (default 5) with p < 0.05.
// Usage: benchgate -max-percent 5 delta.txt
```

Workflow (`.github/workflows/bench-gate.yml`), using the pins from momo-01: `on: { pull_request: {}, workflow_dispatch: {} }`; `permissions: { contents: read }`; one job `allocs` on ubuntu-latest, `timeout-minutes: 20`, `services.postgres` with `pgvector/pgvector:0.8.7-pg17` and `POSTGRES_PASSWORD: momo`; steps: checkout (pinned SHA, `persist-credentials: false`), setup-go (pinned SHA, `go-version: '1.27'`), `go install golang.org/x/perf/cmd/benchstat@v0.0.0-20261009192801-be2c69fb417e`, `cd bench && go test -run '^$' -bench 'BenchmarkAllocs_' -benchmem -count=6 ./... | tee new.txt`, `benchstat -filter '.unit:allocs/op' bench/baseline/allocs.txt new.txt > delta.txt`, `go run ./bench/cmd/benchgate -max-percent 5 delta.txt`. A second step prints `ns/op` for information only and is not gated.

- [ ] Step 1. Write `bench/gate_config_test.go`:

```go
package bench

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestBenchGateConfig(t *testing.T) {
	raw, err := os.ReadFile("../.github/workflows/bench-gate.yml")
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string `yaml:"name"`
				Run  string `yaml:"run"`
				Uses string `yaml:"uses"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatal(err)
	}
	job, ok := wf.Jobs["allocs"]
	if !ok {
		t.Fatal("job allocs missing")
	}
	var script strings.Builder
	sha := regexp.MustCompile(`@[0-9a-f]{40}`)
	for _, s := range job.Steps {
		script.WriteString(s.Run + "\n")
		if s.Uses != "" && !sha.MatchString(s.Uses) {
			t.Errorf("action %s is not pinned by commit SHA", s.Uses)
		}
	}
	for _, want := range []string{"benchstat", "-filter '.unit:allocs/op'", "benchgate", "-max-percent 5"} {
		if !strings.Contains(script.String(), want) {
			t.Errorf("gate script lacks %q", want)
		}
	}
	if strings.Contains(script.String(), "ns/op") && strings.Contains(script.String(), "benchgate -max-percent 5 new") {
		t.Error("timings must not be gated in CI")
	}
}
```

Expect FAIL with `open ../.github/workflows/bench-gate.yml: no such file or directory`.

- [ ] Step 2. Create `.github/workflows/bench-gate.yml` as described, implement `bench/cmd/benchgate/main.go` with `gate(r io.Reader, maxPct float64) (failed bool, err error)`, and write `bench/cmd/benchgate/gate_test.go` (package `main`) with `TestBenchGate`. The two fixtures are benchstat outputs: `testdata/regress.txt` has an allocation regression of +25% with p=0.002, and `testdata/flat.txt` has no significant change:

```go
package main

import (
	"os"
	"strings"
	"testing"
)

func TestBenchGate(t *testing.T) {
	cases := []struct {
		file     string
		wantFail bool
	}{
		{"testdata/regress.txt", true},
		{"testdata/flat.txt", false},
	}
	for _, c := range cases {
		raw, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		failed, err := gate(strings.NewReader(string(raw)), 5)
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if failed != c.wantFail {
			t.Errorf("%s: failed = %v, want %v", c.file, failed, c.wantFail)
		}
	}
}
```

The fixture `testdata/regress.txt` contains a row in benchstat format, `Allocs_CustomerByID/momo-8  120.0 ± 0%  150.0 ± 0%  +25.00%  (p=0.002 n=6+6)`, and `testdata/flat.txt` contains `Allocs_CustomerByID/momo-8  120.0 ± 0%  120.0 ± 0%  ~  (p=0.400 n=6+6)`. Expect FAIL with `undefined: gate` before Step 2's implementation. Run `cd bench && go test ./cmd/benchgate -count=1`; expect `ok`.

- [ ] Step 3. Generate the baseline: run the `allocs` job via `workflow_dispatch` on the default branch, download the `new.txt` artifact (add an `actions/upload-artifact` step pinned by SHA), commit it as `bench/baseline/allocs.txt` in a separate PR, and rerun the job on that PR; expect the gate to pass.
- [ ] Step 4. Commit: `ci(bench): gate allocation regressions with benchstat`.

## Verification of this plan

- [ ] `cd bench && go test ./... -count=1` is green with no database URL (integration tests skip).
- [ ] `BENCH_PG_URL=$URL MOMO_REQUIRE_INTEGRATION=1 go test -tags integration ./... -count=1` from `bench/` is green, including `TestBenchEquivalence`.
- [ ] `make bench-smoke` finishes within the 20-minute CI budget of the `bench-smoke` job in ci.yml.
- [ ] `go list -deps github.com/azrtydxb/momo` shows none of gorm, ent, bun or benchstat.
- [ ] `docs/PERFORMANCE.md` budget values match the committed `bench/results/<run>/summary.txt` of the reference run.
