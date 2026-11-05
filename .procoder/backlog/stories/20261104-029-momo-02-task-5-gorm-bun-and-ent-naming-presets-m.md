# momo-02 Task 5: GORM, Bun and Ent naming presets matching the fixtures

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 5: GORM, Bun and Ent naming presets matching the fixtures (`.procoder/plans/momo-02-generator.md:412`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-69. Files: `gen/naming/gorm.go`, `gen/naming/bun.go`, `gen/naming/ent.go`, `gen/naming/presets_test.go`, `gen/naming/naming.go`.

## Acceptance criteria

- [ ] `TestNamingPresets` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-69] `TestNamingPresets` asserts each preset's names for a fixture set match the names those ORMs generate (recorded fixtures); fails if a preset diverges.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

