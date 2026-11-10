# --from-db: an include list (introspect.only)

Status: open
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: story

## Description

`momo generate --from-db` models every table except those matched by `introspect.skip`. A project that moves onto momo one module at a time needs models only for the tables it has moved. Add `introspect.only: [glob, ...]`, the inverse of `skip`, with the same effect on the init snapshot and `migrate.exclude`. GitHub issue #73.

## Acceptance criteria

- [ ] the tests are shown failing on CI before the change
- [ ] `introspect.only` limits `--from-db` models, the init snapshot and `migrate.exclude` to the matching tables; a test shows a table outside `only` gets no model and is excluded from migrate
- [ ] `only` and `skip` together: `skip` applies within `only`; documented in docs/importers.md or the --from-db docs and in docs/upgrading.md under 0.7.0

## Evidence
