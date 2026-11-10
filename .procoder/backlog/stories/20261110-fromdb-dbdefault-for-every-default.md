# --from-db: dbdefault for constant defaults (DEFAULT 1)

Status: open
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: story

## Description

`--from-db` adds `dbdefault` only to NOT NULL columns with a computed default. `version bigint NOT NULL DEFAULT 1` is generated as `default:1`, and `Insert.Model` writes 0 for an unset field. Add `introspect.dbdefault: all` (integrator decision 2026-11-10), which tags every column that has a default, and document that hand-written models can use `dbdefault` on any column. GitHub issue #77.

## Acceptance criteria

- [ ] the tests are shown failing on CI before the change
- [ ] with `introspect.dbdefault: all`, a `DEFAULT 1` column gets `dbdefault`, and `Insert.Model` with the field unset stores 1 (integration test on PostgreSQL)
- [ ] docs/upgrading.md (0.7.0) and the --from-db docs describe the option

## Evidence
