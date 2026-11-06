# momo-02 Task 19: Cross-package generation without import cycles

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 19: Cross-package generation without import cycles (`.procoder/plans/momo-02-generator.md:1928`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-3. Files: `gen/importpath.go`, `gen/importpath_test.go`, `gen/crosspkg_test.go`, `gen/testdata/fixtures/crossa/a.go`, `gen/testdata/fixtures/crossb/b.go`.

## Acceptance criteria

- [x] `TestImportPath` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestGenerateCrossPackage` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-3] `TestGenerateCrossPackage`, `TestGenerateRefusesHandwritten` and `TestGenerateNameCollisions` (golden tests): models outside package main build without cycles, a handwritten file at a target path is never overwritten, reserved words and colliding names get distinct identifiers; fails if any golden fixture stops compiling.
- [x] `TestGenerateNameCollisions` asserts `len(g.Models) > 0`, its skip is removed, and the collision checks are wired from `model.go` and `load.go`
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Commit 515a3ad test(gen): cross-package generation builds without cycles. `go build ./...`, `go vet ./...`, `make staticcheck` clean; `go test -race -count=1 ./...` all ok (gen 201s).
- TestImportPath failed first with `undefined: ImportPath`, then passed after gen/importpath.go.
- TestGenerateCrossPackage failed first (plan-as-written: ambiguous import, because the temp module path is a prefix of the replaced momo module; then `go mod tidy` needed before Load; then `go build ./...` hit the deliberately broken fixtures). Fixed in the test only: temp module `example.com/cross` with the fixture import path rewritten on copy, tidy before Load, and only crossa/crossb copied. It then passes: Emit output for crossa and crossb builds with no import cycle. Descriptor and meta emitters needed no change.
- TestGenerateNameCollisions has no skip, asserts len(g.Models) > 0 and the column-collision diagnostic; the checks are wired in gen/load.go (collisionDiagnostics over the models built by gen/model.go). TestGenerateRefusesHandwritten passes.
- Review notes: Emit now reports a positioned diagnostic when two models of a package share a SubPkg (TestEmitSubPkgCollision; mutation of the check makes it fail with ErrCollision from the Writer instead); Emit doc says the Writer may hold partial output after an error and must not be committed.
- Deviation: ImportPath is implemented and tested but generated imports still come from types.Package.Path() (already slash-separated); no emitter calls ImportPath yet, since Graph carries no module path/dir.
