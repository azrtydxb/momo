# momo-02 Task 14: MomoMeta emission in the model package

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 14: MomoMeta emission in the model package (`.procoder/plans/momo-02-generator.md:1444`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-3. Files: `gen/emit_meta.go`, `gen/emit_meta_test.go`, `gen/testdata/golden/meta/models/momo_gen.go`. Contributes to S-3; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestEmitMetaGolden` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `go test ./gen -run TestEmitMeta` failed to build (`undefined: EmitMeta`, models lacked MomoMeta). Green: TestEmitMetaGolden, TestEmitMetaBuilds (go build of generated output), TestEmitMetaRuntime (momo.Select over generated meta with fakedriver) pass.
- Mutation: Version index computed from the Identity tag made TestEmitMetaGolden and TestEmitMetaBuilds fail.
- Gate: go build, go vet, go test -race ./... pass; make staticcheck clean; procoder check 0 blocking.
- Deviation: the golden has CreatedAt: -1 (Document has no created_at tag, only read_only), the plan sketch showed 4. Meta.Schema is emitted but momo.Select does not yet render the schema in FROM.
