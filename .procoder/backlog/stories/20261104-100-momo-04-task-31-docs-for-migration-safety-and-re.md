# momo-04 Task 31: Docs for migration safety and recovery

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 31: Docs for migration safety and recovery (`.procoder/plans/momo-04-migrations.md:3262`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-21. Files: `docs/migrations.md`, `docs/README.md`. Contributes to S-21; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] Step 1: Failing check
- [ ] Step 2: Write the guide with sections: Files and formats (`momo.sum`, snapshots, history table, the format versions), Commands and exit codes (copy the Task 29 table), Transactional and non-transactional migrations, Recovering a dirty migration (`momo migrate status`, repair manually, `momo migrate…
- [ ] Step 3: Run the check, expect PASS, then commit `git commit -m "docs: migration safety and recovery guide"`.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- docs/migrations.md already covers files and formats, commands and exit codes, locks, transactional and no-transaction migrations, dirty-migration recovery with migrate resolve, destructive changes, baseline, drift, branch conflicts and roles, and docs/README.md indexes it. TestMigrationsGuideSections (internal/docscheck) pins those sections; it passed on the red run because the guide already existed, so it is a guard, not a red-to-green proof.
- Left open: Step 2 also names SQLite table rebuilds and the SQLite momo_migration_lock, which do not exist in the code (docs/capabilities.md: migrations are PostgreSQL only), and --force-unlock was removed, so those parts cannot be written.
