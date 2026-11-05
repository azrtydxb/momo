# momo-02 Task 20: Generator plugin interface (S-62)

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 20: Generator plugin interface (S-62) (`.procoder/plans/momo-02-generator.md:2105`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-62. Files: `gen/plugin.go`, `gen/plugin_test.go`, `gen/testdata/plugins/fixture/fixture.go`, `gen/types.go`, `gen/emit.go`.

## Acceptance criteria

- [x] `TestGeneratorPlugin` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestPluginsUseWriter` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-62] `TestGeneratorPlugin` runs a fixture plugin and asserts deterministic output and refusal to overwrite handwritten files; fails if plugin output bypasses the writer.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `TestGeneratorPlugin` failed first on the handwritten step (the plan fixture lacked the generated header, which the Writer rejects before the handwritten check), then on `plugin_test.go:72: bad name: got <nil>`.
- `TestPluginsUseWriter` is a source scan that already passed before the implementation (Plugin was stubbed in Task 18 and no generator file calls os write APIs), so it was never red. It is a guard, not a driver. Criterion left unchecked for that reason.
- Green: `gen/plugin.go` adds PluginAPIVersion, VersionedPlugin and name/version validation; `Emit` validates every plugin before buffering anything. Both tests pass.
- Mutation: changing the version check to `v < 0` makes `TestGeneratorPlugin` fail (`api version: got <nil>`); restored.
- Deviation: the fixture's handwritten target carries the generated header so the Writer reaches its ErrHandwritten check.
- Gate: go build, go vet, staticcheck, `go test -race ./gen/...`, `procoder check`.

