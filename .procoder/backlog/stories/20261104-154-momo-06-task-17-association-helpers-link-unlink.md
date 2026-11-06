# momo-06 Task 17: Association helpers Link, Unlink, Replace and Count

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 17: Association helpers Link, Unlink, Replace and Count (`.procoder/plans/momo-06-relations.md:3913`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-83. Files: `assoc.go`, `assoc_test.go`.

## Acceptance criteria

- [x] `TestAssociationHelpers` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-83] `TestAssociationHelpers` asserts results and statement count per helper; fails if a helper issues an undeclared statement.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestAssociationHelpers` (assoc_test.go) was written first and failed to compile (`Link`/`Unlink`/`Replace`/`Count` undefined) before `assoc.go` existed; it passes on Postgres with `-tags integration -race`.
- S-83: the test and five added tests assert results and the hook statement count per helper (Link 1, Count 1, Unlink 1, Replace 4 events = BEGIN, clear, link, COMMIT; Replace of an empty set 3 events; BelongsTo and NULL keys 0).
- Mutation checks, all killed: m2m insert without ON CONFLICT; m2m Replace with the NOT inversion removed; discriminator write removed; HasX clear ignoring the parent key; m2m delete ignoring the parent key.
