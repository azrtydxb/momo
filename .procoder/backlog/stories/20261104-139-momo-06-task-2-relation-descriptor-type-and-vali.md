# momo-06 Task 2: Relation descriptor type and validation

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 2: Relation descriptor type and validation (`.procoder/plans/momo-06-relations.md:288`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-6, S-82. Files: `relation.go`, `relation_internal_test.go`. Contributes to S-82; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestRelationValidate` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-6, part] this story's tests pass for the part of S-6 this task delivers; the full criterion is verified in story 146.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestRelationValidate` failed first (`undefined: Relation`), then passed after relation.go (`ok github.com/azrtydxb/momo`).
- `go build ./... && go vet ./... && go test -race -count=1 ./...` all ok; `procoder check`: 0 blocking findings. Commit 6b275cf.
- [S-6, part] descriptor-shape part asserted by TestRelationValidate (passes); full S-6 verified in story 146.
