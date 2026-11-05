# momo-04 Task 12: PostgreSQL inspection: views, functions, triggers, policies, RLS, partitions

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 12: PostgreSQL inspection: views, functions, triggers, policies, RLS, partitions (`.procoder/plans/momo-04-migrations.md:1320`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-37, S-38, S-39. Files: `migrate/postgres/inspect_objects.go`. Contributes to S-37, S-38, S-39; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestPostgresObjectInspection` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `go test -tags integration ./migrate/postgres/` failed before implementation (TestPostgresObjectInspection: `trigger: []`; details test: `views: []`; UPDATE OF test: err nil). Green: `go test -tags integration -race -count=1 ./migrate/... ./schema/...` ok.
- Mutation: mapping `i` to STABLE in the volatility map failed TestPostgresObjectInspectionDetails; restored.
- View.Depends is populated from pg_depend/pg_rewrite; matview indexes via inspectIndexes; trigger WHEN is cut from pg_get_triggerdef (pg_get_expr cannot deparse OLD/NEW); inherited (tgparentid <> 0) triggers and extension-owned functions/views are hidden.
- Gate: go build, go vet, make staticcheck clean; `procoder check` 0 blocking findings.

