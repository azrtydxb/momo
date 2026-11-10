# Types: map google/uuid.UUID and []uuid.UUID without a type tag

Status: open
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: story

## Description

A field of `github.com/google/uuid.UUID` needs `momo:"type:uuid"` on every field, and `--from-db` needs an `introspect.types` rule to write it. Map `uuid.UUID`, `*uuid.UUID` and `[]uuid.UUID` to `uuid` and `uuid[]` in the type map, and let `--from-db` choose google/uuid with one config switch. GitHub issue #76.

## Acceptance criteria

- [ ] the tests are shown failing on CI before the change
- [ ] a model field of `uuid.UUID`, `*uuid.UUID` and `[]uuid.UUID` maps to uuid and uuid[] with no type tag, and round-trips on PostgreSQL
- [ ] one `--from-db` config switch generates google/uuid fields with no type tags
- [ ] the momo root module gains no new required dependency for users who do not use google/uuid, or the docs state the dependency

## Evidence
