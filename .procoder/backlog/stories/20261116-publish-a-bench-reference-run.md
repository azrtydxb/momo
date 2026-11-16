# Publish a bench reference run

Status: open
Created: 2026-11-16
Epic: -
Sprint: -

## Description

The benchmark tooling (`make bench`, bench/scripts/run.sh, the Ent, Bun, GORM, pgx and momo adapters, `BenchmarkVectorEncoding`) is shipped and its output layout is enforced by `TestBenchResultsLayout`. What is missing is a published reference run: numbers measured on dedicated hardware, as docs/PERFORMANCE.md requires, committed so readers can compare and reproduce them. Shared CI runners are not acceptable for these numbers, and no agent environment has dedicated hardware, so the user runs it. Split out of momo-07 Task 8 and momo-12 Task 5 by the user's decision of 2026-11-16 in `.procoder/ask/decisions.md`.

## Acceptance criteria

- [ ] `make bench` is run on a dedicated machine with `BENCH_PG_URL` set, and its run directory (raw output, environment, pinned versions, `vector-encoding.txt`) is committed under bench/results.
- [ ] docs/PERFORMANCE.md links the committed run and states the hardware it ran on.

## Evidence
