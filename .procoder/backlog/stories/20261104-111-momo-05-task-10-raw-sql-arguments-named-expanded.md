# momo-05 Task 10: Raw SQL arguments: named, expanded IN and identifiers (S-77)

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 10: Raw SQL arguments: named, expanded IN and identifiers (S-77) (`.procoder/plans/momo-05-writes-queries.md:759`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-77. Files: `raw_args.go`, `raw_args_test.go`.

## Acceptance criteria

- [x] `TestRawArgs` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestRawEmptyInIsBuildError` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestRawNonEmptyInBinds` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-77, part] `TestRawArgs` passes for the part of S-77 this task delivers; the full criterion is verified in story 112.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: with the tests written and no implementation, `go vet .` failed to compile the package (`undefined: NamedArg`, `undefined: In`, `undefined: Ident`), so `TestRawArgs` (integration, `raw_args_integration_test.go` via `suite.RawArgs`), `TestRawEmptyInIsBuildError` and `TestRawNonEmptyInBinds` (`raw_empty_in_test.go`) all failed. Green after `raw_args.go`: `go test -tags integration -race -count=1 . ./internal/...` all ok against the running Postgres, `TestRawArgs/postgres` PASS.
- Beyond the plan: a dialect-aware scanner (`scanRaw`) leaves `?` and `@name` untouched inside string literals, quoted identifiers, comments, dollar quotes and the PostgreSQL operators; table-driven in `raw_args_internal_test.go`. Expanded IN lists are checked against `dialect.MaxParams`. `RawArgs` is in `internal/suite/queries.go` and runs as the `raw` subtest of `Queries`.
- The S-77 criterion stays partial: the fuzz target (Task 11) belongs to story 112.

- Review hardening (raw_args_hardening_internal_test.go, red first, then green): a Named argument no @name references is a build error on every dialect; on PostgreSQL a bare native placeholder mixed with a ? or @name bind is a build error (the plan does not specify native placeholders; a lone native placeholder is still left alone); an unterminated string literal, quoted identifier, block comment or dollar quote is a build error; In elements that are Ident, In, Named or Expr arguments are build errors and In of a byte slice expands the bytes as elements (documented); the PostgreSQL containment operator is never an @name. Mutation checks cover MySQL backslash escapes, the PostgreSQL escape-string prefix rule, the MySQL double-dash space rule, identifier-prefixed at-signs and dollar signs, MySQL system variables and the MaxParams check on plain binds; each mutant was reintroduced and killed.
- Scanner assumptions are recorded in the doc comment of Named: MySQL NO_BACKSLASH_ESCAPES and PostgreSQL standard_conforming_strings=off are unsupported, MySQL executable comments are treated as comments, and the PostgreSQL jsonb operators with a question mark are operators (write a space to bind then OR).
