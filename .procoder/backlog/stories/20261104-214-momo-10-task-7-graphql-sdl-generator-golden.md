# momo-10 Task 7: GraphQL SDL generator (golden)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 7: GraphQL SDL generator (golden) (`.procoder/plans/momo-10-ecosystem-generators.md:477`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `gen/graphql/sdl.go`, `gen/graphql/sdl_test.go`, `gen/graphql/testdata/example.graphqls`.

## Acceptance criteria

- [x] `TestGraphQLSDL` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Files: gen/graphql/sdl.go, sdl_test.go, testdata/example.graphqls.
- TestGraphQLSDL (and TestSDLRules) red: CI run 42455374066 (branch worktree-agent-a70d1eb94160543bf-red2, tests alone on sprint-5) failed with "undefined: SDL". Golden mismatch also shown in run 42455728306. Green: CI run 42456595252 (commit 5f4ffe6).
- The golden was written from the first run's output after review: Relay connection and edge types, typed filter and order inputs, shared PageInfo, OrderDirection and String, Int and Time filters, and a Query root with a list and a single-object query per model. Vector, sensitive and unmapped columns are not exposed; an unmapped type is the diagnostic "no GraphQL filter for <type>".
- Header uses the current Header() text as a # comment, not the old version-carrying form. Not done here: the plugin and Writer.Text wiring (Task 8, story 215) and the gqlparser validation (Task 9, story 216).

