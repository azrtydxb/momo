# momo-10 Task 16: Gate for the ecosystem generators

Status: open
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-10-ecosystem-generators Task 16: Gate for the ecosystem generators (`.procoder/plans/momo-10-ecosystem-generators.md:1625`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-63, S-66. Contributes to S-63, S-66; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] Step 1: Run `go test -race -count=1 ./gen/apiquery ./gen/viz ./gen/graphql ./gen/grpc ./gen/openapi ./cmd/momo`; expect every package `ok`.
- [ ] Step 2: Run `cd internal/integration && MOMO_REQUIRE_INTEGRATION=1 PROTOC=$(pwd)/bin/protoc go test -race -count=1 ./...`; expect `ok`.
- [ ] Step 3: Run `go test -count=1 -v ./gen/viz ./gen/graphql ./gen/grpc ./gen/openapi -run 'TestViz|TestGraphQLSDL|TestGraphQLScaffold|TestProtoLockStableNumbers|TestOpenAPIDocumentGolden'` and confirm each named test shows `--- PASS`.
- [ ] Step 4: Run `go list -deps ./momo ./gen/apiquery | grep -E 'gqlgen|grpc|jsonschema|x/tools'`; expect no output, so the runtime and the shared helpers stay out of the framework graph.
- [ ] Step 5: Record the results in the pull request description for this plan. No commit is made by this task.
- [ ] `procoder check` gate is clean

## Evidence

