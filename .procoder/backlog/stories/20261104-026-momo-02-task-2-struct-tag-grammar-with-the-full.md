# momo-02 Task 2: Struct tag grammar with the full S-70 vocabulary

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 2: Struct tag grammar with the full S-70 vocabulary (`.procoder/plans/momo-02-generator.md:154`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-70. Files: `gen/tags/tags.go`, `gen/tags/tags_test.go`. Contributes to S-70; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestParseTags` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestTagGrammar` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `go test ./gen/tags -count=1` failed with `undefined: Parse` before tags.go existed.
- Green: `go test -race -count=1 ./gen/tags` ok; `FuzzParse` ran 25s (about 10M execs) with no failure.
- Hostile input (`gen/tags/hostile_test.go`): empty and whitespace tags, stray and double commas, upper-case keys, duplicate keys, empty values, bare key with value, negative/overflow/float integers, bad enum values, backslash is not an escape.
- Full suite: `go build ./... && go vet ./... && go test -race -count=1 ./...` ok; `procoder check` clean (0 blocking).
- Commit ae9990b.
- The TestTagVocabulary criterion stays open: its schema half is plan Task 10.

