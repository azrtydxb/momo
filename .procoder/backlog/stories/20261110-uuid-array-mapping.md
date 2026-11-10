# Types: uuid[] mapping and typed array operators on UUIDs (issue #57)

Status: open
Created: 2026-11-10
Epic: momo-05-writes-queries
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

A `uuid[]` column has no Go mapping, so it needs a `type:` override and `arrayop` predicates on it are untyped. Apps hold such columns (`department_ids`) and query them with `= ANY($1::uuid[])`. The ask is in GitHub issue #57, from the Kuvryn Owl Enterprise port.

## Acceptance criteria

- [ ] `[]types.UUID` maps to `uuid[]` on PostgreSQL (generator, inspector, `--from-db`), with a round-trip integration test (insert, select, nil vs empty)
- [ ] `[]uuid.UUID` from github.com/google/uuid maps too if it needs no new dependency in the root module; otherwise the story records why not
- [ ] `arrayop.AnyOf`, `Overlaps` and `Contains` are typed on the UUID array column and render `= ANY($1::uuid[])` / `&&` / `@>` (unit tests pin SQL, integration test runs them)
- [ ] the generated type-mapping table and the docs list the mapping; issue #57 is referenced in the commit
