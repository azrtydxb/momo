# momo-05 Task 23: Documented type mapping with round-trip test per mapping (S-31)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -
Carried: 002-generator-from-go-structs-with-its-type-prerequisites — Red-first criterion unverifiable (mapping pre-existed) and S-31 requires every dialect; only PostgreSQL exists. Carried until SQLite and MySQL land.

## Description

Implements plan momo-05-writes-queries Task 23: Documented type mapping with round-trip test per mapping (S-31) (`.procoder/plans/momo-05-writes-queries.md:1896`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-31. Files: `docs/TYPE_MAPPING.md`, `type_mapping_test.go`.

## Acceptance criteria

- [ ] `TestTypeMapping` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-31] `TestTypeMapping` round-trips one value (including edge values: max int64, NaN where legal, pre-1970 time, non-UTC zone) per mapping in the documented table on both dialects; fails if a documented mapping is untested.
- [x] `procoder check` gate is clean and the task's commit is made (gate: 0 blocking; make fmt-check vet staticcheck gosec tidy-check exit 0)

## Evidence

Criteria 1 and 2 stay unticked. Criterion 1: the plan's literal test body calls
driver APIs that do not exist (`db.Driver().Exec`) and a SQLite-keyed DDL map, so
the test was adapted: `TestTypeMappingDoc` (doc sync, `type_mapping_test.go`) and
`TestTypeMapping` (live PostgreSQL round trip, `type_mapping_integration_test.go`,
build tag `integration`) plus `TestTypeMappingNaN`. Both failed first with
`open docs/TYPE_MAPPING.md: no such file or directory` and pass now.
Criterion 2: PostgreSQL is done (one subtest per documented row, edge values
incl. MaxInt64, NaN/Inf, pre-1970, +05:30 zone; mutation-checked: changing the
int32 row or deleting the `[]string` row fails `TestTypeMappingDoc`). The SQLite
half cannot run: no SQLite driver or `testopen` opener exists in this build, and
the plan's SQLite NaN `ErrValidation` float64 codec validator does not exist.
`TestTypeMapping` skips the SQLite dialect explicitly once an opener registers it.

