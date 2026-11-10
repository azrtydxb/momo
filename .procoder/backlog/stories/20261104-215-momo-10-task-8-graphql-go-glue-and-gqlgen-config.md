# momo-10 Task 8: GraphQL Go glue and gqlgen configuration (scaffold)

Status: open
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-10-ecosystem-generators Task 8: GraphQL Go glue and gqlgen configuration (scaffold) (`.procoder/plans/momo-10-ecosystem-generators.md:538`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `gen/graphql/goemit.go`, `gen/graphql/goemit_test.go`, `gen/graphql/plugin.go`.

## Acceptance criteria

- [ ] `TestGraphQLScaffold` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence
- Built: gen/graphql/goemit.go (Go glue: filter, order, connection types, ListXs and GetX with Preload of only the requested relations, scalars; gqlgen.yml; resolver scaffolds), plugin.go (Plugin writes schema.graphqls and gqlgen.yml via w.Text, model/momo_gen.go via w.File, resolver.go and schema.resolvers.go via w.Scaffold), sdl.go (analysis shared with SDL).
- Design deviation from the plan: gqlgen's generated executor cannot import the package that holds the resolvers, so the glue lives in `<Dir>/model` and the resolvers use gqlgen's follow-schema layout; a single-file resolver was rewritten by gqlgen and lost the root resolver (CI run 42771551254).
- TestGraphQLScaffold (gen/graphql/goemit_test.go) and TestGoSourceExample, TestGoSourceRules pass in CI run 42779257792 (headSha b6b30a8): unit, integration (postgres), integration-mysql (mysql84, mysql97, mariadb), fuzz-smoke, bench-module and apidiff-docs green; lint red only on gosec of the protoc stubs (fixed in the next commit). TestGraphQLGenerator, TestProtoGenerator and TestOpenAPIGenerator PASS in run 42774579231 (regen-tmp, commit 41a3405, PostgreSQL with MOMO_REQUIRE_INTEGRATION=1 and the pinned protoc).
- Open: criterion 1 not ticked. Red proof: the tests were written with the code, so no run failed on `undefined` alone; per the 2026-11-08 decision a throwaway red branch (implementation reverted) is still needed to tick this criterion.
