# from-db writes the inverse has_many and has_one

Status: open
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

- [ ] a regression test pins the fix: red before the change, green after
- [ ] `introspect.inverse: true` writes `has_many` (plain foreign key) and `has_one` (unique foreign key column) on the referenced model
- [ ] two foreign keys from one child to one parent give distinct, deterministic names (`MessagesBySender`, `MessagesByRecipient`)
- [ ] no inverse for a referencing table that has no model (`only`, `skip`)
- [ ] `introspect.relations` renames or skips one inverse
- [ ] the default (option off) output is unchanged
- [ ] a PostgreSQL integration test preloads the generated inverse relations
- [ ] docs/migrations.md and an `Upgrading to 0.8.0` subsection in docs/upgrading.md describe it

## Evidence

<!-- Filled at close time. -->
