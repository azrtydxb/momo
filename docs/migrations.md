# Migration safety and recovery

Momo v0.1.0, PostgreSQL only. Every statement here was checked against
`migrate/`, `cmd/momo/`, `schema/` and `dialect/`; the tests that pin a
behavior are named. For the first run see README, [Create and apply the
migration](../README.md#3-create-and-apply-the-migration).

Only PostgreSQL migrations exist. `momo migrate` with another dialect exits 2
("migrations for dialect ... are not available yet").

## 1. Model

```text
models (Go structs) --momo migrate diff--> <v>_<name>.up.sql + <v>_<name>.snapshot.json + momo.sum
                                              |
database <--------momo migrate apply---------+  (history table: momo_migrations)
```

### Files

All files live in one directory per dialect (default `migrations/postgres`,
set `migrate.dir.postgres` or `--dir`). `diff` writes exactly three files:

| File                             | Content                                                                                                                                                                        |
| -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `<version>_<name>.up.sql`        | Statements. Header `-- momo:generated`, plus `-- momo:destructive` and/or `-- momo:no-transaction` when needed. Statements are separated by a blank line and each ends in `;`. |
| `<version>_<name>.snapshot.json` | The whole schema after this migration, wrapped as `{format_version, version, parent, schema}`. `schema` carries its own `format_version`.                                      |
| `momo.sum`                       | Checksum manifest of every file in the directory, rewritten by `diff`.                                                                                                         |

- `version` is a 14-digit UTC timestamp (`YYYYMMDDHHMMSS`). If the clock is not
  past the latest existing version, `diff` uses the second after it, so
  versions only grow.
- `name` must match `^[a-z][a-z0-9_]*$`.
- `.down.sql` files are accepted by the file scanner and hashed, but never run.
  There is no rollback (section 2).
- Files whose name starts with `.` are ignored everywhere (`.DS_Store`,
  `.momo-*.tmp`). A symlink to a directory inside the migration directory is an
  error. Moving an applied file into a subdirectory is fine: history is keyed by
  version, not path (`TestMovingAppliedMigrationIntoArchiveKeepsHistory`).
- `diff` never overwrites a file (exit 1 if one exists) and writes all-or-none:
  temporaries first, then publish, `momo.sum` last.

### Format versions

| Artifact         | Field                                 | v0.1.0 value | Newer than this binary        |
| ---------------- | ------------------------------------- | ------------ | ----------------------------- |
| Snapshot wrapper | `format_version`                      | 1            | read error, "upgrade momo"    |
| Embedded schema  | `schema.format_version`               | 1            | read error, "upgrade momo"    |
| `momo.sum`       | header `# momo.sum format=N`          | 1            | read error                    |
| History table    | `checksum` of the row `__momo_meta__` | 1            | apply refuses, "upgrade momo" |

A snapshot without `format_version` is rejected.

### Diff

`momo migrate diff <name>` runs offline (no database, no driver). It loads the
models, compares them with the snapshot of the highest version in the
directory (the empty schema when there is none), orders the changes by
dependency and renders SQL. No difference: prints `no changes`, exit 0, writes
nothing.

- Renames are never guessed. A drop plus an add prints a
  `hint: possible rename ...`; pass `--rename-table old=new` or
  `--rename-column table.old=new`.
- Dropped extensions and schemas are not dropped in the database; `diff` prints
  a hint that they are no longer managed.
- Diff never reads the database. A table that is already populated is
  indistinguishable from an empty one (see section 5).

`diff` does not depend on `momo generate`. `momo generate --check` verifies only
the generated `*_gen.go` files (exit 1 and `stale: <path>` lines when stale).
There is no `momo migrate diff --check`. To fail CI when the models have no
migration, run `momo migrate diff ci_probe` on a clean checkout and fail unless
it prints `no changes` (when there is a difference it writes files and exits 0,
so also check `git status --porcelain` on the migration directory). This recipe
is derived from the code, not covered by a test.

### Checksums

Two independent checks, both SHA-256 of the file bytes:

1. `momo.sum` must match the directory: every file present, none added,
   removed or changed. `Apply`, `Status` and `Baseline` refuse otherwise
   (`ErrChecksumMismatch`, naming the files). A missing `momo.sum` is an error
   that names `momo migrate hash`. `momo.sum` itself carries a directory hash;
   a hand-edited `momo.sum` is rejected.
2. The history row of an applied migration stores the checksum of its up file.
   `Apply` refuses when that file changed since (the error says
   `... changed after version V was applied`).

`Plan` and `Drift` do not check either.

### History table

`momo_migrations`, created unqualified, so it lives in the first schema of the
session `search_path`. `Config.Schemas[0]` (`migrate.schemas`) must equal
`current_schema()` or the engine returns a configuration error naming both.

| Column               | Type             | Meaning                                                                                   |
| -------------------- | ---------------- | ----------------------------------------------------------------------------------------- |
| `version`            | TEXT PRIMARY KEY | Migration version                                                                         |
| `name`               | TEXT             | Migration name                                                                            |
| `checksum`           | TEXT             | SHA-256 of the up file                                                                    |
| `status`             | TEXT             | `running`, `applied`, `failed`, `rolled_back` (and `meta`)                                |
| `statements_applied` | INTEGER          | Statements that completed                                                                 |
| `error`              | TEXT             | Failure text, empty on success                                                            |
| `applied_at`         | TEXT             | RFC 3339 UTC                                                                              |
| `execution_ms`       | BIGINT           | Duration                                                                                  |
| `apply_id`           | TEXT             | UTC timestamp plus 8 hex digits, shared by one `apply` run; `baseline` for baselined rows |
| `kind`               | TEXT             | `sql`                                                                                     |

One extra row, `version = '__momo_meta__'`, `status = 'meta'`, holds the
history format version. `ReadHistory` and `Inspect` hide it and the table.
`running` and `failed` are "dirty". `rolled_back` is not dirty.

### How apply decides what to run

1. Take the migration lock, set session timeouts, create the history table if
   missing.
2. Refuse (`ErrDirty`, exit 3) if any row is `running` or `failed`.
3. Refuse (`ErrChecksumMismatch`, exit 3) on a checksum problem (above).
4. Pending = every `*.up.sql` whose version has no row other than
   `rolled_back`, run in ascending version order, optionally stopping after
   `--to N` (a version without a file is refused, so a typo applies nothing).

The test is "has no row", not "is newer than the latest applied". An older file
that appears later (a merged branch) is still applied, after newer ones.
`apply` does not check snapshot parents; `validate` does (section 2).

### Drift

`momo migrate drift` inspects the live database and compares it with the
snapshot of the **latest applied** migration (the empty schema if none, so
every object is unmanaged). Exit 0 and `no drift`, or exit 1 and one line per
finding:

| Line                               | Meaning                                                                                                                                                                                                                   |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `changed: <kind> <name>: <detail>` | In both, definition differs. A column, index or constraint present only live is `changed: ... not in snapshot` on that object. An invalid index is `changed: ... invalid index (failed CONCURRENTLY build)`.              |
| `missing: <kind> <name>`           | In the snapshot, not in the database.                                                                                                                                                                                     |
| `unmanaged: <kind> <name>`         | In the database, not in the snapshot: tables, views, functions, enums, schemas, **extensions** (every extension except `plpgsql`, for example `pg_stat_statements`, `pgcrypto`), and triggers/policies on managed tables. |
| `unverified: <kind> <name>`        | Not compared and does not affect the exit code (below).                                                                                                                                                                   |

- Extension versions are ignored when either side leaves the version empty.
- Hide objects you do not manage with `migrate.exclude` (globs over `name` or
  `schema.name`; they apply to tables, views, functions, enums, extensions,
  triggers and policies). Example: `exclude: ["pg_stat_statements", "audit_*"]`.
- `momo_migrations` is never reported.
- A view query, trigger `WHEN` and policy `USING`/`WITH CHECK` changed outside
  migrations is **not detected**; those objects are listed as `unverified:`
  only. Exit 0 does not prove their text matches. Function bodies are compared.
- Declared text (defaults, checks, index predicates) is canonicalized through a
  scratch schema in a rolled-back transaction, which needs `CREATE` on the
  database. Without it drift still runs but can show false `changed` entries
  (never hides a real one).

## 2. CLI

`momo` has three commands: `generate`, `migrate`, `help`. Global exit codes:
0 success, 1 findings, 2 usage/config, 3 database/lock/execution (migrate).

Shared flags on every `migrate` subcommand: `--config`/`-c momo.yaml`, `--dir`,
`--dialect`, `--url`.

| Command                                                                                                               | Needs DB        | Does                                                                                          | Exit 1 when                                                                                     |
| --------------------------------------------------------------------------------------------------------------------- | --------------- | --------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| `diff <name> [--rename-table a=b]... [--rename-column t.a=b]... [--allow-destructive] [--stage] [--enforce-not-null]` | no              | Write a migration (section 1, 5).                                                             | destructive or staged refusal; target file exists; model diagnostics                            |
| `plan`                                                                                                                | yes             | List pending migrations with statement counts. Read-only, no lock, creates nothing.           | never                                                                                           |
| `apply [--to N] [--lock-wait 30s]`                                                                                    | yes             | Apply pending.                                                                                | a chain refusal (`ErrBranchConflict`, `ErrChainInvalid`, `ErrOutOfOrder`); other failures are 3 |
| `status`                                                                                                              | yes             | One line per version: `applied`, `pending`, `dirty`, `mismatch`, `missing`. Read-only.        | any version not `applied`/`pending`                                                             |
| `validate [--db]`                                                                                                     | no (`--db` yes) | Directory checks (below); `--db` adds `status`.                                               | any problem or unhealthy status                                                                 |
| `hash`                                                                                                                | no              | Recompute `momo.sum`, list files it accepts as `changed:`/`added:`/`removed:`, write it.      | never                                                                                           |
| `baseline --version N [--force]`                                                                                      | yes             | Record migrations through N as applied without running them (section 7).                      | database differs from snapshot                                                                  |
| `drift`                                                                                                               | yes             | Section 1.                                                                                    | any change, missing or unmanaged                                                                |
| `resolve --version N --applied` or `--rolled-back`                                                                    | yes             | Settle a dirty migration (section 4).                                                         | never                                                                                           |
| `rehash --version N [--accept]`                                                                                       | yes             | Show, and with `--accept` store, the new checksum of an edited applied migration (section 4). | without `--accept`, when the file differs                                                       |

`validate` checks: files parse and directives are known; `momo.sum` matches;
versions unique and increasing; each snapshot's parent exists; no two snapshots
share a parent (branch conflict); each up file has a snapshot; a hand-written
migration (no `-- momo:generated`) has a snapshot equal to its parent's. Use it
in CI. `apply` runs the chain checks too (fork, missing parent, cycle, gap) and
refuses with the same message (a fork is `ErrBranchConflict`; a missing parent,
cycle or gap is `ErrChainInvalid`; both exit 1, like `validate` findings), and it
refuses with `ErrOutOfOrder` (exit 1) when a pending version is older than the
latest applied one, so a late-merged older migration is never applied after newer
ones (`TestApplyRefusesForkedChain`, `TestApplyRefusesPendingOlderThanApplied`,
`TestApplyRefusesBrokenChain`, `TestMigrateChainRefusalsExitOne`).

Recovering from `ErrBranchConflict` or `ErrOutOfOrder` (there is no `rebase`
command yet). Both mean two teammates branched from the same snapshot and both
merged: two snapshots share a parent, and one of the new migrations has a
version older than the other. Fix it in the files of the branch that merged
last, before it is applied anywhere. The late migration is still pending, so it
needs no `rehash`:

1. Delete its `<version>_<name>.up.sql` and `<version>_<name>.snapshot.json`
   (a generated migration; for a hand-written one rename the `.up.sql` to a
   version later than the newest applied one and copy the newest snapshot to
   the same new version, with `parent` set to the previous snapshot).
2. Run `momo migrate diff <name>`. It compares the models with the latest
   snapshot and writes the migration again with a new version and the right
   parent.
3. Run `momo migrate hash`, review the `changed:/added:/removed:` list, and run
   `momo migrate validate`.

If the older migration was already applied somewhere, do not rewrite it: leave
the chain as it is and add a new migration for the difference.

Not built in v0.1.0 (story 098): `rebase`, `lint`, `rollback`, `new`. Each exits
2 with `migrate <name> is not available yet`. There are no down migrations: undo
a change with a new forward migration.

There is no `--force-unlock`: the PostgreSQL lock is a session advisory lock that
the server frees when its holder's connection ends (section 3). `--lock-wait`
exists on `apply` only.
`--lock-wait 0s` fails immediately when locked. `resolve` and `baseline` wait
the default 30 s.

Database URL: `--url`, else the variable named in `migrate.url_env.postgres`,
else `MOMO_DATABASE_URL`. Prefer the environment, or `--url -`, which reads the
URL from standard input (`printf '%s' "$URL" | momo migrate apply --url -`): a
`--url` value shows in the process list of a shared host, and momo prints a
warning on stderr when it sees one.

`momo.yaml` `migrate:` keys: `dir`, `url_env` (maps keyed by dialect),
`schemas`, `lock_timeout`, `statement_timeout` (Go durations), `exclude`.

## 3. Apply semantics on PostgreSQL

One connection is pinned for the whole run. Order: lock, set timeouts, history
table, migrations, reset timeouts, unlock (reset runs first so a small
`statement_timeout` cannot cancel the unlock). Cleanup runs on a
context detached from Ctrl-C and has two steps: the two RESETs share one 5 s
bound, and the unlock has its own.

### Lock

`pg_try_advisory_lock(id)` on the pinned session, polled every 200 ms until
`--lock-wait` (default 30 s) passes. `id` = FNV-64a of
`momo:<history schema>.momo_migrations`, so schemas are independent. On timeout
`apply` exits 3 with
`migration lock is held by another process: held by pid=N application_name=X`
(`TestApplyLockWaitTimesOutWithHolder`; waiting
callers proceed once it is released, `TestApplyWaitsForLockRelease`).

The lock is session-scoped: it is released when the holder's connection ends,
so a crashed process frees it. There is no lock row to delete and no flag to force
the lock (`--force-unlock` was removed: it never did anything). If a lock appears stuck, find the session
(`pg_stat_activity` by the pid in the message) and terminate it with
`pg_terminate_backend`.

`apply`, `resolve` and `baseline` take the lock. `plan`, `status`, `drift`,
`validate` do not.

### Transactions

- **Default: one transaction per migration file**, not per batch. The
  `running` history row, all statements and the update to `applied` commit
  together. A second migration failing never undoes a first that committed.
- **Marked `-- momo:no-transaction`: the whole file runs with no
  transaction**, one autocommitted statement at a time, with `statements_applied`
  updated after each. Every statement in that file loses atomicity, not only
  the concurrent one. Only the directive selects this mode on apply; `apply`
  does not inspect statements. `diff` writes the directive when it renders a
  concurrent index. A hand-written file must carry the directive itself:
  otherwise PostgreSQL rejects `CREATE INDEX CONCURRENTLY` inside a
  transaction, the version is recorded `rolled_back`, and it can be retried
  after you add the directive. `diff` never mixes the two modes in one file:
  when a diff holds both transactional DDL and statements that must run outside
  a transaction, it writes two migrations, the transactional one first and a
  trailing `<name>_no_tx` one that carries the directive (one second later
  version). A failure in the first file rolls it back whole. A diff with only
  such statements writes the single no-transaction file.
- Statements run one by one with no bind parameters. The splitter respects
  strings, comments and `$$` bodies; `-- momo:split` disables dollar-quote
  awareness and splits at every semicolon.

`CREATE INDEX CONCURRENTLY` applies correctly when it is in a no-transaction
file: the index is valid and the version is `applied`
(`TestCreateIndexConcurrentlyApply`). `diff` produces it from `Concurrently()`
on a model index, and for `ADD UNIQUE` and the `VALIDATE CONSTRAINT` of an
`ADD CHECK` (section 5).

### Session timeouts

Applied with `SET` on the pinned session, `RESET` at the end:

| Setting             | momo.yaml                   | Default | Notes                                                                                                                                                                                                                                                                                                         |
| ------------------- | --------------------------- | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `lock_timeout`      | `migrate.lock_timeout`      | 5 s     | A DDL statement that cannot get its table lock within the limit fails with SQLSTATE 55P03; the transaction rolls back and the version stays retryable. Key absent: 5 s. `0s` sets `lock_timeout = 0`, which disables the limit.                                                                               |
| `statement_timeout` | `migrate.statement_timeout` | not set | Left as the role or server default. A role-level `statement_timeout` therefore applies to long migrations such as index builds. Use a migration role without it. Key absent: untouched. `0s` sends `SET statement_timeout = 0`, which disables the limit even when the role has one (for a long index build). |

If `RESET` fails, the connection is discarded rather than returned to the pool.

### PgBouncer and other poolers

Run migrations on a direct connection or a session-pooled one. Under
transaction pooling the advisory lock, the `SET lock_timeout` and the
`search_path` belong to a server connection that the next statement may not
reuse, so the lock does not protect and the timeouts can leak or not apply.
The driver's default query mode is PgBouncer-safe for application queries; the
migration engine through PgBouncer is **not tested** in v0.1.0. A startup
`search_path` is also not reliable through a pooler, so set the schema on the
role or on a direct connection.

## 4. Failure and recovery

Check state with `momo migrate status` (exit 1 when anything is dirty,
mismatched or missing). Read `error`, shown after the colon.

### Transactional migration fails (`TestPostgresApplyRecovery`)

DDL and the history row roll back; nothing the file did persists. A
`rolled_back` row records the error. That is not dirty:
`status` shows `pending <v> <name>: <error>`, and the next `apply` retries
from statement 1. Fix the cause (or fix the file, section "editing", then
`momo migrate hash`) and run `apply` again. Data-exception errors (SQLSTATE
class 22) store only the code, not the message, because the message may quote
data.

### Interrupted (Ctrl-C, killed, lost connection)

- Transactional: the server aborts the transaction; no `running` row survives.
  Same as above. After a Ctrl-C, rollback and the failure row are still written
  (5 s bound).
- No-transaction: the `running` row was committed, so the version stays
  dirty (`running` or `failed`). The lock is released when the connection dies.

### No-transaction migration fails (partial state)

Statements before the failing one are committed. The row is `failed`, with
`statements_applied = n` and the error. Every later `apply` stops with
`ErrDirty` (`TestApplyRefusesDirtyAndReleasesLock`) until you resolve it.

1. `momo migrate status`, note the version and `statements_applied`.
2. Inspect the database. Statement `n+1` is the one that failed.
3. Make the database match one of two states by hand with `psql`:
   - finish the file yourself, then `momo migrate resolve --version V --applied`
     (the row becomes `applied`, error cleared, checksum of the file as it was
     when the run started is kept); or
   - undo the committed statements, then
     `momo migrate resolve --version V --rolled-back` (the row is deleted; the
     version is pending again and will run from statement 1). A file may be
     edited before the next apply: the version has no applied row, so edit,
     `momo migrate hash`, `apply`.
4. `resolve` takes the migration lock and errors when the version has no
   `running` or `failed` row.

### Invalid index after a failed `CREATE INDEX CONCURRENTLY`

PostgreSQL leaves the index in place, marked invalid, when the build fails
(for example a uniqueness violation or a cancelled build). Momo does not use
`IF NOT EXISTS`, so re-running fails with "already exists" until you drop it.

1. `momo migrate drift` shows `changed: index <table>.<name>: invalid index (failed CONCURRENTLY build)`.
2. `DROP INDEX CONCURRENTLY IF EXISTS <schema>.<name>;` in `psql`.
3. Fix the data or definition.
4. `momo migrate resolve --version V --rolled-back` (if the row is `failed`),
   then `momo migrate apply`. If the failing file contained other statements,
   handle them per the previous subsection first.

`baseline` also counts an invalid index as a mismatch.

### Editing an already-applied migration

`apply` and `status` refuse (`checksum mismatch`, naming the file; status shows
`mismatch`). **`momo migrate hash` does not fix this.** It rewrites
`momo.sum`, but the history row still holds the old checksum, so `apply` still
refuses. Options:

- Revert the file (`git checkout -- <file>`), then write a new migration for the
  change. This is the supported path.
- If the edit is cosmetic (whitespace or a comment) and you accept it: run
  `momo migrate hash` (momo.sum), then `momo migrate rehash --version V` to see
  the recorded and the new checksum, then `momo migrate rehash --version V --accept`
  to store it. Without `--accept` nothing changes and the command exits 1 and says
  so. With it, only the row of version V is updated, and the row's `error` column
  records `checksum accepted by rehash at <time>: <old> -> <new>`, which `status`
  shows. The edit is accepted, not run: the database is not changed, so rehash
  only edits that do not alter what the migration did
  (`TestRehash`, `TestRehashOnPostgres`, `TestMigrateRehashAcceptsEditedMigration`).

Editing a migration that is not applied yet (status `pending`) is fine: run
`momo migrate hash` to accept it. `hash` records whatever is on disk, so review
the printed `changed:/added:/removed:` list first.

A history row whose file is gone shows `missing` and exit 1 on `status`.

### Idempotency

`apply` with nothing pending prints `no pending migrations`, exits 0, and
runs no migration. It takes the lock (an advisory lock, not a write) but writes
nothing: it does not create the history table or its meta row, so it works with a
read-mostly role. When something is pending, the history table is created first
(`TestApplyNothingPendingWritesNothing`,
`TestApplyNothingPendingCreatesNoHistoryTable`). Running it twice,
or two processes at once, is safe: the second waits for the lock, then finds
nothing to do (`TestConcurrentApply`, `TestConcurrentApplyWaits`).
`plan`, `status`, `drift` are read-only.

## 5. Production safety checklist

What Momo enforces, what it only hints at, what it does not see.
A complete worked upgrade (add column, index, rename) is `examples/upgrade-database`.

### Refused unless you opt in (`diff`, exit 1)

`--allow-destructive` is required for: `drop_table`, `drop_column`, `drop_enum`,
and a column type change (`alter_column` with a different type). The file then
carries `-- momo:destructive`. Review these. Type changes render as
`ALTER COLUMN ... TYPE ... USING col::type`: a full table rewrite under an
`ACCESS EXCLUSIVE` lock, which fails on rows that do not cast.

Not destructive, but reported as `risky:` hints by `diff` (no flag needed):
dropping an index, primary key, unique, check or foreign key; adding a unique
or check constraint (both can fail on existing data).

- `ADD CHECK` is written `NOT VALID`; its `VALIDATE CONSTRAINT` (a full scan
  that does not block writes) goes in the trailing no-transaction migration.
  If the validation fails, the trailing migration is `failed` and `apply` stops
  with `ErrDirty`. The constraint stays `NOT VALID`: it still checks new rows.
  Recovery: fix the offending rows, run
  `ALTER TABLE t VALIDATE CONSTRAINT c;` in `psql`, then
  `momo migrate resolve --version V --applied` (V is the `_no_tx` version).
  `resolve --version V --rolled-back` runs the whole file again from its first
  statement, so it is safe only when the file holds just the failed
  `VALIDATE`. If earlier statements of the file (a
  `CREATE UNIQUE INDEX CONCURRENTLY`, another `VALIDATE`) already committed,
  they would fail with "already exists": use `--applied` after finishing by
  hand, or undo those statements first (see "No-transaction migration
  fails").
- `ADD UNIQUE` on an existing table is `CREATE UNIQUE INDEX CONCURRENTLY`
  followed by `ADD CONSTRAINT ... UNIQUE USING INDEX`, both in the trailing
  no-transaction migration. On duplicate rows the build fails and leaves an
  `INVALID` index (`drift` reports it): drop it with
  `DROP INDEX CONCURRENTLY IF EXISTS`, fix the data, then `resolve --rolled-back`
  and re-apply (the unique index is the first statement of its file; if other
  statements of that file already committed, undo them first).
- On a partitioned table (including a sub-partitioned one) `ADD UNIQUE` is a
  plain, transactional `ADD CONSTRAINT ... UNIQUE`, and an index declared with
  `Concurrently()` is created and dropped without `CONCURRENTLY`, because
  PostgreSQL rejects `CONCURRENTLY` there. `ADD CHECK` keeps the `NOT VALID`
  form (`TestPartitionedTableConstraintsAndIndexes`).
- `diff` leaves out the explicit drop of an index, unique, foreign key or
  primary key that lists a column dropped in the same diff: `DROP COLUMN`
  removes it, and an explicit drop after it would fail. A check or an index
  expression or `WHERE` that merely mentions a dropped column's name anywhere
  in its text cannot be classified reliably, so its drop is written
  `DROP ... IF EXISTS`, which is correct whether or not the column took it.
- Limit: an inline foreign key in a new `CREATE TABLE` cannot reference a
  unique constraint that the same diff adds to another table, because that
  unique is created in the later no-transaction migration and the foreign key
  would fail. Workaround: run `diff` twice, first the unique on the existing
  table, then the new table with its foreign key.
- Not flagged at all: dropping a view, function, trigger or policy, and any
  change you write by hand.

The `-- momo:destructive` header is informational. `diff` writes it and
`--allow-destructive` is the only guard, and that guard exists in `diff` only:
`apply` does not read the header or scan statements, so a hand-written file
with a `DROP` applies without confirmation. Review every `.up.sql` in code
review.

### NOT NULL columns

An added `NOT NULL` column (or an existing column made `NOT NULL`) with no
default, identity or generation expression is refused (`ErrStagedRequired`)
for any table that already exists in the snapshot. `diff` never queries the
database, so it cannot tell whether the table has rows: an empty existing
table is refused too. A column that looks like a rename (same table and type
as a dropped column) is refused the same way; the message names
`--rename-column`. With `--stage`, then `--enforce-not-null`:

1. `diff --stage` adds the column as nullable and prints the hint
   `staged: T.C was added as nullable; backfill it, then run migrate diff with --enforce-not-null to emit ALTER TABLE T ALTER COLUMN C SET NOT NULL`.
   The new snapshot records the column as nullable, like the database will
   have it, so `validate` and `status` agree after you apply.
2. Apply, backfill in batches outside the migration.
3. Check the backfill: `SELECT count(*) FROM T WHERE C IS NULL` must return 0.
4. `diff <name> --enforce-not-null` emits
   `ALTER TABLE T ALTER COLUMN C SET NOT NULL` and records `NOT NULL` in the
   snapshot. A plain `diff` (without the flag) is still refused, because only
   you know the backfill is done. Apply it. The statement scans the table
   under a lock; do it when the table is quiet or add a validated `CHECK`
   first by hand.

Several columns can be staged in separate migrations before any backfill:
each stays recorded as nullable until enforced, and one `--enforce-not-null`
emits `SET NOT NULL` for all of them. The flag enforces every column that is
nullable in the snapshot and `NOT NULL` without a default in the model, not
only staged ones, so backfill all of them first. New `NOT NULL` columns stay
refused.

If NULLs remain, `apply` of the `SET NOT NULL` migration fails (exit 3). The
migration is transactional, so it rolls back: the version is `rolled_back`
(not dirty), `status` shows it as pending with the error, and nothing
changed. Finish the backfill and run `apply` again. Only a hand-edited
`-- momo:no-transaction` file can leave a dirty row; settle that with
`momo migrate resolve --version V --applied` or `--rolled-back` (section 4).

The same applies to an existing column made `NOT NULL`: `diff --stage` writes
nothing (the snapshot already has the column nullable; the hint is only
printed when the same run writes other changes), and `diff <name> --enforce-not-null` emits
the `SET NOT NULL` after the backfill. `--enforce-not-null` only affects
existing columns: a new `NOT NULL` column without a default still needs
`--stage`.

A `NOT NULL` column with a default is not staged. A constant default is cheap
on PostgreSQL 11 and later. A volatile default (`random()`,
`clock_timestamp()`, `gen_random_uuid()`, `nextval(...)` and a few more
well-known functions) rewrites the table under `ACCESS EXCLUSIVE`; `diff`
prints a `risk:` hint (PostgreSQL only) for an `ADD COLUMN` with one on an existing table. It
only knows the built-in names it lists; a function you wrote yourself is not
checked.

### Other items

- **Backfills**: Momo has no data-migration step. Do them in a hand-written
  migration (section 6) or a separate job. A long backfill inside a
  transactional migration holds its locks until commit and can hit
  `lock_timeout`/`statement_timeout`.
- **Large-table indexes**: declare `Concurrently()` on the index in
  `MomoConfig`. `diff` then writes `CREATE INDEX CONCURRENTLY` in its own
  no-transaction migration, so the build does not block writes. Without
  `Concurrently()` the index is a plain `CREATE INDEX` that blocks writes while
  it builds. Set a role without `statement_timeout` for the run. A hand-written
  alternative: `examples/custom-sql-migration`.
- **Lock timeout**: the default 5 s lock wait fails fast instead of queuing
  behind a long transaction. A retry is safe for transactional migrations.
- **Deploy order**: migrate first, then roll out code, with every migration
  backward compatible with the running code (add before use; drop only after no
  code reads it; rename as add, copy, switch, drop).
- **Run once**: one `apply` job per deploy. Concurrent applies are safe but
  wait up to `--lock-wait`.
- **Backups**: take a backup or snapshot before any destructive, type-change
  or no-transaction migration. PostgreSQL DDL rolls back only inside a
  transaction; no-transaction files cannot be rolled back.
- **CI**: run `momo generate --check`, `momo migrate validate`, and the
  model-vs-migrations probe from section 1 on every change.
- **After**: `momo migrate status` (exit 0) and `momo migrate drift`.
- **Roles**: `CREATE EXTENSION` (pgvector, and any extension the models
  declare) needs a role allowed to create it. A missing package or privilege is
  reported, with exit 3, as
  `extension "x" is not installed (install its server package and run CREATE EXTENSION as a role that may)`.

## 6. Custom SQL migrations

Worked example: `examples/custom-sql-migration`.

There is no `momo migrate new --custom` in v0.1.0. You can still add hand-written
SQL by creating the files yourself:

1. Choose a version greater than the latest and a name:
   `20260301120000_backfill_slug.up.sql`. Do not put `-- momo:generated` in it.
2. Optional directives at the top, before the first statement:
   `-- momo:no-transaction`, `-- momo:split`, `-- momo:destructive`
   (informational, never enforced), `-- momo:generated` (do not use it here)
   and `-- momo:sqlite-foreign-keys-off` (SQLite only). An unknown `-- momo:` directive is an error.
3. Create `20260301120000_backfill_slug.snapshot.json` as a copy of the previous
   snapshot with `version` set to the new version and `parent` set to the
   previous version. The `schema` must stay equal to the parent's, or
   `validate` reports "custom migration ... must not change the schema".
4. `momo migrate hash`, `momo migrate validate`, then `apply`.

If the SQL changes the schema, change the models too and run `diff` afterward,
which compares against the latest snapshot as written; otherwise the next `diff`
will try to generate the same change again.

## 7. Adopting an existing database

Worked example: `examples/adopt-existing-schema`.

Momo has no command that turns a database into models or a snapshot.
(`migrate.Engine.Inspect` returns the live schema as a Go value, but nothing
in the CLI writes it.) Procedure:

1. Write the models so they match the database.
2. `momo migrate diff init` against the empty directory writes the initial
   migration and snapshot. Do not apply it.
3. `momo migrate baseline --version <init version>`. It checks the live
   database against that snapshot, including view, policy and trigger text, and
   exits 1 with `differing object kinds: ...` on a mismatch, then records every
   migration up to that version as `applied` with `statements_applied = 0` and
   `apply_id = baseline`, without running them. `--force` records anyway; use it
   only after reviewing the mismatch. Schema objects outside the models, such as
   extra extensions, count as a mismatch: add them to `migrate.exclude`.
4. From then on use `diff` and `apply`.

`baseline` needs `CREATE` on the database for the canonicalization step when the
snapshot has views, functions, triggers or policies; `--force` does not waive
that. Without that privilege it fails with an error naming it. It takes the
lock and refuses on dirty or checksum problems like `apply`.

A database that uses `serial` or `bigserial` ids is adopted with the `serial`
tag, as in `pk,serial` on an `int64` field. The tag renders `smallserial`,
`serial` or `bigserial` by the Go integer width. Inspection reads a column
whose default is `nextval` of a sequence owned by the column back as the same
serial column, so the snapshot matches and `diff` stays empty. `serial` is
PostgreSQL only; other dialects refuse it. Turning an existing column into or
out of serial is refused by `diff`, with an error that asks for a
hand-written migration.

Upgrade note: inspection used to report a serial column as `identity`. If you
adopted a serial database with `pk,identity` on the model, or kept a snapshot
taken from one, `diff` now stops with `column "id" is a serial column in the
database but the model declares identity`. Change the tag to `pk,serial` and
regenerate the snapshot; or write a migration that converts the column to
identity. Momo does not treat the two as equal, because a new environment
would then be built with identity while production keeps its serial.

`AutoMigrate` is a Go API for development and tests. It applies additive
changes only, writes no history and no files, and returns the rest as
`Skipped`. Do not mix it with versioned migrations on one schema.

## 8. Security

- `momo` redacts connection strings from everything it prints (stdout, stderr,
  error text, and bad flag values that a pasted URL lands in): URL userinfo
  becomes `***@`, a URL in text that has a query keeps the part before `?` and
  shows `?***`, a `password=`/`passwd=`/`pwd=` value becomes `***`, and the
  exact URL and password given are replaced by `***`.
  History `error` text is the server message only, never `Detail`. Messages of
  SQLSTATE class 22 (data exception) are withheld because they can contain
  data values. Connection URLs and `password=` values in the text are replaced
  by `***` before it is stored (`TestHistoryErrorIsRedacted`).
- A warning on stderr (not a failure) when a command that needs a database is
  given a URL with `sslmode=disable` and any non-local host:
  `TLS is disabled (sslmode=disable) for remote host H: credentials and data cross the network unencrypted; use sslmode=require or stronger`.
  It names the hosts only.
  `localhost`, `127.0.0.1`, `::1` and unix socket paths do not warn. Use
  `sslmode=verify-full` against a real server.
- Do not pass the URL with `--url` on shared hosts (momo warns); use
  `migrate.url_env`, `MOMO_DATABASE_URL` or `--url -` (stdin).
- Review `*.up.sql` in code review like any code: `apply` runs it as the
  connected role, and `momo.sum` only detects change, not intent.

## Upgrading: generated file header

Generated Go files start with `// Code generated by momo (format 1). DO NOT EDIT.`.
The header carries the generated-file format number, which decides
compatibility, and no momo release version, so a later momo upgrade does not
rewrite generated files whose code is unchanged. Earlier releases wrote
`// Code generated by momo v0.1.0-dev (format 1). DO NOT EDIT.`; momo still
recognises that form, so orphan detection keeps working.

After upgrading, run `momo generate` once. Until then `momo generate --check`
reports the header change in every generated file.

## Known issues in v0.1.0

Each is filed as a bug story in `.procoder/backlog/stories/<id>.md`.

None open. Fixed issues are listed in the changelog.
