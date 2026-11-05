# momo-04 Task 18: Baseline and drift

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 18: Baseline and drift (`.procoder/plans/momo-04-migrations.md:2115`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11. Files: `migrate/baseline.go`. Contributes to S-10, S-11; those criteria are verified in the stories that define their named tests.

## Dependencies

- Hard dependency (Task 11): drift must call `postgres.Dialect.InspectWithInvalid`, not plain `Inspect`, and report every returned invalid index name as Changed with Detail "invalid index (failed CONCURRENTLY build)". A plain Inspect hides a failed CONCURRENTLY build.

## Hard requirement (Task 12 review)

A dialect hook `Canonicalize(ctx, conn, *schema.Schema) error` creates the declared views and functions in a scratch schema (dependencies first), reads back `pg_get_viewdef`, `pg_get_functiondef` and the policy and trigger expressions, and rewrites the declared text. Baseline stores the canonical text, not the inspected text. Without it the first Diff after baseline drops and recreates every view and its dependents, losing materialized view data.

## Acceptance criteria

- [x] Drift excludes `momo_migrations` and honours `InspectOptions.Exclude` (the dialect Inspect does not apply Exclude)
- [x] Baseline then diff gives zero changes for a declared view written differently from pg_get_viewdef's output, for a declared function with a differently formatted body, and for a materialized view (data kept)

- [x] `TestBaselineAndDrift` exists as written in the plan, failed before the implementation, and passes now
- [x] Drift reports invalid indexes (via `InspectWithInvalid`) as Changed with Detail "invalid index (failed CONCURRENTLY build)"
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

Tests were written first and were red: `go test ./migrate/ -run TestBaselineAndDrift` failed to compile with `e.Baseline undefined`, `e.Drift undefined`, `undefined: migrate.ErrBaselineMismatch` and `undefined: migratetest.CountingDriver`; `go vet -tags integration ./migrate/postgres/` failed with `d.Canonicalize undefined`. They pass now.

- Drift excludes `momo_migrations` and honours `Config.Exclude`: `TestDriftExcludesHistoryTableAndHonoursExclude` (history table present, `tmp_*` excluded, only `audit_log` unmanaged). `Engine.Inspect` filters the history table (story 086) and now also applies `Config.Exclude`, because the dialect Inspect does not.
- Baseline then diff, zero changes: `TestBaselineThenDiffKeepsViewsAndMatviewData` (migrate/postgres, integration) declares two views (one on the other), a materialized view, a SQL function with a differently formatted body, a trigger with a WHEN condition and a policy, all written differently from what PostgreSQL reports. Baseline succeeds, `Engine.Diff` returns nil, `Drift` is clean, and the materialized view still holds its 3 rows. Mutation check: with the Canonicalize call disabled in `Baseline` the test fails with a mismatch on kinds policy, trigger, view. `TestCanonicalizeRewritesDeclaredText` shows the rewritten view text equals the inspection and that no `momo_canon_*` schema is left behind; `TestCanonicalizeNoObjectsIsNoop` shows a schema without such objects runs no DDL.
- `TestBaselineAndDrift` as written in the plan, plus `TestBaselineWritesHistoryRows` (rows applied, statements_applied 0, apply_id baseline, checksum equal to momo.sum, later version stays pending, repeat is a no-op), `TestBaselineMismatchAndForce`, `TestBaselineNoSnapshot`, `TestBaselineGates` (ErrChecksumMismatch, ErrDirty), `TestDriftIsReadOnlyAndTreatsEverythingAsUnmanagedWithoutHistory` (no history table created, zero write statements through the counting driver), `TestDriftReportsMissingAndChanged`.
- Invalid indexes: `TestDriftReportsInvalidIndex` leaves a failed `CREATE UNIQUE INDEX CONCURRENTLY` behind and expects exactly one Changed entry, Detail "invalid index (failed CONCURRENTLY build)"; `TestBaselineRefusesInvalidIndex`. Drift and Baseline call `InspectWithInvalid` through the optional `migrate.InvalidInspector` interface.
- Commands, all green: `MOMO_REQUIRE_INTEGRATION=1 ... go test -tags integration -race -count=1 ./migrate/...`, `go vet ./...`, `go vet -tags integration ./migrate/...`, `gofmt -l .` (empty), `go test ./...`. `procoder check`: 0 blocking findings.

Design decisions:

- The `Canonicalizer` hook is an optional dialect interface (`migrate.Canonicalizer`, implemented by `postgres.Dialect.Canonicalize`). It creates the declared objects in a `momo_canon_<random>` scratch schema inside a transaction that is always rolled back, so no DDL persists. Baseline canonicalizes the snapshot it verifies; it does not rewrite snapshot files (the engine only reads the directory), so the stored text stays the declared text and a later Diff of the same models compares equal.
- Drift is read-only and runs no DDL, so it does not canonicalize: it compares views, policy and trigger expressions by identity (text blanked on both sides) and everything else after `schema.Normalized`, as the plan's guide section says.
- Children of a table present on both sides that exist only in the live database (for example an added column) are reported as Changed with Detail "not in snapshot", so Unmanaged lists only top-level objects as the plan test requires; children missing from the live database are Missing.
- The test databases always have the vector extension installed, so the test snapshots declare it.

Review fixes (2026-11-05):

- Invalid index on an excluded table: `Engine.inspectOn` now drops invalid index names whose table or view matches `Config.Exclude` (`dropExcludedInvalid`, run before the table is stripped, because the index is located through it). Red first: `TestBaselineIgnoresInvalidIndexOfExcludedTable` (ErrBaselineMismatch before) and `TestDriftIgnoresInvalidIndexOfExcludedTable` (Changed index before). `TestBaselineInvalidIndexControlWithoutExclude` shows the same database is refused without the exclusion, and the Drift test also shows an invalid index on a managed table is still reported. `TestDriftAppliesExcludeToSnapshotSide` kills the mutation "Drift stops applying exclude to the snapshot side" (verified: Missing table tmp_scratch appears with the mutation).
- Missing CREATE privilege: `postgres.Dialect.Canonicalize` checks `has_database_privilege(current_database(), 'CREATE')` before any statement and returns an error wrapping the new `migrate.ErrCanonicalizePrivilege`, naming the privilege and suggesting a role that has it. It never skips silently; force does not bypass it; a snapshot without views, functions, triggers and policies runs no DDL and needs no privilege. Documented on `Baseline` and `Canonicalizer`. Tests: `TestCanonicalizeNeedsCreateOnDatabase` (momo_app via MOMO_TEST_POSTGRES_APP_URL), `TestCanonicalizeWithoutObjectsNeedsNoPrivilege`, `TestBaselineSurfacesCanonicalizePrivilegeError` (force true and false, nothing recorded). Limitation found: Baseline as `momo_app` cannot get as far as Canonicalize, because `EnsureHistory` runs CREATE TABLE IF NOT EXISTS on the schema and fails with permission denied there first. That is a separate issue; the privilege error is verified at the Canonicalize level and through Baseline with a stand-in dialect.
- Drift ignoring view, trigger and policy text: new `DriftReport.Unverified []ObjectRef` lists views, triggers and policies on both sides that carry text which was not compared; `Clean` ignores it. Drift's doc comment says a body changed out of band is not detected and points to Unverified. Test: `TestDriftListsObjectsWhoseBodiesWereNotCompared` (policy and view changed out of band, report Clean, five objects listed). Follow-up story: `20261105-momo-04-drift-detects-out-of-band-view-trigger-and-policy.md`. Plan Task 18 has a reconciliation note.
- Baseline over later applied versions: the plan says "for every migration with version <= version that has no row", which is additive, so the behaviour is kept. `TestBaselineWithLaterVersionAlreadyApplied` pins it (later row untouched, deleted earlier row re-recorded).
