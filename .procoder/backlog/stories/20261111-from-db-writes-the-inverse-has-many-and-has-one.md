# from-db writes the inverse has_many and has_one

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #91. Reproduce: run `momo generate --from-db` against a database
where `books.author_id` references `authors (id)`.

Observed: the generator writes `Author *Author` with `belongs_to` on `Book` and
nothing on `Author`. `Has`, `HasNo`, `LoadCounts` and eager loading of children
need the inverse, so each one is hand-written into a generated model file, which
a later `--from-db` run reports as a conflict.

Expected: the inverse is written on the referenced model: `has_one` when the
foreign key column is unique, `has_many` otherwise, with distinct names when one
child has two foreign keys to the same parent, and `introspect.relations` to
rename or skip one.

Root cause: `generator.foreignKey` in gen/fromdb/emit.go only records the
child-side `relation`; nothing walks those relations to add the parent side.

## Acceptance criteria

- [x] a regression test pins the fix: red before the change, green after
- [x] `introspect.inverse: true` writes `has_many` (plain foreign key) and `has_one` (unique foreign key column) on the referenced model
- [x] two foreign keys from one child to one parent give distinct, deterministic names (`MessagesBySender`, `MessagesByRecipient`)
- [x] no inverse for a referencing table that has no model (`only`, `skip`)
- [x] `introspect.relations` renames or skips one inverse
- [x] the default (option off) output is unchanged
- [x] a PostgreSQL integration test preloads the generated inverse relations
- [x] docs/migrations.md and an `Upgrading to 0.8.0` subsection in docs/upgrading.md describe it

## Evidence

- Regression test, red: ci-agent run 42977879827 (sha 3abfe72, branch worktree-agent-red-91, deleted). gen/fromdb/inverse_test.go failed to build (`unknown field Inverse in struct literal of type Config`, `undefined: RelationRule`) and cli TestGenerateFromDBInverse failed on the unknown `inverse` yaml key.
- Green (first pass): ci-agent run 42989097917, headSha e7892964220c706282d15e160821b94118950477, all jobs success.
- Review round, red: ci-agent run 42993453779 (sha 931d310, branch worktree-agent-red-91, deleted). TestInverseUnmatchedRuleWarns and TestInverseCollisionWarns failed in every integration job; the TestInverseKindMatrix cases (has_one for the whole primary key, unique constraint and unique index, has_many for partial, expression and composite unique keys, none for a composite foreign key) passed.
- Review round, green: ci-agent run 42993589803, headSha 215c5acd44e72e22863da267ba50c869ddc0c21a, success. A rule key that matches no inverse and a collision suffix each print a warning; the usage text and docs say relations needs inverse: true and is keyed by the name before any suffix.
- Unit tests cover kinds (has_many, has_one), two-foreign-key names, self reference, rules, only/skip and the unchanged default (gen/fromdb/inverse_test.go). The PostgreSQL integration test cli/generate_fromdb_inverse_integration_test.go preloads Books, AuthorProfile, MessagesBySender and MessagesByRecipient from the generated models.
- Docs changed: docs/migrations.md, docs/upgrading.md (Upgrading to 0.8.0), and the --from-db usage text in cli/generate.go.
