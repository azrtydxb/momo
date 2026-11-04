# momo-02 Task 25: Plan gate for the generator

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 25: Plan gate for the generator (`.procoder/plans/momo-02-generator.md:2731`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-2, S-3, S-43, S-62, S-67, S-69, S-70, S-71.

## Acceptance criteria

- [ ] [S-2] [S-3] `TestGenerateCompanion` (generator golden test): a model package with tags and a MomoConfig companion generates code that `go build` accepts; `TestGenerateDiagnostics` asserts an unsupported companion expression yields file:line:col and writes no files; fails if the generator guesses or emits partial output.
- [ ] [S-3] `TestGenerateCrossPackage`, `TestGenerateRefusesHandwritten` and `TestGenerateNameCollisions` (golden tests): models outside package main build without cycles, a handwritten file at a target path is never overwritten, reserved words and colliding names get distinct identifiers; fails if any golden fixture stops compiling.
- [ ] [S-43] `TestColumnByName` rejects unknown names and returns the typed column for known ones; `TestEscapeLike` matches literal `%`, `_` and the escape character on both dialects; fails if a name is ever quoted into SQL without the allowlist.
- [ ] [S-62] `TestGeneratorPlugin` runs a fixture plugin and asserts deterministic output and refusal to overwrite handwritten files; fails if plugin output bypasses the writer.
- [ ] [S-67] `TestEmbeddedAndMixins` (generator golden + integration) asserts prefixed columns and mixin indexes exist and round-trip; fails if an embedded field is skipped.
- [ ] [S-69] `TestNamingPresets` asserts each preset's names for a fixture set match the names those ORMs generate (recorded fixtures); fails if a preset diverges.
- [ ] [S-70] `TestTagVocabulary` asserts each tag reaches the schema, migration SQL and inspection round trip on every dialect or yields a dialect diagnostic; fails if a tag is ignored silently.
- [ ] [S-71] `TestFieldPermissions` plus compile-fail fixtures assert Set on a read-only/immutable column does not compile and create-only columns are rejected in updates; fails if any path writes them.
- [ ] Step 1: Run `go test -race -count=1 ./gen/... ./internal/golden ./internal/testmodels/... ./cmd/momo ./momo`; expect every package `ok`.
- [ ] Step 2: Run `go test -count=1 -v ./gen/... ./cmd/momo ./momo ./internal/testmodels/example -run 'TestEmitDescriptorsGolden|TestGenerateDiagnostics|TestGenerateDeterministic|TestGenerateCheck|TestGenerateCrossPackage|TestGenerateRefusesHandwritten|TestGenerateNameCollisions|TestEmbeddedAndMixins|Tes…
- [ ] Step 3: Run `go list -deps ./momo ./dialect ./schema ./vector ./types | grep -E 'golang.org/x/tools|yaml'`; expect no output (dependency rule 1).
- [ ] Step 4: Run `go vet ./...` and `gofmt -l .`; expect no output.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

