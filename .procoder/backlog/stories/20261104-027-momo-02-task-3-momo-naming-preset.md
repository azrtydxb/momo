# momo-02 Task 3: Momo naming preset

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 3: Momo naming preset (`.procoder/plans/momo-02-generator.md:270`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-69. Files: `gen/naming/naming.go`, `gen/naming/momo.go`, `gen/naming/naming_test.go`. Contributes to S-69; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestNamingMomoPreset` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestNamingMomoPreset: first run failed to compile (undefined: Momo, ByName, ToSnake, Plural); after implementation `go test ./gen/naming -count=1` printed ok.
- Added tests for acronyms, digits, single letters, snake input, unicode, empty string and extra plurals; full build, vet and `go test -race -count=1 ./...` passed.
- procoder check gate: 3 clean, 0 blocking. Commit e98bc57.
- [S-69] TestNamingPresets criterion left open: it needs the GORM/Bun/Ent presets and fixtures from Tasks 4-5.
