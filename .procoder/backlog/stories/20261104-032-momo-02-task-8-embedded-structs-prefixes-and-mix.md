# momo-02 Task 8: Embedded structs, prefixes and mixins

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 8: Embedded structs, prefixes and mixins (`.procoder/plans/momo-02-generator.md:687`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-67. Files: `gen/embed.go`, `gen/embed_test.go`, `gen/testdata/fixtures/mixins/mixins.go`, `gen/model.go`.

## Acceptance criteria

- [x] `TestEmbeddedAndMixins` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-67] `TestEmbeddedAndMixins` (generator golden + integration) asserts prefixed columns and mixin indexes exist and round-trip; fails if an embedded field is skipped.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Failing run before implementation (`go test ./gen -run TestEmbeddedAndMixins -count=1`):
  `--- FAIL: TestEmbeddedAndMixins` ... `load: err=<nil> diags=[.../mixins.go:12:22: momo tags on non-model type Timestamps ... :17:16: momo tags on non-model type Address]`.
  The mixin structs were rejected as stray tagged non-models, and embedded fields were skipped. The plan expected `columns=[id updated_at ...]`; the observed failure differs but is the same cause.
- After implementation: `go test ./gen -count=1` -> `ok github.com/azrtydxb/momo/gen`.
- Extra tests: `TestEmbeddedNestedAliasAndSkip` (nested prefixes, alias embed, `momo:"-"`, Path, Base, index names) and `TestEmbeddedDiagnostics` (prefix vs direct field collision via Load, pointer embed, non-struct `embedded`, prefix without `embedded`).
- Mutation check: removing `prefix +` from the column made TestEmbeddedAndMixins, TestEmbeddedNestedAliasAndSkip and TestEmbeddedDiagnostics fail; making appendPath drop the index made TestEmbeddedAndMixins and TestEmbeddedNestedAliasAndSkip fail. Both reverted.
- `go build ./... && go vet ./... && go test -race -count=1 ./...` -> all `ok`.
- `procoder check` -> `7 clean, 0 unformatted, 0 unchecked, 0 out of scope, 35 hygiene finding(s) (0 blocking)`.
- Commit: `feat(gen): embedded structs, prefixes and mixins`.
