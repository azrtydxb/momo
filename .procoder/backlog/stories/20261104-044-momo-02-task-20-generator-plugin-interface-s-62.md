# momo-02 Task 20: Generator plugin interface (S-62)

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 20: Generator plugin interface (S-62) (`.procoder/plans/momo-02-generator.md:2105`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-62. Files: `gen/plugin.go`, `gen/plugin_test.go`, `gen/testdata/plugins/fixture/fixture.go`, `gen/types.go`, `gen/emit.go`.

## Acceptance criteria

- [ ] `TestGeneratorPlugin` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestPluginsUseWriter` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-62] `TestGeneratorPlugin` runs a fixture plugin and asserts deterministic output and refusal to overwrite handwritten files; fails if plugin output bypasses the writer.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

