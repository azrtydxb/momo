# momo-05 Task 23: Documented type mapping with round-trip test per mapping (S-31)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-05-writes-queries Task 23: Documented type mapping with round-trip test per mapping (S-31) (`.procoder/plans/momo-05-writes-queries.md:1896`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-31. Files: `docs/TYPE_MAPPING.md`, `type_mapping_test.go`.

## Acceptance criteria

- [ ] `TestTypeMapping` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-31] `TestTypeMapping` round-trips one value (including edge values: max int64, NaN where legal, pre-1970 time, non-UTC zone) per mapping in the documented table on both dialects; fails if a documented mapping is untested.
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

### Sprint 7

- Criterion 2 ticked: TestTypeMapping now also round-trips every documented SQLite column (rows marked unsupported/n/a skipped; NaN skipped on SQLite) on a live SQLite; TestTypeMappingNaNSQLite pins that NaN is stored as NULL and +Inf survives. MySQL: the document lists every Go type as unsupported (the generator emits no MySQL models), so there is no MySQL mapping to test. Run 42769037273 (ci-agent, headSha 4d04a3d, all jobs green).
- Criterion 1 (red-first) stays unticked: the mapping pre-existed the tests.
- Decision (user, .procoder/ask/decisions.md): SQLite rejects a bound float NaN with ErrValidation. Built in internal/sqlshared/args.go (checkArgs, called from every pool, transaction, pinned and read-only query and exec, so both the cgo and the pure driver); TestTypeMappingNaNSQLite asserts it and docs/TYPE_MAPPING.md states it.
