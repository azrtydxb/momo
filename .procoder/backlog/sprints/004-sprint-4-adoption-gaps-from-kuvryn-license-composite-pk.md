# Sprint 4: adoption gaps from kuvryn-license - composite-PK foreign keys, serial ids, aliases, INSERT SELECT, typed expressions

Status: closed 2026-11-07
Created: 2026-11-07

## Goal

An existing PostgreSQL application such as kuvryn-license can describe its live schema in momo models with no hand-edited init migration (composite-key foreign keys, serial ids), and write its store queries without raw SQL fragments: aliased select-list expressions, INSERT ... SELECT, and typed substr, CASE, column arithmetic and current_schema. Closes GitHub issues #13 to #17.

## Result

committed: 7
done: 7 (20261107-alias-select-list-expressions, 20261107-declare-serial-and-bigserial-ids, 20261107-expression-helpers-coalesce-cast-string-functions, 20261107-foreign-key-on-a-column-that-is-part-of-a-composite-primary, 20261107-insert-select, 20261107-tuple-result-rows, 20261107-typed-expressions-substr-case-arithmetic-current-schema)
carried: 0

## Retro

**What slowed us down.** The gaps came from the first real port, kuvryn-license, one issue at a time, because "done" had meant the spec's checkboxes, not real store code. Mid-sprint the repo moved off the external DATA disk: every agent worktree lost its git link, the agents could not be resumed, and Go's env file still pointed GOCACHE and GOMODCACHE at the missing disk, which blocked every commit at the gate until it was reset. Hand-written golden expectations failed CI twice (parenthesised predicates, a pointer method on a non-addressable value).

**What we change.** Measure completeness against real application code: the gap audit in .procoder/research/kuvryn-license-gap-audit.md becomes the checklist, and v0.3.0 waits until the license port can finish. Nothing runs on the laptop; CI is the only verifier. Keep tool caches and repos on the main disk.

**Keep.** A fresh re-review of the review-fix commits, not only the first pass: it caught an alias accepted as a keyset key, a new bug introduced by the fix itself.
