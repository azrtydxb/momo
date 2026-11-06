# momo-02 Task 5: GORM, Bun and Ent naming presets matching the fixtures

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 5: GORM, Bun and Ent naming presets matching the fixtures (`.procoder/plans/momo-02-generator.md:412`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-69. Files: `gen/naming/gorm.go`, `gen/naming/bun.go`, `gen/naming/ent.go`, `gen/naming/presets_test.go`, `gen/naming/naming.go`.

## Acceptance criteria

- [x] `TestNamingPresets` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-69] `TestNamingPresets` asserts each preset's names for a fixture set match the names those ORMs generate (recorded fixtures); fails if a preset diverges.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

Before implementation: `go test ./gen/naming -run TestNamingPresets -count=1`
--- FAIL: TestNamingPresets (0.00s)
presets_test.go:26: unknown naming preset "gorm"

After: `go test ./gen/naming -count=1` -> ok github.com/azrtydxb/momo/gen/naming
Mutation: bun underscore also inserted between uppercase runes -> FAIL `bun ColumnName(UserProfile.UserID)="user_i_d" want "user_id"`; reverted.
Gate: `go build ./... && go vet ./... && go test -race -count=1 ./...` all ok; `procoder check`: 0 unformatted, 0 blocking.
Note: gorm/bun join_tables fixtures echo an explicit tag; doc comments on GORM and Bun say the join rule is a convention, not an ORM default.
