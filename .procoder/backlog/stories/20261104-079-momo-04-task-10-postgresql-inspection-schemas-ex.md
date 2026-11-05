# momo-04 Task 10: PostgreSQL inspection: schemas, extensions, enums, tables, columns, constraints

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 10: PostgreSQL inspection: schemas, extensions, enums, tables, columns, constraints (`.procoder/plans/momo-04-migrations.md:1128`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-12, S-36, S-74. Files: `migrate/postgres/dialect.go`. Contributes to S-12, S-36, S-74; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestPostgresInspectColumnsAndConstraints` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made
- [x] PostgreSQL inspection reads `Column.Collate` (non-default collation) and `Column.Comment` (`COMMENT ON COLUMN`), so a schema built by momo-02 Task 10 with them is `schema.Equal` to its inspected form (round-trip test; otherwise permanent drift)

## Evidence

- Red: `go test -tags integration ./migrate/postgres/` failed to build before implementation (`no non-test Go files` / undefined mpg.New); green after: `ok github.com/azrtydxb/momo/migrate/postgres`, with `./migrate/... ./schema/...` also ok under MOMO_REQUIRE_INTEGRATION=1 and -race.
- Tests: TestPostgresInspectColumnsAndConstraints (plan test plus check/pk/fk-ref asserts), TestPostgresInspectCollateCommentSerialUnique, TestPostgresInspectRoundTripCollateComment (schema.Equal with Collate "C" and a column comment), TestPostgresInspectOtherSchemaAndPartitions.
- Mutation: dropping the `col.Collate` assignment in migrate/postgres/inspect.go made the Collate and round-trip tests fail; restored.
- Gate: gofmt, go build, go vet, make staticcheck clean.
