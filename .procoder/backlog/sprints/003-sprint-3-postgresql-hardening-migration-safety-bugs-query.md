# Sprint 3: PostgreSQL hardening - migration safety bugs, query and relations polish

Status: closed 2026-11-07
Created: 2026-11-06

## Goal

An application team can run Momo migrations against a production PostgreSQL
database without the known v0.2.0 traps: staged NOT NULL is enforced,
`Concurrently()` and the destructive directive work, apply refuses broken
chains, and history never leaks credentials. Projections respect soft
delete, joined and graph loading cover the edge cases the reviews found, and
conditional upserts work on SQLite partial indexes. Every fix ships with a
regression test and the docs' known-issues lists shrink to match.

## Result

committed: 21
done: 20 (20261105-momo-07-support-hnsw-scan-mem-multiplier-search-setting, 20261106-apply-creates-the-history-table-and-meta-row-even-when, 20261106-apply-does-not-run-chain-and-fork-checks, 20261106-combine-scoped-options-and-nested-loaders-on-one-relation, 20261106-diff-marks-a-whole-file-no-transaction-when-it-has-a, 20261106-force-unlock-is-a-no-op-for-every-dialect, 20261106-generate-meta-for-projection-structs, 20261106-graph-writes-polish-soft-delete-findorcreate-bulk-children, 20261106-history-error-column-is-not-url-redacted-and-url-is-visible, 20261106-index-concurrently-is-ignored-by-migrate-diff, 20261106-index-drop-constraint-drop-unique-and-check-are-not-flagged, 20261106-joined-loading-polish-column-named-null-error-fewer, 20261106-lock-timeout-and-statement-timeout-of-0s-cannot-disable-the, 20261106-no-warning-for-not-null-with-a-volatile-default, 20261106-no-way-to-accept-an-edited-applied-migration, 20261106-not-null-staging-refusal-says-populated-table-but-fires-for, 20261106-projections-over-soft-delete-tables-skip-the-deleted-at, 20261106-rename-candidate-exception-skips-staging-for-not-null, 20261106-staged-not-null-is-never-enforced-after-diff-stage, 20261106-the-momo-destructive-directive-is-never-enforced)
carried: 1 (20261106-conditional-upsert-follow-ups-sqlite-target-literals-excluded-in-target)

## Retro

**What slowed us down.** Six parallel branches touched the same files: migrate/engine.go, migrate/errors.go, docs/ai-guide.md and docs/migrations.md. Every merge after the first needed conflict work, and the staged NOT NULL plus no-transaction split interaction only surfaced at merge time. Agents also kept polling CI with sleep timers, which flooded the session with stale notifications. Review rounds ran 2 to 3 per branch.

**What we change.** Group stories that touch the same file into one branch, or order those branches and rebase each before review. Agents push and stop; the coordinator watches CI with one background poll every 4 minutes. Doc code spans avoid `(...)` and nested calls from the start, so the commit gate does not block merges.

**Keep.** A fresh reviewer over the combined merge result, not only over each branch. It found the staged-snapshot interaction across two branches, and the pinning test TestStagedColumnWithConcurrentIndexSplit now holds it.
