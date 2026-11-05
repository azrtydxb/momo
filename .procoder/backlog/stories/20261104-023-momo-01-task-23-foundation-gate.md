# momo-01 Task 23: Foundation gate

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 23: Foundation gate (`.procoder/plans/momo-01-foundation.md:2241`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan.

## Acceptance criteria

- [x] Step 1: Run the unit gate. `make fmt-check vet tidy-check test test-race test-compilefail`. Expected: every target exits 0.
- [x] Step 2: Run the fuzz smoke. `make test-fuzz FUZZTIME=10s`. Expected: `FuzzBuilder`, `FuzzSchemaSort` and `FuzzNormalizeType` each complete without a failure.
- [x] Step 3: Run the PostgreSQL integration gate. `make db-up`, then `make test-integration-postgres`. Expected: exit 0; the suite is empty of dialect subtests until momo-03 adds the PostgreSQL entry to `testopen.Each`. Then `make db-down`.
- [x] Step 4: Confirm the contract names exist. Run `go doc github.com/azrtydxb/momo Querier`, `go doc github.com/azrtydxb/momo Error`, `go doc github.com/azrtydxb/momo/schema Load` and `go doc github.com/azrtydxb/momo/dialect MySQLFlavor`. Each prints a declaration.
- [x] `procoder check` gate is clean

## Evidence

- Step 1: `make fmt-check vet tidy-check test test-race test-compilefail` exited 0; `go mod tidy -diff` clean, all 8 packages `ok` under test and test-race, `ok github.com/azrtydxb/momo 0.542s` for TestCompileFail.
- Step 2: `make test-fuzz FUZZTIME=10s` exited 0; `ok github.com/azrtydxb/momo 10.480s` (FuzzBuilder), `ok .../schema 10.545s` (FuzzSchemaSort), `ok .../schema 11.372s` (FuzzNormalizeType).
- Step 3: `make db-up` exited 0 (postgres16, postgres17, pgbouncer Healthy); `make test-integration-postgres` exited 0, every package `ok`, no dialect subtests; `make db-down` removed the containers and network.
- Step 4: `go doc` printed `type Querier interface`, `type Error struct`, `func Load(r io.Reader) (*Schema, error)` and `func MySQLFlavor(f Flavor) *Dialect`.
- `procoder check` gate is clean: see the format and commit gate run at close.

