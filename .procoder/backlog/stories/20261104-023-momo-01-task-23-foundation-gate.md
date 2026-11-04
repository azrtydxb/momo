# momo-01 Task 23: Foundation gate

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: -

## Description

Implements plan momo-01-foundation Task 23: Foundation gate (`.procoder/plans/momo-01-foundation.md:2241`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan.

## Acceptance criteria

- [ ] Step 1: Run the unit gate. `make fmt-check vet tidy-check test test-race test-compilefail`. Expected: every target exits 0.
- [ ] Step 2: Run the fuzz smoke. `make test-fuzz FUZZTIME=10s`. Expected: `FuzzBuilder`, `FuzzSchemaSort` and `FuzzNormalizeType` each complete without a failure.
- [ ] Step 3: Run the PostgreSQL integration gate. `make db-up`, then `make test-integration-postgres`. Expected: exit 0; the suite is empty of dialect subtests until momo-03 adds the PostgreSQL entry to `testopen.Each`. Then `make db-down`.
- [ ] Step 4: Confirm the contract names exist. Run `go doc github.com/azrtydxb/momo Querier`, `go doc github.com/azrtydxb/momo Error`, `go doc github.com/azrtydxb/momo/schema Load` and `go doc github.com/azrtydxb/momo/dialect MySQLFlavor`. Each prints a declaration.
- [ ] `procoder check` gate is clean

## Evidence

