# momo-02 Task 3: Momo naming preset

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 3: Momo naming preset (`.procoder/plans/momo-02-generator.md:270`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-69. Files: `gen/naming/naming.go`, `gen/naming/momo.go`, `gen/naming/naming_test.go`.

## Acceptance criteria

- [ ] `TestNamingMomoPreset` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-69] `TestNamingPresets` asserts each preset's names for a fixture set match the names those ORMs generate (recorded fixtures); fails if a preset diverges.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

