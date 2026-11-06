# Index Concurrently() is ignored by migrate diff

Status: open
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query
Type: bug
Severity: s3

## Description

Reproduction: declare an index with `Concurrently()` on a model (`MomoConfig`),
run `momo migrate diff`. The generated migration contains a plain
`CREATE INDEX`, with no `CONCURRENTLY` and no `-- momo:no-transaction`.

Observed vs expected: `gen/companion.go:528-531` parses `Concurrently()` into
`ix.Concurrently`, and `migrate/postgres/render.go:127-128` and `518-519` would
emit `CONCURRENTLY` and mark the migration no-transaction. But
`migrate/diff.go:60-61` diffs `schema.Normalized` snapshots, and normalization
sets `ix.Concurrently = false` (`schema/opclass.go:39`; documented at
`schema/normalize.go:269,303`). `indexEqual` also ignores it
(`migrate/diff.go:584`). The renderer never sees the flag, so diff always writes
a plain `CREATE INDEX`, which takes a SHARE lock and blocks writes on the table
for the whole build. On a large table that is an outage. Expected: diff emits
`CREATE INDEX CONCURRENTLY` with `-- momo:no-transaction`.

Workaround (v0.1.0): edit the generated file by hand to `CREATE INDEX
CONCURRENTLY`, add `-- momo:no-transaction`, keep it as a separate migration
containing only that statement (see the related story about a whole file
becoming no-transaction), then run `momo migrate hash` to refresh momo.sum.

## Acceptance criteria

- [ ] a regression test pins the fix: red before the change, green after
- [ ] a diff of a model index declared with `Concurrently()` emits `CREATE INDEX CONCURRENTLY` and the file carries `-- momo:no-transaction`
- [ ] the concurrent index is emitted in its own migration file, not mixed with other statements (related s3 story: diff-marks-a-whole-file-no-transaction-when-it-has-a)
- [ ] a diff with no `Concurrently()` change still reports no changes (idempotent against the snapshot)
- [ ] docs/migrations.md section 5 and its known-issues entry are updated when fixed

## Evidence

<!-- Filled at close time. -->
