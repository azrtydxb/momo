# momo-04 Task 31: Docs for migration safety and recovery

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 31: Docs for migration safety and recovery (`.procoder/plans/momo-04-migrations.md:3262`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-21. Files: `docs/migrations.md`, `docs/README.md`.

## Acceptance criteria

- [ ] [S-21] `TestExamples` runs every example end to end against both databases in CI; docs/ contains architecture, capability matrix, migration safety and recovery guide, contributor guide and supported versions; fails if an example is not wired into the test.
- [ ] Step 1: Failing check
- [ ] Step 2: Write the guide with sections: Files and formats (`momo.sum`, snapshots, history table, the format versions), Commands and exit codes (copy the Task 29 table), Transactional and non-transactional migrations, Recovering a dirty migration (`momo migrate status`, repair manually, `momo migrate…
- [ ] Step 3: Run the check, expect PASS, then commit `git commit -m "docs: migration safety and recovery guide"`.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

