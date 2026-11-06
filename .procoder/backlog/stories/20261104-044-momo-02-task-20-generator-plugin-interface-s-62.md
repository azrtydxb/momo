# momo-02 Task 20: Generator plugin interface (S-62)

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 20: Generator plugin interface (S-62) (`.procoder/plans/momo-02-generator.md:2105`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-62. Files: `gen/plugin.go`, `gen/plugin_test.go`, `gen/testdata/plugins/fixture/fixture.go`, `gen/types.go`, `gen/emit.go`.

## Acceptance criteria

- [x] `TestGeneratorPlugin` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestPluginsUseWriter` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-62] `TestGeneratorPlugin` runs a fixture plugin and asserts deterministic output and refusal to overwrite handwritten files; fails if plugin output bypasses the writer.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `TestGeneratorPlugin` failed first on the handwritten step (the plan fixture lacked the generated header, which the Writer rejects before the handwritten check), then on `plugin_test.go:72: bad name: got <nil>`.
- `TestPluginsUseWriter` is a go/ast guard (resolves aliases of os, io/ioutil, syscall; ignores comments and strings; scans gen/... including testdata/plugins; only gen/writer.go exempt). red demonstrated by injected violation: `os.WriteFile(` in gen/emit.go, an aliased `o "os"` `o.MkdirAll`, and `ioutil.WriteFile` each made it FAIL; reverted. `TestFsViolationsDetector` unit-tests the detector. No CLI package exists to scan.
- Review fixes: duplicate plugin names rejected; tests for slice order, rejected plugin sets leaving Outputs() empty, and %w wrapping with errors.Is; Plugin and Emit document that callers must not Commit after an error.
- Plan gap: the plan's fixture wrote a headerless handwritten target, but the Writer rejected headerless output before checking handwritten files, so ErrHandwritten was unreachable. Writer.File now checks isHandwritten first (TestWriterHandwrittenBeforeHeaderless); the fixture is as in the plan.
- Green: `gen/plugin.go` adds PluginAPIVersion, VersionedPlugin and name/version validation; `Emit` validates every plugin before buffering anything. Both tests pass.
- Mutation: changing the version check to `v < 0` makes `TestGeneratorPlugin` fail (`api version: got <nil>`); restored.
- Gate: go build, go vet, staticcheck, `go test -race ./gen/...`, `procoder check`.

