# Schema: composite foreign keys (issue #54)

Status: done
Created: 2026-11-10
Epic: momo-04-migrations-core-postgresql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

A multi-tenant schema uses composite foreign keys as an isolation guarantee: `(company_id, document_id) REFERENCES document(company_id, id)` makes linking rows of two tenants impossible. momo cannot declare one today. `belongs_to` refuses a composite target, there is no `Config.ForeignKey`, and `momo generate --from-db` drops them, so `baseline` reports a mismatch. The ask is in GitHub issue #54. It was found by the Kuvryn Owl Enterprise port, where almost every one of 36 tables carries such a key, and one key has three columns.

## Acceptance criteria

- [x] `MomoConfig` can declare a composite foreign key (`c.ForeignKey(name, fields, refTable, refColumns)` with `OnDelete`/`OnUpdate`), referencing the target's primary key or any unique constraint on it; a two- and a three-column case are covered by tests
- [x] the generated schema and the PostgreSQL migration render `FOREIGN KEY (a, b) REFERENCES t (x, y) ON DELETE CASCADE`; diff adds, alters and drops it like a single-column key (integration test on PostgreSQL)
- [x] `momo generate --from-db` emits the declaration, and `baseline` on a database with composite keys reports no mismatch (integration test)
- [x] docs (schema/migrations pages, ai-guide) describe the declaration; issue #54 is referenced in the commit

## Evidence

CI: ci-agent run 42780348454, headSha 69fa9dcd7162eae24ea58216e294601a079f3ec0, all nine jobs green (lint, unit, fuzz-smoke, bench-module, apidiff-docs, integration postgres, mysql84, mysql97, mariadb).

- Declaration (`c.ForeignKey(...).OnDelete/OnUpdate`, two- and three-column cases, refusals for a non-unique target, an unknown table and an unknown column): gen/composite_fk_declared_test.go (TestCompanionForeignKey, TestCompanionForeignKeyRefusals), fixtures gen/testdata/fixtures/companionfk and companionfkbad; reader gen/companion.go, builder gen/schema.go (companionForeignKeys).
- Rendering, diff add/alter/drop on PostgreSQL: migrate/postgres/composite_fk_declared_test.go (TestDeclaredCompositeForeignKeys).
- `--from-db` emission and baseline with no mismatch: gen/fromdb/fromdb_test.go (TestGenerateFromDBCompositeForeignKey), cmd/momo/generate_fromdb_integration_test.go (TestGenerateFromDBBaseline, with a composite key in legacyDDL).
- Docs: docs/ai-guide.md, docs/migrations.md, docs/reference.md (regenerated from CI output); commit trailer Refs #54.
- Not built (optional in the issue): a `belongs_to` form over a composite pair.

