# momo-02 Task 4: Record GORM, Bun and Ent naming fixtures

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 4: Record GORM, Bun and Ent naming fixtures (`.procoder/plans/momo-02-generator.md:334`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-69. Files: `internal/namingrec/go.mod`, `internal/namingrec/models/models.go`, `internal/namingrec/record_gorm/main.go`, `internal/namingrec/record_bun/main.go`, `internal/namingrec/ent/schema/user_profile.go`, `internal/namingrec/record_ent/main.go` …. Contributes to S-69; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestNamingFixturesRecorded` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

