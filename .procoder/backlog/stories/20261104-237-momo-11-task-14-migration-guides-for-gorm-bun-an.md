# momo-11 Task 14: Migration guides for GORM, Bun and Ent users

Status: open
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: -

## Description

Implements plan momo-11-adoption Task 14: Migration guides for GORM, Bun and Ent users (`.procoder/plans/momo-11-adoption.md:1566`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-95. Files: `docs/guides/gorm.md`. Contributes to S-95; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] Step 1: Failing check
- [ ] Step 2: Write the guides. GORM guide: Models (struct tags to Momo `momo:` tags and `MomoConfig`), Queries (`db.Find`/`Where` to `momo.Select[T]`), Relations (`Preload` to `momo.Load` and `.With`), Hooks (`BeforeCreate` to typed model hooks, S-85), Transactions (`db.Transaction` to `db.Tx`, no autom…
- [ ] Step 3: Run the check, expect PASS for the headings check (`grep -c` returns 7 each).
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

