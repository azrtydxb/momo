# momo-02 Task 25: Plan gate for the generator

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 25: Plan gate for the generator (`.procoder/plans/momo-02-generator.md:2731`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-2, S-3, S-43, S-62, S-67, S-69, S-70, S-71. Contributes to S-2, S-3, S-43, S-62, S-67, S-69, S-70, S-71; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] Step 1: Run `go test -race -count=1 ./gen/... ./internal/golden ./internal/testmodels/... ./cmd/momo ./momo`; expect every package `ok`.
- [ ] Step 2: Run `go test -count=1 -v ./gen/... ./cmd/momo ./momo ./internal/testmodels/example -run 'TestEmitDescriptorsGolden|TestGenerateDiagnostics|TestGenerateDeterministic|TestGenerateCheck|TestGenerateCrossPackage|TestGenerateRefusesHandwritten|TestGenerateNameCollisions|TestEmbeddedAndMixins|Tes…
- [ ] Step 3: Run `go list -deps ./momo ./dialect ./schema ./vector ./types | grep -E 'golang.org/x/tools|yaml'`; expect no output (dependency rule 1).
- [ ] Step 4: Run `go vet ./...` and `gofmt -l .`; expect no output.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

