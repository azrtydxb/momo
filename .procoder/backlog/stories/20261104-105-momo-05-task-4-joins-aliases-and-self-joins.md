# momo-05 Task 4: Joins, aliases and self-joins

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 4: Joins, aliases and self-joins (`.procoder/plans/momo-05-writes-queries.md:331`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `integration_joins_test.go`, `select.go`.

## Acceptance criteria

- [x] `TestJoinsAliasesSelfJoin` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Failing first: `go test -tags integration -run Join .` failed to build with `suite.DocTitle.As("l").AsExpr undefined`; with only AsExpr added, TestJoinColumnFromMissingTableErrors and TestCorrelatedSubqueryReferenceAllowed failed (`err = <nil>, want ErrValidation`).
- Now: TestJoinsAliasesSelfJoin passes on postgres (SQLite is covered by SQL rendering through fakedriver, not execution); the full root package passes with -tags integration -race against PostgreSQL. `go build`, `go vet`, `go test -race ./...` and `make staticcheck` are clean.
- Mutation check: making the select scope expose the bare table name instead of the alias fails TestSelfJoinSQLQualifiesEachSide and TestCorrelatedSubqueryReferenceAllowed.
- Build-time source check: Builder.pushScope/popScope in builder.go and SelectQuery.render in select.go reject a column qualified by a table that is not a FROM or JOIN source (ErrValidation); a correlated reference to an enclosing SELECT is allowed.
- Review fixes: statement builders render through `Builder.withScope` (documented on the builder); TestMutationScopeAllowsCorrelatedSubquery simulates a mutation target scope.
