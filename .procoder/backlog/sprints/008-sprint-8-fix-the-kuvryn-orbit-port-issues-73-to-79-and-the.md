# Sprint 8: fix the Kuvryn Orbit port issues #73 to #79 and the open follow-ups

Status: closed 2026-11-11
Created: 2026-11-10

## Goal

Kuvryn Orbit can port its remaining modules without workarounds:

- **`--from-db`:** it can model a subset of tables, writes only the functions those tables use, and keeps every database default.
- **Raw reads:** they scan into plain structs of any width.
- **UUIDs:** `google/uuid` works without a type tag.
- **Writes:** a `RETURNING` write has a one-row helper.
- **Registry:** its SQL matches what runs.

The flaky `TestApplyCancelledStillUnlocks` gets a named cause and a fix, and the generator simplify cuts land. The sprint ships as v0.7.0.

## Result

committed: 10
done: 10 (20261110-fromdb-dbdefault-for-every-default, 20261110-fromdb-functions-only-those-modeled-tables-use, 20261110-fromdb-introspect-only-include-list, 20261110-gen-one-primary-key-lookup-and-no-redundant-schema-guards, 20261110-google-uuid-without-type-tag, 20261110-raw-query-into-plain-struct, 20261110-registry-insert-sql-depends-on-values, 20261110-testapplycancelledstillunlocks-races-the-server-releasing-a, 20261110-write-returning-one-row-helper, 20261111-fromdb-singularizer-names-table-releases-as-releas)
carried: 0

## Retro

What slowed us down:

- **The first fix of a word list caused a regression.** The `-se` singularizer fix (#82) passed CI with the words it was given, then broke `aliases`, `biases`, `caches` and similar words. It took a second red-then-green round to fix.
- **`--from-db` needed three review rounds.** An `only` list made `migrate diff` plan a `DROP TABLE` for unmanaged tables. Glob matching differed between `fromdb` and `migrate`. A blank `only` entry and an `only` list that matched nothing both reported success. CI was green each time, because no test covered these paths.
- **release-please wrote the wrong breaking-change note.** The squash commit still carried footers from earlier sprint commits, so release-please announced a `dbdefault` break that is not in this release. The release PR's changelog had to be rewritten by hand.
- **Releases were held up between steps.** After each green check, the release waited for the integrator to come back to it, instead of running through merge, tag, Latest and closing issues in one go.

What we change next sprint:

- **A word-list fix gets an opposing list first:** words the current rule already handles correctly go into the test before the rule changes.
- **A filter option (`only`, `skip`, include lists) gets a test for every consumer of the filtered set** (generate, inspect, diff), and a test for an empty or unmatched filter.
- **The squash commit body is the PR body only.** Old commit messages and their footers are deleted from the squash body, so release-please reads only this release's breaking changes.
- **A release is run by one script:** wait for checks, merge, verify the tag, mark Latest and close the issues, with no wait for the integrator in between.

One adaptation worth keeping:

- **Every piece of new behaviour was first shown failing on a throwaway branch,** and the run is cited in the story. That is how the singularizer regression and the embedded-cycle stack overflow were caught before `main`.
