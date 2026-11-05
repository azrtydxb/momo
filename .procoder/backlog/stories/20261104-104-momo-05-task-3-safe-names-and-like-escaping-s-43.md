# momo-05 Task 3: Safe names and LIKE escaping (S-43)

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 3: Safe names and LIKE escaping (S-43) (`.procoder/plans/momo-05-writes-queries.md:249`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-43. Files: `allowlist.go`, `allowlist_test.go`.

## Acceptance criteria

- [x] `TestColumnByName` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestEscapeLike` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-43] `TestColumnByName` rejects unknown names and returns the typed column for known ones; `TestEscapeLike` matches literal `%`, `_` and the escape character on both dialects; fails if a name is ever quoted into SQL without the allowlist.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- RED: `go test -run TestColumnByName|TestEscapeLike|TestContainsLiteral .` failed to build: undefined momo.NewAllowlist, momo.EscapeLike, momo.ContainsLiteral.
- GREEN: same run with `-tags integration -race` on PostgreSQL passes: TestColumnByName, TestEscapeLike/postgres (literal `%_` and a backslash each match exactly one row), TestContainsLiteralSQLPerDialect (ESCAPE with one backslash on PostgreSQL and SQLite, doubled on MySQL).
- Mutation: removing the backslash from the escaped set in EscapeLike fails TestEscapeLike; restored.
- FuzzEscapeLike (new Makefile FUZZ_TARGETS line) checks round-trip and no unescaped wildcard; 5s run passes.
- go build, go vet, go test -race ./... and make staticcheck are clean.
- Deviation: the plan test uses unqualified Document/DocTitle; the DB part uses suite.Document and suite.DocTitle because the root unit fixture has no Document model. Allowlist.Column does not echo the rejected name in its error.
- Revision after review: escape character changed from backslash to `!` (ESCAPE with `!` on every dialect, immune to MySQL NO_BACKSLASH_ESCAPES and PostgreSQL standard_conforming_strings). A value containing `!` matches literally (TestEscapeLike), NewAllowlist skips nil entries (TestNewAllowlistSkipsNil), dropping `!` from the escaped set fails TestEscapeLike, 10s fuzz run passes. This supersedes the backslash and per-dialect ESCAPE statements above.
