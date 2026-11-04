# momo-10 Task 9: GraphQL server end to end with batched relations

Status: open
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 9: GraphQL server end to end with batched relations (`.procoder/plans/momo-10-ecosystem-generators.md:609`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-63. Files: `internal/integration/graphql/graphql_test.go`, `internal/integration/fixture/fixture.go`, `internal/integration/graphql/example/graph/`, `internal/integration/graphql/example/gqlgen.yml`, `gen/graphql/goemit.go`.

## Acceptance criteria

- [ ] `TestGraphQLGenerator` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-63] `TestGraphQLGenerator` generates for the example models, builds a gqlgen server and asserts a query with filter, pagination and nested relation returns expected data with the batched query count; fails if nested fields cause per-row queries.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

