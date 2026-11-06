# momo-06 Task 17: Association helpers Link, Unlink, Replace and Count

Status: done 2026-11-06
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 17: Association helpers Link, Unlink, Replace and Count (`.procoder/plans/momo-06-relations.md:3913`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-83. Files: `assoc.go`, `assoc_test.go`.

## Acceptance criteria

- [x] `TestAssociationHelpers` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-83] `TestAssociationHelpers` asserts results and statement count per helper; fails if a helper issues an undeclared statement.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestAssociationHelpers` (assoc_test.go) was written first and failed to compile (`Link`/`Unlink`/`Replace`/`Count` undefined) before `assoc.go` existed; it passes on Postgres with `-tags integration -race`.
- S-83: the test and five added tests assert results and the hook statement count per helper (Link 1, Count 1, Unlink 1, Replace 5 events = BEGIN, select, clear, link, COMMIT; Replace of an empty set 3 events; BelongsTo and NULL keys 0).
- Mutation checks, all killed: m2m insert without ON CONFLICT; m2m Replace with the NOT inversion removed; discriminator write removed; HasX clear ignoring the parent key; m2m delete ignoring the parent key.

### Review fixes (2026-11-06)

- Bind limit: Link and Unlink chunk with `keysPerStatement` (one statement per chunk) instead of returning ErrValidation; S-83 wording is now "one statement per chunk" (plan Task 17 reconciliation note updated). A `*DB` call with more than one chunk runs in one transaction. Replace SELECTs the current child keys inside its transaction, diffs in Go, and unlinks only removed and links only added keys in chunks; an empty set still clears in one statement.
- Tests (`assoc_chunk_test.go`, `assoc_internal_test.go`): chunk counts and rows with `WithMaxParams(4)` for m2m Link/Unlink/Replace and has-many Link/Unlink/Replace; `*DB` multi-chunk Link atomic on a later-chunk CHECK failure (m2m and has-many); Replace and Unlink of 70000 tags at the real PG limit; NOT NULL FK wrapped (`ErrNotNullViolation`, message says "nullable foreign key", SQLSTATE 23502 still reachable with errors.As); Replace keeps rows on a join table without a unique key; polymorphic Unlink, Replace and Replace(empty) leave another owner type's children with a colliding owner_id alone (images.owner_id made nullable in the test's scratch schema).
- Godoc: helpers write directly (no model hooks, field hooks or UpdatedAt); m2m Link needs a unique or primary key on the join columns.

### Re-review fixes (2026-11-06)

- After merging sprint-2 the test package failed `go vet` (`ids` and `mustExec` redeclared). The branch helper is now `assocQueryIDs`, the duplicate `mustExec` is removed; `go vet . && go vet -tags integration .` pass.
- `Replace` locks the parent row (`SELECT 1 ... FOR UPDATE`, dialects with `Locking`: PostgreSQL, MySQL; skipped on SQLite) before reading current keys. Godoc documents this, the lock lifetime on a `*Tx`, and that multi-chunk `Link`/`Unlink` on a `*Tx` has no savepoint.
- `TestAssociationConcurrentReplaceSameParent` (integration tag): two goroutines Replace the same parent with {1,3,4} and {2,3,5} from {1,2}, 20 rounds; asserts no error and the final set equals exactly one input. Mutation: removing the lock call makes it fail on the first run (final set mixed, `[3 4 5]`). Existing statement-count tests add one event when `Locking` is set.
