# Inspect models attached partition indexes as standalone

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #86. Reproduce on PostgreSQL: create a range-partitioned table
with a DEFAULT partition, `CREATE INDEX` on the parent (PostgreSQL creates and
attaches an index on each partition), then `ALTER TABLE ... RENAME COLUMN` on
the parent so the partition indexes keep their old names.

Observed: inspection reports each attached partition index as a standalone
index of the partition. `generate --from-db` emits it as `c.Index(...)` on the
partition model; applying that to an empty database gives the partition a
duplicate index, and `drift --strict` reports the PostgreSQL-created index as
`not in snapshot`.

Expected: only the parent index is modeled; PostgreSQL owns the partition
indexes.

Root cause: `qIndexes` in `migrate/postgres/inspect_index.go` excluded
constraint-backed indexes but not indexes that have a `pg_inherits` parent.
(Partition PK and unique constraints were already skipped by
`conparentid = 0` in the constraint query.)

## Acceptance criteria

- [x] a regression test pins the fix: red before the change, green after
- [x] inspecting a partitioned table with a parent index and two partitions models the index only on the parent
- [x] applying the inspected model to an empty schema creates no duplicate partition indexes and diffs clean
- [x] docs/migrations.md and docs/upgrading.md describe the change

## Evidence

- Red: ci-agent run 42980092749 (test-only commit, branch worktree-agent-red-86, deleted): `--- FAIL: TestInspectSkipsIndexesAttachedToParentIndex` with `inspect_partition_index_test.go:55: ev_2026 must not carry the attached index` (index `ev_2026_org_id_created_at_idx` modeled on the partition).
- Green: ci-agent run 42980096188 at headSha fc64f9d89767ebf61e156b57f5362a2947cb983b, all 9 jobs success (integration postgres, lint, apidiff-docs, unit, mysql, fuzz-smoke, bench-module).
- The test also applies the inspected model to an empty schema: each partition has 2 indexes (pkey + attached) and the follow-up diff is empty.
- Docs changed: docs/migrations.md, docs/upgrading.md (Upgrading to 0.8.0). No exported API change, so docs/reference.md is unchanged.
