# momo-04 Task 2: Schema normalization and FuzzNormalize

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 2: Schema normalization and FuzzNormalize (`.procoder/plans/momo-04-migrations.md:233`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1, S-11. Files: `schema/normalize.go`.

## Acceptance criteria

- [ ] `TestNormalizeTypes` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestNormalizeExprDBRewrites` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEqualIgnoresOrder` exists as written in the plan, failed before the implementation, and passes now
- [ ] `FuzzNormalize` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-1] `TestSchemaJSONRoundTrip` and `FuzzSchemaSort` (schema package): JSON round-trips losslessly and `Sort` makes equivalent schemas byte-identical; fails if a field is dropped from serialization or Sort is order-sensitive.
- [ ] [S-11] `TestStagedChange` on a populated DB: add nullable column → data migration backfill → SET NOT NULL applies and all rows satisfy the constraint; fails if diff emits NOT NULL directly for a populated table without a default.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

