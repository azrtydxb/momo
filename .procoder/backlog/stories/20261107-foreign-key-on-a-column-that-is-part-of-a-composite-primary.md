# Foreign key on a column that is part of a composite primary key

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-02-generator
Sprint: 004-sprint-4-adoption-gaps-from-kuvryn-license-composite-pk

## Description

Teams adopting momo on an existing PostgreSQL database (issue #13, kuvryn-license) have tables such as `exams` with primary key (license_id, exam_id) where license_id references licenses(license_id). The generator refuses a belongs_to on a column that is part of a composite primary key, so models cannot describe the schema and `migrate diff` proposes dropping the foreign key. Done means such a foreign key is declared in models, emitted in DDL, and matches the live schema with no diff.

## Acceptance criteria

- [x] A belongs_to on a column that is part of the referencing table's composite primary key generates without error and emits FOREIGN KEY (license_id) REFERENCES licenses(license_id)
- [x] migrate diff against a database that already has that foreign key proposes no change (integration test on CI)
- [x] Composite foreign key targets are either supported or refused with an ErrValidation-style message naming the columns (test)
- [x] docs describe the declaration

## Evidence

- Criterion 1: TestBelongsToOnCompositePKColumn and TestDescriptorsForCompositePKReferencer (gen), CI run 42343239959 success; FK exams_license_id_fkey FOREIGN KEY (license_id) REFERENCES licenses(license_id) emitted.
- Criterion 2: TestCompositePKForeignKeyNoDiff (migrate/postgres integration, pg16 and pg17), CI run 42343239959 success; diff against live tables is empty.
- Criterion 3: TestBelongsToCompositeTargetRefused (gen), CI run 42343239959 success; composite targets are refused with a message naming the columns.
- Criterion 4: docs/relations.md and docs/ai-guide.md describe the declaration; docs gate clean.
- Correction: Relation.ChildPK is not only a chunk-size input. assoc.go uses it to match child keys in Link, Unlink and Replace (currentKeys, childPKs, updateChunk), so a child whose PK contains its foreign key relies on it being the full composite key.
- Limit: has_many Unlink and Replace write NULL into the child foreign key, which the database rejects when that column is part of the child primary key; docs/relations.md says to delete such children instead.
- Generated code: TestCompositeFKGeneratedCodeBuilds (gen) emits the compositefk fixture and runs go build over it on CI. TestCompositePKChildRelationsIntegration (root, integration tag) runs CreateGraph, Preload of License.Exams and a joined Exam to License load against PostgreSQL; its descriptors are hand-written mirrors of the generated ones, since generated scratch packages cannot be imported from a runtime test.
- Integration runs: the CI integration job runs make test-integration-postgres (Makefile line 48) with MOMO_REQUIRE_INTEGRATION=1 and -tags integration, so testdb fails rather than skips; a green job means the integration-tagged tests ran. CI run 42375905383 green on 122a2ac.
