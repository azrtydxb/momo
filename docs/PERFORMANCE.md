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

The `bench/` module holds the workload contract (`bench/workload`), the shared
connection settings and the handwritten pgx and database/sql reference
adapters (`bench/adapters`), and the environment capture (`bench/env`,
`make bench-env`). Pool settings are identical for every adapter: 16
connections, one hour connection lifetime. Timings are judged only on a
dedicated machine; shared CI runners are too noisy for them.
