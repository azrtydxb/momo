# momo-01 Task 10: Spike repairs: flags, qualified table references and soft-delete filter

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 10: Spike repairs: flags, qualified table references and soft-delete filter (`.procoder/plans/momo-01-foundation.md:948`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4, S-75. Files: `meta.go`, `select_repair_test.go`. Contributes to S-4, S-75; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestSoftDeleteFilterWithColumns` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestTableRefSchemaQualified` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestSoftDeleteFilterWithColumns`: before the implementation the package failed to compile (`unknown field Schema in struct literal of type momo.TableRef`); with only the soft-delete condition reverted it fails with `soft-delete filter lost when columns are restricted: got SELECT ... FROM "documents"` (no WHERE); now `--- PASS: TestSoftDeleteFilterWithColumns`.
- `TestTableRefSchemaQualified`: failed to compile before the implementation, then `--- PASS: TestTableRefSchemaQualified` after it.
- `go build ./... && go vet ./... && go test -race ./...` all `ok`.
- `launcher.sh check`: `5 clean, 0 unformatted, 0 unchecked, 0 out of scope, 48 hygiene finding(s) (0 blocking)`.
- Commit 634869f `fix(momo): keep the soft-delete filter with restricted columns and align Meta with the contract`.
