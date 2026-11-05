# momo-04 Task 4: Migration file discovery, directives, statement splitter

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 4: Migration file discovery, directives, statement splitter (`.procoder/plans/momo-04-migrations.md:578`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-94. Files: `migrate/files.go`. Contributes to S-11, S-94; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestSplitStatements` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSplitDollarQuotes` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSplitSQLiteTrigger` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestDirectives` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestDiscoverRejectsMalformedNames` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- RED: `go test ./migrate/` failed to build with `undefined: Discover`, `undefined: SplitStatements`, `undefined: ParseDirectives`.
- GREEN: `go test -race -count=1 ./migrate/` ok. The five named tests pass, plus hostile-input tables (BOM, CRLF, nested comments, E-strings, tagged dollar quotes, trigger CASE/END, unterminated input, bad names, duplicate versions, unknown directives).
- Mutation check: disabled the dollar-quote branch in the splitter; TestSplitDollarQuotes failed (4 statements instead of 2). Restored.
- Fuzz: FuzzSplitStatements (no panic, trimmed non-empty statements, re-split idempotence) ran 20s clean; added to Makefile FUZZ_TARGETS.
- Gate: `procoder check` 0 blocking findings; go build, go vet and go test -race pass.

