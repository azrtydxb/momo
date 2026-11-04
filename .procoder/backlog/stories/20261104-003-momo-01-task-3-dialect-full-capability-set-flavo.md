# momo-01 Task 3: Dialect: full capability set, flavours and quoting

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 3: Dialect: full capability set, flavours and quoting (`.procoder/plans/momo-01-foundation.md:285`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4, S-31. Files: `dialect/dialect.go`, `dialect/dialect_test.go`. Contributes to S-4, S-31; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestDialectCapabilities` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestAppendIdentQuoting` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestPlaceholderStyle` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestDialectCapabilities: `go test ./dialect/ -count=1` before implementation failed to compile (`pg.Merge undefined`, `undefined: MySQLFlavor`); after: `ok github.com/azrtydxb/momo/dialect`
- TestAppendIdentQuoting: same red run (`undefined: MySQLFlavor`); `go test ./dialect/ -run TestAppendIdentQuoting -v -count=1` PASS
- TestPlaceholderStyle: same red run; `go test ./dialect/ -run TestPlaceholderStyle -v -count=1` PASS
- Gate and commit: `launcher.sh check dialect/dialect.go dialect/dialect_test.go` clean; commit d19c341 `feat(dialect): add MySQL flavours and the full capability set`; `go build ./... && go vet ./... && go test -race ./...` all ok
