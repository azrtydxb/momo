# momo-02 Task 22: Example models with go:generate, and generated output that builds

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 22: Example models with go:generate, and generated output that builds (`.procoder/plans/momo-02-generator.md:2464`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-2, S-3. Files: `internal/testmodels/example/models.go`, `internal/testmodels/example/generate.go`, `internal/testmodels/example/example_test.go`, `internal/testmodels/example/momo_gen.go`. Contributes to S-3; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestGenerateCompanion` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-2] [S-3] `TestGenerateCompanion` (generator golden test): a model package with tags and a MomoConfig companion generates code that `go build` accepts; `TestGenerateDiagnostics` asserts an unsupported companion expression yields file:line:col and writes no files; fails if the generator guesses or emits partial output.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- FAIL before goldens: `example_test.go:44: .../example/comment/momo_gen.go is stale`; after generating: `ok github.com/azrtydxb/momo/internal/testmodels/example`.
- Mutation: changing the Opclass `vector_cosine_ops` to `vector_l2_ops` fails with `.../example/momo_gen.go is stale`; reverted, passes.
- Goldens generated with a throwaway driver over gen.Load/Emit/Writer.Commit (cmd/momo is not on this branch yet). The CLI `go:generate` line is in generate.go; `momo generate --check` verification follows once cmd/momo lands.
- `go build ./...`, `go vet ./...`, `make staticcheck` clean; `go test -race -count=1 ./gen/... ./internal/...` ok; procoder check: 0 blocking.

