# momo-02 Task 9: Statically read MomoConfig companion with diagnostics

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 9: Statically read MomoConfig companion with diagnostics (`.procoder/plans/momo-02-generator.md:783`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-2. Files: `gen/companion.go`, `gen/companion_test.go`, `gen/testdata/fixtures/companion/ok.go`, `gen/testdata/fixtures/companionbad/bad.go`, `gen/model.go`.

## Acceptance criteria

- [x] `TestCompanionReaderAccepts` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestGenerateDiagnostics` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-2, part] `TestGenerateDiagnostics` passes for the part of S-2 this task delivers; the full criterion is verified in story 046.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Base: `git reset --hard sprint-2`, then `git merge-base --is-ancestor sprint-2 HEAD && echo ok` printed `ok`.
- RED (tests and fixtures written first): `go test ./gen -run 'TestCompanion|TestGenerateDiagnostics' -count=1` printed `gen/companion_test.go:32:20: doc.Config.Indexes undefined (type *CompanionResult has no field or method Indexes)` ... `FAIL github.com/azrtydxb/momo/gen [build failed]`.
- GREEN: `go test ./gen -count=1` printed `ok  github.com/azrtydxb/momo/gen`.
- Mutation check: disabling the duplicate-name branch in `declare` (`dup && false`) made `go test ./gen -run TestCompanionHostileShapes` print `FAIL` with `missing diagnostic hostile.go:64: "index name \"dups_name_idx\" is already declared at"` and `hostile.go:65: ... already produced by the index tag on field Slug`. Reverted; green again.
- Hostile shapes covered by `TestCompanionHostileShapes` and `TestCompanionUnknownMethods`: method in a for loop and an if, variable and concatenated arguments, `&r.Hidden`, `&r.Model`, `&r.Owner.Name`, unnamed receiver, wrong MomoConfig signature, duplicate and tag-clashing index names, duplicate Default, unknown methods, structured literals with a variable, unkeyed fields and nested composites.
- Gate: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...` all `ok`; `procoder check` printed `procoder gate: 9 clean, 0 unformatted, 0 unchecked, 0 out of scope, 46 hygiene finding(s) (0 blocking)`.
- Commit: `1256d03 feat(gen): statically read MomoConfig companions with positioned diagnostics`.
- Deviations: (1) fixture `ok.go` declares a local `Vector` type because `vector.Vector` does not exist yet (momo-07); (2) fixtures carry the licence header, so the plan's bad.go lines 14 and 16 are 16 and 18; (3) extra fixtures `companionbad/hostile.go`, `companionstruct`, `companionunknown` and extra tests for hostile shapes; (4) dropping a package's models applies to companion diagnostics only, since the broader rule broke existing Task 6-8 tests that expect models alongside tag diagnostics (Task 17 owns the global rule).
