# Schema: composite foreign keys (issue #54)

Status: open
Created: 2026-11-10
Epic: momo-04-migrations-core-postgresql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

A multi-tenant schema uses composite foreign keys as an isolation guarantee: `(company_id, document_id) REFERENCES document(company_id, id)` makes linking rows of two tenants impossible. momo cannot declare one today. `belongs_to` refuses a composite target, there is no `Config.ForeignKey`, and `momo generate --from-db` drops them, so `baseline` reports a mismatch. The ask is in GitHub issue #54. It was found by the Kuvryn Owl Enterprise port, where almost every one of 36 tables carries such a key, and one key has three columns.

## Acceptance criteria

- [ ] `MomoConfig` can declare a composite foreign key (`c.ForeignKey(name, fields, refTable, refColumns)` with `OnDelete`/`OnUpdate`), referencing the target's primary key or any unique constraint on it; a two- and a three-column case are covered by tests
- [ ] the generated schema and the PostgreSQL migration render `FOREIGN KEY (a, b) REFERENCES t (x, y) ON DELETE CASCADE`; diff adds, alters and drops it like a single-column key (integration test on PostgreSQL)
- [ ] `momo generate --from-db` emits the declaration, and `baseline` on a database with composite keys reports no mismatch (integration test)
- [ ] docs (schema/migrations pages, ai-guide) describe the declaration; issue #54 is referenced in the commit
