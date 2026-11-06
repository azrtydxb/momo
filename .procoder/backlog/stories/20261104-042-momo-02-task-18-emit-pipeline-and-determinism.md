# momo-02 Task 18: Emit pipeline and determinism

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 18: Emit pipeline and determinism (`.procoder/plans/momo-02-generator.md:1857`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-3. Files: `gen/emit.go`, `gen/emit_test.go`, `gen/writer.go`. Contributes to S-3; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestGenerateDeterministic` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- RED: `go test ./gen -run TestGenerateDeterministic` failed to compile: `undefined: Emit`, `undefined: Output`, `w.Outputs undefined`.
- GREEN: `TestGenerateDeterministic`, `TestGenerateDeterministicAcrossLoads` (fresh Load per run, GOMAXPROCS 1/4/1/8), `TestEmitOutputsSortedAndLayout`, `TestEmitRunsPluginsInOrder`, `TestEmitDiagnosticsWriteNothing` pass.
- Mutation: removing the sort in `Writer.Outputs` makes TestGenerateDeterministic and TestEmitOutputsSortedAndLayout fail; restored.
- Note: EmitDescriptors already embeds the EmitLookup output (ColumnByName, ColumnNames) in the subpackage momo_gen.go, so Emit does not call EmitLookup separately (a second write would be an ErrCollision).
