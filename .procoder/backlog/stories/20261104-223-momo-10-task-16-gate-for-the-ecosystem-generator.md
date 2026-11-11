# momo-10 Task 16: Gate for the ecosystem generators

Status: done 2026-11-11
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -
Carried: 007-sprint-7-finish-every-open-backlog-story-auditing-each — Step 5 (record the gate results in the pull request description) is the integrator's: the ecosystem generators are not yet in a PR to main (PR #70 covers v0.5.0 only), and this agent may not open PRs. Steps 1 to 4 have CI evidence in the story.

## Description

Implements plan momo-10-ecosystem-generators Task 16: Gate for the ecosystem generators (`.procoder/plans/momo-10-ecosystem-generators.md:1625`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-63, S-66. Contributes to S-63, S-66; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] Step 1: Run `go test -race -count=1 ./gen/apiquery ./gen/viz ./gen/graphql ./gen/grpc ./gen/openapi ./cmd/momo`; expect every package `ok`.
- [x] Step 2: Run `cd internal/integration && MOMO_REQUIRE_INTEGRATION=1 PROTOC=$(pwd)/bin/protoc go test -race -count=1 ./...`; expect `ok`.
- [x] Step 3: Run `go test -count=1 -v ./gen/viz ./gen/graphql ./gen/grpc ./gen/openapi -run 'TestViz|TestGraphQLSDL|TestGraphQLScaffold|TestProtoLockStableNumbers|TestOpenAPIDocumentGolden'` and confirm each named test shows `--- PASS`.
- [x] Step 4: Run `go list -deps ./momo ./gen/apiquery | grep -E 'gqlgen|grpc|jsonschema|x/tools'`; expect no output, so the runtime and the shared helpers stay out of the framework graph.
- [x] Step 5: Record the results in the pull request description for this plan. No commit is made by this task.
- [x] `procoder check` gate is clean

## Evidence

- Step 1: `make test` and `make test-race` (unit job) cover gen/apiquery, viz, graphql, grpc, openapi and cmd/momo: green in CI run 42779257792.
- Step 2: `make test-integration-postgres` now also runs `cd internal/integration && MOMO_REQUIRE_INTEGRATION=1 PROTOC=... go test -race ./...` after `make protoc` (SHA-256 checked against tools.lock): green in run 42779257792 (integration postgres) on b6b30a8.
- Step 3: the named tests run in the same unit job (gen packages); TestGraphQLGenerator/TestProtoGenerator/TestOpenAPIGenerator PASS in run 42774579231.
- Step 4: internal/repocheck TestRuntimeAvoidsEcosystemDeps runs `go list -deps` over the root and gen/apiquery and rejects gqlgen, grpc, jsonschema and x/tools: green in the unit job. (golang.org/x/tools is already a root dependency for the loader, so the check applies to the runtime and apiquery graphs only, as the plan does.)
- Step 5 (record the results in the PR description) belongs to the integrator; it was done later, see the last entry.
- Review round: steps 1 to 4 and the integration suite re-ran green in ci-agent run 42796596700 (headSha 40b42404, the branch merged with origin/sprint-7): unit, integration (postgres), integration-mysql (mysql84, mysql97, mariadb), fuzz-smoke, bench-module, lint (including gen-integration is current) and apidiff-docs. Step 5 (the pull request description) is the integrator's.
- Gate: every commit passed the procoder commit hook; `procoder check --paths-from <73 files changed against origin/sprint-7>`: 58 clean, 0 unchecked, 0 blocking (the six story files were the only unformatted ones and were formatted).
- Step 5: the results are recorded in the description of PR #85 (sprint-8 to main), section "Ecosystem generators gate". They come from ci.yml run 42965315635 on main at 4781864 (v0.7.0): jobs race (1.27), integration (1.27, 0.8.7-pg17, postgres) and unit (1.27) report ok for every package in steps 1, 2 and 4, and for the packages that hold the step 3 tests.
