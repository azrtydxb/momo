# Performance

Budgets are measured, never aspirational. This page starts with the hook
overhead budget that `TestSelectNoHooksAllocBudget` enforces. The `bench/`
module (a separate Go module, so GORM, Ent and Bun never enter the library's
import graph) holds the comparison workloads; its methodology, versions and
regression policy are added here as the suite lands.

## Hook overhead budget

A primary-key lookup on a database with no hooks registered must not allocate
more than the budget below. The figure is `allocs/op` of
`BenchmarkSelectNoHooks`, measured by the `TestSelectNoHooksAllocBudget` run in CI. The full CI matrix
(Go 1.26 and 1.27, PostgreSQL 16 and 17 with pgvector 0.8.7) measures the same
32, so one budget covers every combination. The test fails when the measured
value rises above the budget, so an allocation added to the disabled-hook path
is caught. Lower the budget in the same change whenever the path gets cheaper.

| Benchmark                        | Budget |
| -------------------------------- | ------ |
| BenchmarkSelectNoHooks allocs/op | 32     |

To reproduce, run `make db-up`, then `make bench-hooks`. The benchmark opens a database, so it only compiles with the `integration` build tag and `make bench-smoke` skips it; the `bench-module` CI job runs `make bench-hooks` against the pgvector service.

## Comparison suite

The `bench/` module compares Momo with handwritten pgx and database/sql, GORM,
Ent and Bun on one PostgreSQL server. Every operation is defined once in
`bench/workload` and each library implements it in `bench/adapters`; the
equivalence test `TestBenchEquivalence` fails when an adapter returns different
rows than the handwritten pgx reference or leaves different state after a
write, so a workload whose SQL meaning differs between libraries cannot be
published.

### Methodology

- Identical pool settings for every adapter: 16 connections, one hour
  connection lifetime. The native pgx path uses the statement cache; the other
  libraries run on the pgx standard library driver.
- GORM runs with `SkipDefaultTransaction`, as Momo adds no implicit
  transaction. Bun runs on the same pgx standard library `database/sql` handle
  as Ent and GORM, so the comparison isolates the ORM layer. No library cache
  is enabled.
- The data is deterministic: a fixed seed, 200 customers with 5 orders each.
  `ANALYZE` runs after seeding.
- ORM-only microbenchmarks report p50, p95 and p99 per call beside allocations,
  for six operations over all six libraries. A mixed read and write workload
  with concurrency and the bulk insert paths have their own runners.
- Bulk insert compares chunked VALUES, COPY and Momo's `Unnest`, one statement
  with one array parameter per column, at 1000, 10000 and 40000 rows.
- Vector search is compared at equal recall: a sweep picks the smallest
  `hnsw.ef_search` whose recall at 10 reaches 0.95 against exact search, and both
  the Momo builder and the handwritten statement then run at that setting.
- Generated-code size and compile cost of the benchmark models are measured by
  `bench/codesize`.

### Pinned versions

The module versions are pinned in `bench/go.mod`; each run records them in
`versions.txt` and `env.json`, together with the Go toolchain, the machine, the
PostgreSQL version and the pgvector version. The database image is
`pgvector/pgvector:0.8.7-pg17`.

### Reproducing a run

`make db-up` starts PostgreSQL with pgvector. Then `make bench` runs the whole
suite ten times per benchmark and writes `bench/results/` under a directory named
by UTC time, operating system and architecture, with `env.json`, `versions.txt`,
`codesize.json`, `raw.txt` and `summary.txt`, plus `vector-encoding.txt` (the
`BenchmarkVectorEncoding` run of text against binary pgx vector codecs) when a
database is configured. `make bench-suite-smoke` runs one
iteration of everything and is what CI uses. `make bench-generate` regenerates
the Momo metadata and the Ent client.

### Regression policy

- Timings are judged only on a dedicated machine: a regression is flagged when
  benchstat reports a change above 10 percent with p below 0.05 across two
  consecutive full runs. Shared CI runners are too noisy for timings, so CI never
  gates on them.
- Allocation counts are deterministic, so CI gates them: the `bench-module`
  job of `ci.yml` and `ci-agent.yml` runs `make bench-gate`, which runs the
  allocation benchmarks, compares allocs/op with `bench/baseline/allocs.txt`
  using benchstat and fails above 5 percent with p below 0.05. The gate runs in
  that one job only, against the PostgreSQL service the job already starts, so
  a pull request pays for one database job and not a second one. The recipe
  runs under bash with pipefail, and `benchgate` exits 2 (never 0) when the new
  run has no allocs/op rows or lacks a benchmark that the baseline has, so a
  run that never happened cannot pass. `make bench-gate` runs the same
  comparison locally and `make bench-baseline` rewrites the baseline.
- Budgets are the median of the reference run plus 10 percent and are reviewed
  on every full run.

### Status of the published numbers

No full run on a dedicated machine has been committed yet, so this page records
no comparative timings. The only budget in force is the hook overhead budget
above, which CI enforces. Timing figures will be added from the stored
`bench/results/` run, never from a shared runner.
