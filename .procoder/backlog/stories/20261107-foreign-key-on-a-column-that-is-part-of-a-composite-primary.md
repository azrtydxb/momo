# Foreign key on a column that is part of a composite primary key

Status: open
Created: 2026-11-07
Epic: momo-02-generator
Sprint: 004-sprint-4-adoption-gaps-from-kuvryn-license-composite-pk

## Description

Teams adopting momo on an existing PostgreSQL database (issue #13, kuvryn-license) have tables such as `exams` with primary key (license_id, exam_id) where license_id references licenses(license_id). The generator refuses a belongs_to on a column that is part of a composite primary key, so models cannot describe the schema and `migrate diff` proposes dropping the foreign key. Done means such a foreign key is declared in models, emitted in DDL, and matches the live schema with no diff.

## Acceptance criteria

- [ ] A belongs_to on a column that is part of the referencing table's composite primary key generates without error and emits FOREIGN KEY (license_id) REFERENCES licenses(license_id)
- [ ] migrate diff against a database that already has that foreign key proposes no change (integration test on CI)
- [ ] Composite foreign key targets are either supported or refused with an ErrValidation-style message naming the columns (test)
- [ ] docs describe the declaration

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
