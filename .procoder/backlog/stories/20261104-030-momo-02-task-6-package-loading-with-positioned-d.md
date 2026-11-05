# momo-02 Task 6: Package loading with positioned diagnostics

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 6: Package loading with positioned diagnostics (`.procoder/plans/momo-02-generator.md:491`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-2, S-3. Files: `gen/types.go`, `gen/load.go`, `gen/load_test.go`, `gen/testdata/fixtures/brokenpkg/broken.go`. Contributes to S-2, S-3; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestLoadPackagesDiagnostics` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- RED: `go test ./gen -run TestLoadPackagesDiagnostics -count=1` failed with `undefined: Load` / `undefined: Config`.
- GREEN: `go test ./gen -count=1 -v` passes TestLoadPackagesDiagnostics (brokenpkg diagnostic at broken.go:8:col), TestLoadSyntaxErrorPositioned (real syntax error, temp module, line 5), TestLoadTypeErrorPositioned, TestLoadMissingPackage, TestLoadEmptyPatterns, TestDiagnosticString, TestLoadCopiesPlugins.
- `go build ./... && go vet ./... && go test -race -count=1 ./...`, `go test ./internal/repocheck -run TestImportGraph` and `make tidy-check` all pass (golang.org/x/tools v0.52.0, latest; plan names none).
- Commit cd8dc06 `feat(gen): package loading with positioned diagnostics`; commit gate ran clean on it.
- Deviation: the committed brokenpkg fixture line 8 is a type error (`Name Undefined`) because the format gate refuses unparsable Go; the real syntax-error case is covered by a test that writes a temp module.

