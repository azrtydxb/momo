# momo-06 Task 16: Relation predicates, traversal and count ordering

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 16: Relation predicates, traversal and count ordering (`.procoder/plans/momo-06-relations.md:3589`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-76. Files: `relpred.go`, `relpred_test.go`.

## Acceptance criteria

- [x] `TestRelationPredicates` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestRelationTraverseAndOrder` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-76] `TestRelationPredicates` asserts SQL shape (EXISTS) and rows for each; fails if a predicate duplicates parents via JOIN.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `go test ./ -run 'TestRelationPredicates|TestRelationTraverseAndOrder'` failed to compile (`document.Comments.Has undefined`, also `HasNo`, `From`, `CountOrder`). Green on PostgreSQL with `-tags integration -race` after `relpred.go`.
- S-76: `TestRelationPredicates` asserts `EXISTS (SELECT 1 FROM` / `NOT EXISTS (SELECT 1 FROM` and no `JOIN` in the SQL for Has and HasNo, plus rows ([1 3], [2 4]); many-to-many Has goes through `momo_j` and returns [1 3]. `TestRelationPredicateShapes` adds self-reference (`HasFrom`), composite key, polymorphic discriminator and composite traversal.
- Mutation checks (each made the relation tests fail, then restored): drop NOT in HasNo, correlate on first key column only, CountOrder ignores direction, drop scope predicates, `IN` to `NOT IN` in traversal, drop the discriminator (killed only after adding the polymorphic case).
- Deviations are recorded in the plan's Task 16 reconciliation note (2026-11-06).
- Review fixes (2026-11-06): `From` on a many-to-many relation now carries the parents' Querier, so both `SQL()` and `All()` return `ErrValidation` (`TestRelationFromManyToManyIsValidation`; failed with "query has no Querier" before). New soft-deletable fixture child `Note` (boolean marker, `document.Notes`); `TestRelationPredicatesSkipSoftDeletedChildren` covers Has, HasNo and CountOrder. `TestRelationFromPolymorphicDiscriminator` (colliding author-owned image) and `TestRelationFromSelfReferencing` (employee reports and manager) added. Has godoc notes the many-to-many join table gets no soft-delete filter.
- Review mutations, each killed then restored: drop the child soft-delete filter (Has returned [1 2 3]), drop the discriminator in `From` (author image leaked in), many-to-many error query without Querier.
