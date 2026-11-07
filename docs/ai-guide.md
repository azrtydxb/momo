# Momo guide for AI coding agents

Target: `github.com/azrtydxb/momo` v0.1.0. Read this file instead of the Momo
source. Every identifier below was checked against v0.1.0. Where the guide says
DON'T, the call either fails, silently does the wrong thing, or is a known bug.

Imports used throughout:

```go
import (
	"github.com/azrtydxb/momo"                    // core: DB, Tx, Select, Insert, Update, Delete, errors
	"github.com/azrtydxb/momo/driver/pgxdriver"   // PostgreSQL driver (pgx v5)
	"github.com/azrtydxb/momo/vector"             // vector.Vector, HalfVector, SparseVector, Bits
	"github.com/azrtydxb/momo/types"              // types.UUID, Date, JSON[T], TimeOfDay, Hstore
	"yourmodule/models"                           // your structs
	"yourmodule/models/task"                      // GENERATED: one package per model (lowercase model name)
)
```

Deeper docs: [README](../README.md), [relations](relations.md),
[migrations](migrations.md), [vectors](vectors.md), [capabilities](capabilities.md),
[supported versions](supported-versions.md), [type mapping](TYPE_MAPPING.md),
[architecture](ARCHITECTURE.md), [examples](../examples/README.md).

## 1. Scope

- v0.1.0 supports PostgreSQL 16 and 17 with pgvector 0.8.x, through pgx v5 only.
- SQLite and MySQL/MariaDB are NOT usable. There is no driver for either and
  `momo migrate` exits 2 for them. Do not write code for them, do not set
  `dialects:` to anything but `[postgres]`.
- Go 1.26 or newer. Pre-v1: no compatibility promise; pin the exact version.
- Forward migrations only. No rollback, no `migrate new`, no `lint`, no `rebase`.
- No command turns a database into models. Write models by hand.
- Hybrid (full text plus vector) search, `MERGE` builder and window-function API
  do not exist. Use `momo.SQL` / `momo.TypedSQL` for those.

Details: [capabilities.md](capabilities.md). Example: [quickstart](../examples/quickstart/).

## 2. Install and tooling

```sh
go get github.com/azrtydxb/momo@v0.1.0                # adds the library to go.mod and writes go.sum
go install github.com/azrtydxb/momo/cmd/momo@v0.1.0   # the CLI: generate, migrate, help
```

Run `go get` first in a fresh module: it produces `go.sum`, which `go run` and
the generated code's compilation need. `go install` puts `momo` in
`$(go env GOPATH)/bin`; make sure that is on `PATH`.

Layout of a project (the `models` package holds structs; `momo generate` writes
into it):

```text
go.mod
momo.yaml
models/models.go          # yours
models/momo_gen.go        # generated (MomoMeta for each model). Commit it.
models/task/momo_gen.go   # generated: columns and relations of Task. Commit it.
migrations/               # momo migrate diff writes here. Commit it.
```

Minimal `momo.yaml` (at the module root; the CLI finds it by walking up to `go.mod`):

```yaml
version: 1
packages:
  - ./models
dialects: [postgres]
migrate:
  dir:
    postgres: migrations
  url_env:
    postgres: DATABASE_URL
```

`naming:` is optional: `momo` (default: snake_case columns, plural snake_case
tables), `gorm`, `bun`, `ent`. Pick one and never change it: a change renames
columns.

Generate:

```go
//go:generate momo generate
```

```sh
momo generate            # writes *_gen.go
momo generate --check    # CI: exit 1 and "stale: <path>" lines when stale; writes nothing
```

Rules:

- DO run `momo generate` after every change to a model struct or tag, before
  compiling code that uses the generated packages.
- DO commit generated files. DON'T edit `*_gen.go` and DON'T name your own
  files `*_gen.go` (the generator refuses to overwrite non-generated files).
- DO put `*_gen.go text eol=lf` in `.gitattributes` if anyone uses Windows, or
  `--check` reports CRLF files as stale.
- DON'T use `go run` of the CLI inside app code; the library (`momo`) never
  imports the generator.

Details: README "Configure and generate". Example: [quickstart](../examples/quickstart/).

## 3. Connecting

```go
func openDB(ctx context.Context, url string) (*momo.DB, func(), error) {
	pool, err := pgxdriver.NewPool(ctx, url) // (ctx, string) (*pgxpool.Pool, error)
	if err != nil {
		return nil, nil, err
	}
	return momo.New(pgxdriver.New(pool)), pool.Close, nil // New(pool, opts ...Option) momo.Driver
}
```

- The caller owns and closes the pool. `momo.DB` has no `Close`.
- `*momo.DB` is safe for concurrent use; create one at startup and share it.
- DSN: `postgres://user:password@host:5432/dbname?sslmode=verify-full`. Use
  `sslmode=disable` for local development only. Momo adds no TLS options; pgx
  reads `sslmode`, `sslrootcert`, ... from the URL.
- Read the URL from the environment, never hard-code credentials.
- Pool sizing: Momo documents no recommendation and sets nothing. pgxpool
  options (for example `pool_max_conns=N` in the URL) apply. Each open `Tx`,
  `WithSearch` block, `Replace` and `CreateGraph` holds one connection for its
  whole duration.

Other constructors:

```go
sqlDB, drv := pgxdriver.FromConnConfig(cfg, opts...) // cfg *pgx.ConnConfig; returns (*sql.DB, momo.Driver)
drv = pgxdriver.FromSQLDB(sqlDB)                     // sqlDB must be backed by pgx stdlib; for an existing *sql.Tx see SQLTx, AdoptTx and UnwrapSQLTx in section Transactions
```

`database/sql` paths have no Batch or COPY path (relation loading falls back to
sequential statements; correct, just slower).

PgBouncer (transaction pooling):

- `pgxdriver.New` defaults to `pgx.QueryExecModeExec` (unnamed statements,
  server-side binding). That is PgBouncer-safe; set nothing.
- `pgxdriver.WithExecMode(m)` accepts `pgx.QueryExecModeCacheStatement`,
  `pgx.QueryExecModeCacheDescribe`, `pgx.QueryExecModeDescribeExec` (import
  `github.com/jackc/pgx/v5`; not tested through PgBouncer). The simple protocol is rejected
  with `ErrUnsupported` on every query. DON'T select it.
- With `FromConnConfig` set `cfg.DefaultQueryExecMode = pgx.QueryExecModeExec`
  first; pgx defaults to `CacheStatement`, which is NOT PgBouncer-safe. Its
  `opts` have no effect.
- DON'T run `momo migrate` through a transaction pooler (section 5).
- Set `search_path` per role or per transaction, not at connection startup.

Details: [capabilities.md](capabilities.md) "Getting connected",
[supported-versions.md](supported-versions.md) "PgBouncer".

## 4. Modeling

A model is a struct that embeds `momo.Model` BY VALUE. The tag on that embedded
field names the table. The tag grammar is `momo:"opt,opt:value,..."`; a value
cannot contain a comma. An unknown, duplicate or malformed option is a generator
diagnostic and nothing is written for that package.

```go
type Account struct {
	momo.Model `momo:"table:accounts"`
	ID         int64      `momo:"pk,identity"`
	Email      string     `momo:"size:200,unique"`
	Plan       string     `momo:"size:20,default:'free',index"`
	Note       *string    `momo:"size:500"`                         // pointer = nullable
	Balance    int64      `momo:"default:0"`
	Version    int64      `momo:"version,default:0"`                // optimistic lock
	CreatedAt  time.Time  `momo:"column:created_at,created_at"`     // set by Momo on insert
	UpdatedAt  time.Time  `momo:"column:updated_at,updated_at"`     // set by Momo on update
	DeletedAt  *time.Time `momo:"column:deleted_at,soft_delete:timestamp"`
	Embedding  vector.Vector `momo:"dim:1536"`                      // pgvector column

	Posts []Post `momo:"has_many,fk:AccountID"`
}
```

Nullability and column rules:

- A non-pointer Go field is `NOT NULL`. A pointer (`*string`), a `sql.Null*`
  type, or the `nullable` tag makes the column nullable. `notnull` is redundant
  for non-pointers and a diagnostic on pointers.
- A nullable field generates `momo.NullColumn[T]` (has `SetNull()`); a
  non-nullable one generates `momo.Column[T]` (`SetNull()` does not compile).
- A primary key must not be nullable.
- Scanning NULL into a non-pointer field is an error. Use pointers for nullable
  columns.
- Go type to column type: see [TYPE_MAPPING.md](TYPE_MAPPING.md) (`int` is
  `bigint`, `string` is `text`, `string` with `size:N` is `varchar(N)`,
  `time.Time` is `timestamptz`, `types.UUID` is `uuid`, `types.JSON[T]` is
  `jsonb`). `uint64`, maps and unknown types have no mapping: add `type:<sql>`
  or a codec.
- Column names default to the naming preset (`CreatedAt` becomes `created_at`).
  Fields are exported only; a tag on an unexported field is a diagnostic. Skip a
  field with `momo:"-"`.

Tag reference (all lower case):

| Tag                                                                       | Meaning                                                                                                                                      |
| ------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `table:<name>` (on `momo.Model`)                                          | table name; always set it. Also `schema:<name>`.                                                                                             |
| `pk`                                                                      | primary key; several `pk` fields make a composite key (relations by tag need a single column)                                                |
| `identity`                                                                | database-generated identity (`GENERATED BY DEFAULT AS IDENTITY`)                                                                             |
| `column:<name>`                                                           | column name                                                                                                                                  |
| `size:<n>`                                                                | `varchar(n)`; strings only                                                                                                                   |
| `precision:<p>`, `scale:<s>`                                              | numeric precision (float/decimal only); `scale` needs `precision`                                                                            |
| `dim:<n>`                                                                 | vector dimension; required for vector fields, invalid elsewhere                                                                              |
| `default:<sql>`                                                           | column default, raw SQL (`default:now()`, `default:'free'`, `default:0`)                                                                     |
| `unique` / `unique:<name>`                                                | unique index (single column)                                                                                                                 |
| `index` / `index:<name>`                                                  | index; modifiers `sort:asc\|desc`, `nulls:first\|last`, `method:<m>` need `index` or `unique`                                                |
| `version`                                                                 | optimistic-lock column (integer type)                                                                                                        |
| `created_at[:unix\|unixmilli\|unixnano]`, `updated_at`                    | automatic timestamps; field type `time.Time`, `*time.Time`, `int64` or `*int64`                                                              |
| `soft_delete:timestamp\|unix\|bool`                                       | soft-delete marker; Go type `*time.Time`, `*int64` or `bool`                                                                                 |
| `read_only`, `create_only`, `immutable`                                   | write permissions: never written; written on insert only; never updated                                                                      |
| `sensitive`                                                               | excluded from the generated `ColumnByName` allowlist (use for secrets)                                                                       |
| `type:<sql>`, `collate:<c>`, `check:<expr>`, `comment:<t>`, `enum:<name>` | raw column type, collation, check, comment, enum type                                                                                        |
| `embedded`, `prefix:<p>`                                                  | flatten a named struct-typed field into this table (anonymous embedded structs flatten by themselves); `prefix` prepends to its column names |
| `nullable`, `notnull`                                                     | explicit nullability (rarely needed)                                                                                                         |

`MomoConfig` for what tags cannot express: composite or partial indexes, vector
indexes, checks, enums, views, triggers, policies. The generator reads this
method statically and never runs it; arguments are `&receiver.Field` pointers and
string literals only.

```go
func (a *Account) MomoConfig(c *momo.Config) {
	c.Unique("accounts_email_plan_key", &a.Email, &a.Plan)
	c.Index("accounts_plan_live_idx", &a.Plan).Where("deleted_at IS NULL")
	c.Index("accounts_embedding_hnsw", &a.Embedding).
		Method("hnsw").
		Opclass(&a.Embedding, "vector_cosine_ops").
		With("m", "16").With("ef_construction", "64")
	c.Check("accounts_balance_nonneg", "balance >= 0")
}
```

An index name declared in `MomoConfig` must not collide with an auto-named
`<table>_<col>_idx` produced by an `index` tag (the `Plan` field above already
yields `accounts_plan_idx`); the generator refuses the duplicate.

Config methods: `Index`, `IndexExpr`, `Unique`, `Check`, `Enum`, `Default`,
`Comment`, `Collate`, `View`, `Function`, `Trigger`, `Policy`, `RLS`, `Partition`,
`PartitionOf`. Index chain: `Where(pred)`, `Method(m)`, `With(k, v)`,
`Opclass(field, op)`, `Desc(field)`, `Nulls(field, "first"|"last")`,
`Concurrently()` (`diff` writes the index as `CREATE INDEX CONCURRENTLY` in its own no-transaction migration).

Many-to-many needs `join:` with the same table on both models you declare it on
(the reverse side is optional, but if declared it must carry `join:` too):

```go
type Document struct {
	momo.Model `momo:"table:documents"`
	ID         int64 `momo:"pk,identity"`
	Title      string
	Tags       []Tag `momo:"many2many,join:taggings"` // join table taggings(document_id, tag_id)
}

type Tag struct {
	momo.Model `momo:"table:tags"`
	ID         int64 `momo:"pk,identity"`
	Name       string
	Documents  []Document `momo:"many2many,join:taggings"` // optional reverse side, same join table
}
```

Relation tags (a relation is an exported field of struct, slice or pointer type;
it is not a column):

| Tag                      | Field type | FK lives on | Notes                                                                                                               |
| ------------------------ | ---------- | ----------- | ------------------------------------------------------------------------------------------------------------------- |
| `belongs_to,fk:<Field>`  | `*Target`  | this struct | `fk` names the FK field on THIS struct; make it a pointer if the relation is optional                               |
| `has_one,fk:<Field>`     | `*Target`  | the target  | `fk` names the field on the TARGET                                                                                  |
| `has_many,fk:<Field>`    | `[]Target` | the target  | `fk` names the field on the TARGET                                                                                  |
| `many2many,join:<table>` | `[]Target` | join table  | set `join:` (same table) on every side you declare; the reverse side is optional; two key columns `<model>_<pkcol>` |

Extra relation options: `polytype:<column>` + `polyvalue:<value>` (both, on
`has_one`/`has_many` only: polymorphic), `on_delete:` / `on_update:`
(`cascade`, `restrict`, `set_null`, `set_default`, `no_action`; they affect only
the FK emitted for `belongs_to`), `no_constraint` (emit no FK). `has_one` and
`has_many` create no FK constraint themselves; the `belongs_to` on the other
side does. Many-to-many join tables get a composite primary key plus two FKs with
`ON DELETE CASCADE`.

Generator diagnostics (nothing is written for the package until fixed): more
than one relation kind on a field; field type does not match the kind; target is
not a Momo model; `fk` missing or naming a missing field; `many2many` without
`join:`, or a declared reverse side without `join:`; `polytype` without `polyvalue` (or the
reverse), or on a kind other than `has_one`/`has_many`; `polytype` column not a
field of the target; many-to-many to the model's own type; `momo.Model` embedded
by pointer; a generic model; `size` on a non-string; `dim` on a non-vector;
vector field without `dim`; `scale` without `precision`; `notnull` on a pointer;
nullable primary key; unknown tag option; Go type with no SQL mapping.

What the generator emits per model `Task` (package `task`):

- `task.Table` (a `momo.TableRef`), one column variable per field (`task.ID`,
  `task.Title`), `task.As("t")` for aliased columns, `task.ColumnByName(name)`
  and `task.ColumnNames()` (an allowlist for user-supplied sort/filter names).
- One `momo.Relation[Parent, Child]` per relation field (`document.Comments`,
  `document.Author`).
- Vector fields become `momo.VectorColumn[vector.Vector]` (nullable:
  `NullVectorColumn`).
- `read_only` / `create_only` / `immutable` fields become permission-typed
  columns that have no `Set` (or insert-only `Set`).
- Package name = lowercase model name. It can collide with your local variable
  names (`task := ...` shadows the package). Name variables differently or
  alias the import.

### Projections

A projection reads a column subset or aggregate into a non-table struct. Embed
`momo.Projection` by value; `momo generate` emits its `MomoMeta` into
`momo_gen.go` (every exported field, in declaration order; column name from the
naming rule or a `momo:"column:..."` tag). NEVER hand-write `MomoMeta` for a
projection. The `Columns(...)` list of the query must equal the struct field
order.

```go
type Card struct {
	momo.Projection
	ID    int64
	Title string
	Score int64
}
```

Use it like this:

```go
cards := momo.Select[models.Card](q).From(article.Table).
	Columns(article.ID, article.Title, article.Score).
	OrderBy(article.ID.Asc()).
	All(ctx)
```

When `From` is a plain generated table (`article.Table`) with a soft-delete
column (timestamp, unix or bool), the live-row filter is added automatically,
exactly as for a model; `WithDeleted()` and `OnlyDeleted()` work too. Do NOT add
`Where(<table>.DeletedAt.IsNull())` yourself. A `Join` does not change this:
the `From` table is filtered and joined tables are not. To alias use
`article.Table.As("a")`, which keeps the filter. The filter is NOT added when
`From` is a subquery or CTE, or a hand-built `momo.TableRef{Name:..., Alias:...}`
(it has no soft-delete column): filter those yourself.

If a package still has a hand-written projection `MomoMeta` from before the
generator emitted it, delete it (method and `Meta` variable); the generated one
redeclares it.

A projection cannot be inserted, updated or deleted (it embeds `Projection`, not
`Model`). Computed columns (a distance, `momo.CountAll()`) are valid entries in
`Columns`. The select list is positional and not aliased: the `Name` you give a
computed column in `Columns` of the meta (for example `distance`) is only a label.

Examples: [projection-pagination](../examples/projection-pagination/),
[vector-search](../examples/vector-search/) (`Hit`).

Details: [TYPE_MAPPING.md](TYPE_MAPPING.md), [relations.md](relations.md) section 1.
Example: [relations](../examples/relations/models/models.go).

## 5. Migrations workflow

Flow: edit models, `momo generate`, `momo migrate diff <name>`, REVIEW the SQL,
commit, `momo migrate apply`.

```sh
momo generate
momo migrate diff add_plan              # offline; writes 3 files into migrations/
momo migrate diff init                  # first run on an empty dir: the initial schema
momo migrate plan                       # list pending (needs DB)
export DATABASE_URL='postgres://user:pass@host:5432/app?sslmode=verify-full'
momo migrate apply                      # [--to N] [--lock-wait 30s]
momo migrate status                     # applied | pending | dirty | mismatch | missing
momo migrate drift                      # live DB vs latest applied snapshot
```

- `diff` writes `<version>_<name>.up.sql`, `<version>_<name>.snapshot.json` and
  rewrites `momo.sum`. Commit all three. `diff` never overwrites; the name must
  match `^[a-z][a-z0-9_]*$`; the version is a 14-digit UTC timestamp.
- `diff` compares models with the latest snapshot, never with the database.
  Prints `no changes` and writes nothing when equal.
- Never edit an applied migration file, even cosmetically: `apply` refuses on the
  changed checksum. For a cosmetic edit you accept: `momo migrate hash`, then
  `momo migrate rehash --version V --accept` (without `--accept` it only shows
  the two checksums); see [migrations.md](migrations.md) section 4. Prefer a new
  migration.
- Renames are never guessed: a rename looks like drop plus add and is refused.
  Say it explicitly: `--rename-column table.old=new`, `--rename-table old=new`.
- Destructive changes (`drop_table`, `drop_column`, `drop_enum`, a column type
  change) are refused without `--allow-destructive`. Type changes rewrite the
  table under `ACCESS EXCLUSIVE` and fail on rows that do not cast.
- A `NOT NULL` column without a default on a table that exists in the snapshot is
  refused (`ErrStagedRequired`). Give it a `default:`, or stage it: `diff --stage`,
  apply, backfill, check `SELECT count(*) FROM t WHERE c IS NULL` is 0, then
  `diff --enforce-not-null`. The flag enforces every nullable column the model
  has as NOT NULL, not only staged ones; with NULLs left, `apply` fails (exit 3)
  and rolls back cleanly (not dirty): backfill and `apply` again.
- URL: `--url` (warns: visible in the process list; `--url -` reads stdin), else
  `migrate.url_env.postgres`, else `MOMO_DATABASE_URL`.
  Exit codes: 0 ok, 1 findings, 2 usage/config, 3 database/lock/execution error.
  An invalid migration name (`diff Bad-Name`) exits 3, not 2.
- Hand-written SQL migration: create `<version>_<name>.up.sql` (no
  `-- momo:generated` line) plus a snapshot file copied from the previous one
  with `version` and `parent` changed; its schema must equal its parent's. Then
  `momo migrate hash`, `momo migrate validate`. Directives (first lines):
  `-- momo:no-transaction`, `-- momo:split`, `-- momo:destructive`
  (informational). Unknown `-- momo:` directive is an error.
- Existing database: write models, `momo migrate diff init` (do not apply), then
  `momo migrate baseline --version <init version>`; the live schema must match
  the snapshot (extra objects: add globs to `migrate.exclude`). Example:
  [adopt-existing-schema](../examples/adopt-existing-schema/).
- The first migration with a vector column runs
  `CREATE EXTENSION IF NOT EXISTS "vector"`. pgvector must be installed on the server (image
  `pgvector/pgvector`), and the migration role must be allowed to create
  extensions, otherwise apply exits 3 naming the extension.

CI (every change):

```sh
go install github.com/azrtydxb/momo/cmd/momo@v0.1.0
momo generate --check                                   # generated code is current
momo migrate validate                                   # files vs momo.sum, version chain, forks, snapshots
momo migrate diff ci_probe | grep -qx 'no changes'      # models have a migration
```

The last line is derived from the code, not covered by a test: when models
changed, `diff` writes files (exit 0) and `grep` fails the job; discard the files
(`git checkout -- migrations && git clean -fd migrations`). There is no
`diff --check`. `validate` already verifies `momo.sum`; do not run
`momo migrate hash` in CI.

Production checklist:

1. `momo migrate validate` and `momo migrate plan` before apply; read the SQL.
2. One `apply` job per deploy, over a DIRECT connection (not PgBouncer in
   transaction mode: the lock is a session advisory lock).
3. Migrate first, then roll out code; every migration must be backward
   compatible with the running code (add, backfill, switch, drop later; a rename
   is add, copy, switch, drop).
4. Back up before any destructive, type-change or no-transaction migration.
5. Use a migration role without `statement_timeout`; `migrate.lock_timeout`
   defaults to 5 s and fails fast on a blocked DDL (safe to retry).
6. After: `momo migrate status` (exit 0) and `momo migrate drift`.
7. Failed transactional migration: nothing persists; fix and re-run. Failed
   no-transaction migration leaves partial state and a dirty row: finish or undo
   by hand in `psql`, then `momo migrate resolve --version V --applied` or
   `--rolled-back`. Full procedure: [migrations.md](migrations.md) section 4.

Known issues in v0.1.0, as rules (each is a filed bug):

- For large tables declare `Concurrently()` on the index in `MomoConfig`: `diff`
  writes `CREATE INDEX CONCURRENTLY` in its own `-- momo:no-transaction`
  migration (a trailing `<name>_no_tx` file when the diff also has other DDL).
  Without it the index is a plain `CREATE INDEX` that blocks writes.
  A failed concurrent build leaves an INVALID index: `drift` reports it; drop it with
  `DROP INDEX CONCURRENTLY IF EXISTS`, then `resolve --rolled-back` and re-apply.
- `-- momo:no-transaction` applies to the WHOLE file: every statement loses
  atomicity and a mid-file failure leaves earlier statements committed (dirty).
  `diff` never mixes modes: it splits concurrent indexes, `ADD UNIQUE` and
  `VALIDATE CONSTRAINT` into a trailing no-transaction migration and keeps the
  transactional DDL in the first file. Keep hand-written no-transaction files
  to one statement.
- `-- momo:destructive` is informational: `diff` writes it, `apply` does not
  read it, so a hand-written `DROP` applies without confirmation. The only
  guard is `diff --allow-destructive`. Review every `.up.sql` like code.
- `diff` prints `risky:` hints for dropping an index, primary key, unique, check
  or foreign key and for adding `UNIQUE`/`CHECK`; these need no flag.
  `ADD CHECK` is emitted `NOT VALID` with a later `VALIDATE CONSTRAINT`, and
  `ADD UNIQUE` as `CREATE UNIQUE INDEX CONCURRENTLY` plus `USING INDEX`. Both
  can still fail on existing violating data. Hand-written changes are not flagged.
- NEVER edit an applied migration. `apply` and `status` refuse
  (`ErrChecksumMismatch`) and `momo migrate hash` alone does NOT fix it (the
  history row keeps the old checksum). Preferred: `git checkout -- <file>` and
  write a new migration. A cosmetic edit you accept: `hash`, then
  `momo migrate rehash --version V --accept`. Editing a still-pending migration
  is fine; then run `hash`. `hash` accepts every edit, so read its
  `changed:/added:/removed:` list first.
- `apply` refuses a forked snapshot chain (same error as `validate`) and a pending
  version older than the latest applied one (`ErrOutOfOrder`); both exit 1, as
  does a broken chain (`ErrChainInvalid`). There is no `rebase` command: delete
  the late migration's `.up.sql` and `.snapshot.json`, run
  `momo migrate diff <name>` to regenerate it with a new version, then
  `momo migrate hash` (a pending file needs no `rehash`); see [migrations.md](migrations.md) section 2. Still
  run `validate` in CI.
- There is no rollback and no down migrations. Undo with a new forward
  migration. `resolve --rolled-back` runs no SQL.
- There is no `--force-unlock` (an unknown flag, exit 2). The migration lock is a
  session advisory lock, freed when its holder disconnects. A stuck lock: find
  the session in `pg_stat_activity` and `pg_terminate_backend` it.
- `migrate.lock_timeout: 0s` and `statement_timeout: 0s` disable the timeouts
  (`SET ... = 0`); leave the key out for the defaults (5 s, untouched).
- `apply` with nothing pending writes nothing, not even the history table.
- `--url` is visible in the process list (momo warns): use `migrate.url_env`,
  `MOMO_DATABASE_URL` or `--url -`. URLs in history `error` text are redacted.
- `momo_migrations` lives in the first schema of `search_path`;
  `migrate.schemas[0]` must equal `current_schema()` or the engine errors.
- Dropped extensions and schemas are not dropped in the database.
- `AutoMigrate` (Go API) is additive only, writes no history; DON'T mix it with
  versioned migrations on one schema.

Details: [migrations.md](migrations.md). Examples:
[upgrade-database](../examples/upgrade-database/),
[custom-sql-migration](../examples/custom-sql-migration/),
[adopt-existing-schema](../examples/adopt-existing-schema/).

## 6. Queries cheat sheet

All builders take a `momo.Querier` (`*momo.DB` or `*momo.Tx`), so write helpers
that accept `momo.Querier` and call them inside or outside a transaction. Builders
are immutable values: every method returns a copy. Values are always bound
(`$1`...); identifiers come from generated columns. Errors from building (bad
column, wrong shape) surface at `SQL()` or at the terminal call.

Select:

```go
tasks, err := momo.Select[models.Task](q).
	Where(task.Done.Eq(false), task.Priority.Ge(2)). // several predicates are ANDed
	OrderBy(task.Priority.Desc(), task.ID.Asc()).
	Limit(20).Offset(0).
	All(ctx) // []T; an empty result is a nil slice and nil error, never ErrNotFound

one, err := momo.Select[models.Task](q).Where(task.ID.Eq(id)).One(ctx)   // ErrNotFound / ErrTooManyRows
first, err := momo.Select[models.Task](q).OrderBy(task.ID.Asc()).First(ctx) // ErrNotFound when empty
n, err := momo.Select[models.Task](q).Where(task.Done.Eq(true)).Count(ctx)  // int64; ignores order and After
ok, err := momo.Select[models.Task](q).Where(task.Title.Eq("x")).Exists(ctx)
```

Predicates on columns:
`Eq Ne Lt Le Gt Ge In NotIn Between IsNull IsNotNull IsDistinctFrom IsNotDistinctFrom InQuery NotInQuery`
and `*Expr` variants;
combinators `momo.And`, `momo.Or`, `momo.Not`, `momo.Like`, `momo.ILike`,
`momo.ContainsLiteral`, `momo.Exists(subquery)`, `momo.NotExists`.
`momo.EscapeLike(s)` escapes user text for `Like`. Find NULLs with `IsNull()`;
`Eq(nil)` is not expressible. Other builder methods: `Distinct`, `Join`,
`LeftJoin`, `CrossJoin`, `GroupBy`, `Having`, `ForUpdate(opt...)`
(`"NOWAIT"`, `"SKIP LOCKED"`), `ForShare`, `With`, `WithRecursive`, `Union`,
`FromSubquery`, `Each(ctx, fn)`, `Iter(ctx)` (`iter.Seq2[T, error]`).
Aggregates: `momo.CountAll()`, `CountOf`, `CountDistinct`, `Sum`, `Avg`, `Min`,
`Max`.

- DO order every paginated or `First` query; without `OrderBy` row order is
  unspecified.
- `Each` and `Iter` stream rows and return `ErrUnsupported` when the query has
  `Preload`.
- `IndexedBy` is `ErrUnsupported` on PostgreSQL. Don't use it.

Keyset pagination (do not use `Offset` for deep pages):

```go
var size int64 = 20 // Limit takes int64
q := momo.Select[models.Task](db).OrderBy(task.Priority.Desc()) // PK is appended as tie-breaker
if cur != nil {
	q = q.After(cur.Priority, cur.ID) // ONE value per effective ORDER BY key, PK included
}
page, err := q.Limit(size + 1).All(ctx) // fetch size+1 to learn whether another page exists
```

`After` needs `OrderBy`, a value count equal to the number of effective keys
(`ErrValidation` otherwise) and NON-nullable keys (`ErrUnsupported` otherwise).
For a projection (no PK in its metadata) list the unique key in `OrderBy`
yourself. A keyset order that is not total skips or repeats rows. Example:
[projection-pagination](../examples/projection-pagination/).

Insert:

```go
t := models.Task{Title: "write guide"} // Priority 0, Done false, Note NULL
// Model writes EVERY column except identity, read_only and Omit()ted ones; zero values are written.
_, err := momo.Insert[models.Task](q).Model(&t).Exec(ctx)
// Values writes ONLY the named columns; the rest take the database default.
_, err = momo.Insert[models.Task](q).Values(task.Title.Set("x")).Exec(ctx)
// Many rows, one statement (chunked atomically under the 65535 bind limit):
_, err = momo.Insert[models.Task](q).Models(rows).Exec(ctx)
// Read database-generated values (identity, defaults) back:
rows2, err := momo.Insert[models.Task](q).Model(&t).Returning(task.ID).Rows(ctx)
if err != nil { return err }
defer func() { _ = rows2.Close() }()
for rows2.Next() { if err := rows2.ScanModel(&t); err != nil { return err } }
return rows2.Err()
```

- DO choose `Model` when the struct is the truth, `Values` when "unset" must mean
  "database default". A zero `Priority` with `Model` is stored as 0, NOT as the
  column default.
- `Exec` does not fill the identity into your struct; use `Returning(...).Rows`.
  `ReturningAll()` returns every mapped column. `CreateGraph` reads identities
  back itself.
- A `ReadOnly` / `identity` column is never written by `Insert.Model`.
- `Insert.Models([]T{})` (empty) is `ErrValidation`.

Update:

```go
// Set writes exactly the given values, including 0, false, "" ; other columns untouched.
res, err := momo.Update[models.Task](q).
	Set(task.Priority.Set(0), task.Note.SetNull(), task.Title.SetExpr(momo.TypedSQL[string]("upper(title)"))).
	Where(task.ID.Eq(id)).
	Exec(ctx) // res.RowsAffected
// Atomic arithmetic (no read-modify-write race):
//   .Set(account.Balance.SetExpr(momo.TypedSQL[int64]("balance + ?", delta)))
// Other assignments: col.SetDefault(), col.SetExcluded() (upsert), nullCol.SetNull().

// Whole row from a struct (writes every writable column, zero values included):
cur, _ := momo.Select[models.Task](q).Where(task.ID.Eq(id)).One(ctx)
cur.Done = true
_, err = momo.Update[models.Task](q).Model(&cur).Exec(ctx)
```

- An `Update` or `Delete` without `Where` fails with `ErrUnfilteredMutation`
  unless you call `.AllRows()`. DON'T add `AllRows()` to silence it; add the
  filter.
- `Update.Set` with zero rows matched is NOT an error; check `res.RowsAffected`.
- `Update.Model(&m)` identifies the row by primary key. Without a version
  column a missing row is `ErrNotFound`. With a `version` column the statement
  is `WHERE pk = ? AND version = ?` and `SET version = version + 1`: a stale copy
  fails with `ErrOptimisticLock`, a missing row with `ErrNotFound`, and after
  success your struct's version field is advanced. Reload the row and retry on
  `ErrOptimisticLock`.
- Only `Update.Model` checks and advances the version. A `Set`-based update,
  `Upsert` and relation helpers do NOT. DON'T `Set` the version, pk, soft-delete,
  created_at or updated_at column directly (`ErrValidation`); writing a
  `read_only`, `immutable` (on update) or identity column is `ErrValidation`.
  `Rows` is not supported on a versioned `Model` update; use `Exec`.
- `updated_at` is set automatically on `Update` (Set or Model), not on raw SQL
  and not on relation helpers.

Delete and soft delete:

```go
_, err := momo.Delete[models.Task](q).Where(task.ID.Eq(id)).Exec(ctx)   // hard delete
_, err = momo.SoftDelete[models.Task](q).Where(task.ID.Eq(id)).Exec(ctx) // stamps the soft_delete column
_, err = momo.ForceDelete[models.Task](q).Where(task.ID.Eq(id)).Exec(ctx) // hard delete on a soft-delete model

live, _ := momo.Select[models.Task](q).All(ctx)                  // soft-deleted rows hidden by default
all, _ := momo.Select[models.Task](q).WithDeleted().All(ctx)
dead, _ := momo.Select[models.Task](q).OnlyDeleted().All(ctx)
```

- On a model with a soft-delete column plain `Delete` is refused with
  `ErrValidation`. Use `SoftDelete` or `ForceDelete`.
- The soft-delete filter applies automatically to `Select`, `Count`, relation
  loads and `Has`/`HasNo` of a model. It does NOT apply to raw SQL (`momo.SQL`,
  `momo.Exec`, `momo.Scalars`), or to a projection whose `From` is a subquery or
  a hand-built `TableRef` (see "Projections" in section 4): add
  `deleted_at IS NULL` / `<table>.DeletedAt.IsNull()` yourself. A projection
  read `From` a generated table (also with joins) is filtered automatically.
- A soft-deleted row still occupies its unique keys. `FindOrCreate` does not
  treat it as found: with a unique constraint that covers deleted rows the
  insert conflicts and it returns `ErrUniqueViolation` (it never returns or
  restores the deleted row). A partial unique index would release the key, but
  `FindOrCreate` cannot target one (its `ON CONFLICT (key)` has no predicate;
  use `Insert` + `OnConflict(key).Where(...)`). Use partial unique indexes
  (`Where("deleted_at IS NULL")`) if
  keys must be reusable.
- `Delete.Model(&m)` deletes by primary key (`ErrNotFound` if missing).

Upsert (PostgreSQL `ON CONFLICT`):

```go
// Conflict target = primary key; DO UPDATE assigns every writable column from EXCLUDED.
_, err := momo.Upsert[models.Task](q).Model(&t).Exec(ctx)
// Any unique key, explicit behavior:
_, err = momo.Insert[models.Task](q).Model(&t).
	OnConflict(task.Title).               // needs a UNIQUE constraint/index on exactly these columns
	DoUpdate(task.Priority.SetExcluded()).
	Exec(ctx)
_, err = momo.Insert[models.Task](q).Model(&t).OnConflict(task.Title).DoNothing().Exec(ctx)
err = momo.UpsertByPK(ctx, q, &t) // one-call form of Upsert(q).Model(m); reads generated columns back
// Conditional upsert (lease/lock): update only when the condition holds.
res, err := momo.Insert[models.Lock](q).
	Values(lock.Name.Set("n"), lock.Holder.Set(me), lock.Until.Set(until)).
	OnConflict(lock.Name).
	// .Where(targetPreds...) here names a partial unique index as the target
	DoUpdate(lock.Holder.SetExcluded(), lock.Until.SetExcluded()).
	Where(momo.Or(lock.Until.Le(now), lock.Holder.Eq(me))). // columns = existing row
	Exec(ctx)                                               // res.RowsAffected == 1: you hold it; 0: refused
// Compare with the proposed row: lock.Until.LtExpr(lock.Until.Excluded())
```

Insert from a SELECT (`INSERT ... SELECT`), including a FROM-less select guarded by
`WHERE NOT EXISTS`, as one atomic statement:

```go
// INSERT INTO meters (product, id, kind, ...) SELECT $1, $2, 'distinct', ...
//   WHERE NOT EXISTS (SELECT 1 FROM meters WHERE product = $7 AND legacy)
//   ON CONFLICT (product, id) DO NOTHING
row := momo.SelectExprs(momo.Value(product), momo.Value(id), momo.Value("distinct"), momo.Value(now)).
	Where(momo.NotExists(
		momo.SelectExprs(momo.TypedSQL[int]("1")).From(meterTable). // constant 1, not a bound value
			Where(meter.Product.Eq(product), meter.Legacy.Eq(true))))
res, err := momo.Insert[models.Meter](q).
	FromSelect(row, meter.Product, meter.ID, meter.Kind, meter.CreatedAt). // explicit typed target columns
	OnConflict(meter.Product, meter.ID).DoNothing().
	Exec(ctx) // res.RowsAffected: 1 inserted, 0 skipped
```

- `FromSelect` takes any `Subquery` (the first argument) plus the typed target columns:
  `SelectExprs` (a FROM-less or single-source select of expressions, with `From` and
  `Where`) or a `Select[T]` query. The select list must have as many columns as the
  target columns (`ErrValidation` otherwise; a `RawSubquery` is checked by the
  database). `FromSelect` runs no model hooks.
- The soft-delete filter on a `SelectExprs` source (including inside `NotExists`) is
  added only when `From` gets a `TableRef` that carries the soft-delete marker.
  Generated `Table` refs do; a hand-built `TableRef{Name: ...}` does not and then
  counts soft-deleted rows, so set its `SoftDelete` and `SoftDeleteKind`.
- Only the listed columns are written. Nothing is injected: no automatic
  created_at/updated_at, no Go-side defaults, no field validators. List those columns
  and select their values. Identity, read-only and non-writable columns, unmapped or
  repeated columns, and mixing with `Model`/`Values`/`Omit` are `ErrValidation`.
- Composes with `OnConflict` (`DoNothing`, `DoUpdate`, both `Where`s), `Returning`,
  `ReturningAll`, `Rows` and `Upsert[T]` (default DO UPDATE set = the listed
  columns). Placeholders number across the select first, then ON CONFLICT (`$n` on
  PostgreSQL, `?` on SQLite/MySQL). For the constant in `EXISTS (SELECT 1 ...)` use
  `momo.TypedSQL` with the SQL text `1`, as in the example: PostgreSQL cannot infer
  the type of a bound `Value` there.
- MySQL has no `ON CONFLICT` (`ErrUnsupported`) but a plain `FromSelect` works; a FROM-less select with a
  `WHERE` renders `FROM DUAL`.

- Upsert never overwrites pk, version, soft-delete or created_at columns, and does
  not advance or check the version (no optimistic-lock detection).
- `InsertQuery.Where` is the DO UPDATE condition: valid only after `DoUpdate` or on
  `Upsert[T]` (keeps its default set); after `DoNothing` or with no conflict clause it
  is `ErrValidation`. Columns render qualified by the table name (existing row);
  `col.Excluded()` is `EXCLUDED.col`, used with `LtExpr`/`LeExpr`/`EqExpr`. A false
  condition affects 0 rows and, with `Returning`, returns no row.
- `OnConflict(cols...).Where(preds...)` renders `ON CONFLICT (cols) WHERE ...` for a
  partial unique index (unqualified columns); PostgreSQL rejects the statement
  without it. It needs target columns (`ErrValidation` otherwise). Multiple `Where`
  calls AND. MySQL: both are `ErrUnsupported`. `Excluded()` in the target
  predicate is `ErrValidation` (use it in the `DoUpdate` `Where`). On SQLite the
  target predicate renders bool, integer, finite float and string values inline
  (`"live" = 1`), because SQLite matches it against the index text; a value
  that cannot be inlined (time, bytes, pointer, NULL, NaN, a string with a NUL
  byte) is `ErrValidation`, and a type with `Value()` is inlined as that value.
  This is golden-tested but not yet run against a real SQLite; `momo.SQL("live = 1")`
  is the proven fallback. Write the literals in the index exactly as Momo renders them: integers as `2`, floats in plain decimal with `.0` for whole numbers (`2.0`) for magnitudes from 1e-6 up to 1e21, and exponent form (`1e-07`, `1e+21`) outside that range; otherwise use `momo.SQL`. The values appear in the SQL text, so they show up in
  logs and traces.
- Mixing zero and non-zero identity values in one `Models` upsert is `ErrValidation`.
  Explicit identity values do not advance the sequence: `setval` it yourself.

Raw SQL (escape hatches; the SQL text must be a constant, pass user input only
as arguments):

```go
// Predicate fragment:           ?  binds the next argument, ?? is a literal ?, @name binds momo.Named("name", v)
momo.Select[models.Task](q).Where(momo.SQL(`lower("tasks"."title") LIKE ?`, "%go%"))
// Typed expression for Set/Columns/OrderBy:
momo.TypedSQL[int64]("balance + ?", delta)
// Statements and scalar lists:
affected, err := momo.Exec(ctx, q, `UPDATE tasks SET done = ? WHERE id IN (?)`, true, momo.In(ids))
ids, err := momo.Scalars[int64](ctx, q, `SELECT id FROM tasks WHERE done = ?`, true)
// Helpers: momo.In(slice) (empty slice is a build error), momo.Ident("col"), momo.RawSubquery(sql, args...)
```

- DON'T build SQL text with `fmt.Sprintf` from user input. Use `?`, `In`, `Ident`.
- QUALIFY column names in raw fragments inside joined or preloaded queries
  (`"tasks"."title"`): Momo qualifies its own columns, and an unqualified name
  becomes ambiguous with a joined table. A column qualified by a table that is not
  in the statement's FROM/JOIN is `ErrValidation` at build time.
- On PostgreSQL write `? |` with a space for a bind followed by OR; `?|` and `?&`
  are operators. Do not mix `?`/`@name` with native `$n` in one fragment.
- Returned results of raw queries bypass soft-delete, hooks and timestamps.

`SelectQuery.SQL()` returns `Compiled{SQL, Args}` without running. `Explain(ctx)`
runs `EXPLAIN` (never `ANALYZE`) and returns plan text; it binds parameters and
does not execute the query. Inside a `Tx`, it sees that transaction's settings.

Select-time guards: `Limit`/`Offset` negative is `ErrValidation`.

Details: README sections 5-7, [capabilities.md](capabilities.md).
Examples: [crud-zero-null](../examples/crud-zero-null/),
[projection-pagination](../examples/projection-pagination/).

## 7. Transactions

```go
err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
	// use tx (a momo.Querier), NOT db, for everything in here
	if _, err := momo.Update[models.Account](tx).
		Set(account.Balance.SetExpr(momo.TypedSQL[int64]("balance + ?", amt))).
		Where(account.Owner.Eq(to)).Exec(ctx); err != nil {
		return err // any error (or panic) rolls back
	}
	return nil // nil commits
})
```

- `db.Tx(ctx, opts, fn)` commits on nil, rolls back on error or panic. `fn` runs
  exactly once; Momo never retries it.
- DON'T use `db` inside the closure: it runs outside the transaction on another
  connection and can deadlock on a small pool.
- `TxOptions{ReadOnly: true}` makes the server refuse writes (SQLSTATE 25006;
  Momo has no sentinel for it: `errors.As(err, &pgErr)` with
  `*pgconn.PgError`). `TxOptions{Isolation: sql.LevelSerializable}` sets the level;
  supported: read uncommitted, read committed, repeatable read, serializable. Any
  other level is `ErrUnsupported`.
- Savepoints: `tx.Tx(ctx, func(sp *momo.Tx) error {...})` runs in a savepoint; if
  it fails only its work is undone and the outer transaction continues. Without a
  savepoint PostgreSQL aborts the whole transaction at the first error. Manual:
  `tx.Savepoint(ctx, name)`, `tx.RollbackTo(ctx, name)`, `tx.Release(ctx, name)`.
- Manual control: `tx, err := db.Begin(ctx, opts)`, `tx.Commit(ctx)`,
  `tx.Rollback(ctx)`; a finished `*Tx` returns `ErrTxDone`. Prefer `db.Tx`.
- Incremental adoption in a `database/sql` or pgx codebase, both directions:
  - Adopt a transaction the caller began: `mtx := db.AdoptTx(pgxdriver.SQLTx(sqlTx))`
    for a `*sql.Tx` (database must be pgx-stdlib backed, `FromSQLDB`), or
    `db.AdoptTx(pgxdriver.PgxTx(pgxTx, opts...))` for a `pgx.Tx` (pgxpool driver; `opts` are the options of `New`, such as `WithExecMode`; an unsupported exec mode returns `ErrUnsupported` on use, as with `New`). An adopted `pgx.Tx` keeps Batch and `COPY`; an adopted `*sql.Tx` has neither. Statements,
    hooks, error classification and savepoints (`mtx.Tx`) work as usual. `Commit`
    and `Rollback` on `mtx` return `ErrAdoptedTx` and do nothing: the caller
    commits or rolls back its own transaction. After that, `mtx` statements
    return `ErrTxDone`. A nil argument panics.
  - Expose the handle behind a `*Tx` to legacy helpers:
    `pgxdriver.UnwrapSQLTx(tx)` returns `(*sql.Tx, bool)` and
    `pgxdriver.UnwrapPgxTx(tx)` returns `(pgx.Tx, bool)`; `false` when the `Tx`
    belongs to the other driver kind. Works for `db.Tx` callbacks, adopted
    transactions and savepoint scopes (the outer transaction). Finish a
    momo-begun transaction through momo, never through the unwrapped handle.
  - Mixing is safe: a legacy `SELECT ... FOR UPDATE` helper and momo statements
    can alternate on the same transaction.
- Do not call blocking network I/O inside `fn`.
- Retry the WHOLE `db.Tx` on `ErrSerialization` (PostgreSQL 40001 serialization
  failure and 40P01 deadlock). Only when `fn` has no side effects outside the
  database. Copy of `examples/transactions`:

```go
func retrySerializable(ctx context.Context, db *momo.DB, fn func(tx *momo.Tx) error) error {
	const (
		maxAttempts = 3
		backoff     = 20 * time.Millisecond // doubles in a real service
	)
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = db.Tx(ctx, momo.TxOptions{Isolation: sql.LevelSerializable}, fn)
		if !errors.Is(err, momo.ErrSerialization) {
			return err
		}
		if attempt == maxAttempts {
			break // out of attempts: return the error without sleeping
		}
		d := backoff/2 + rand.N(backoff) // math/rand/v2: jitter so competitors do not collide in lockstep
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	return err
}
```

- On a pinned connection (`db.OnConn(pc)`) transactions are not available:
  `Begin`, `Tx`, `Replace`, `CreateGraph` return `ErrUnsupported`.
- Several momo helpers open their own transaction on a `*DB` and a savepoint on a
  `*Tx`: `Replace`, `CreateGraph`, chunked `Insert.Models`, `WithSearch`.

Details: README section 8. Example: [transactions](../examples/transactions/).

## 8. Relations

Declare with tags (section 4). Relations never load by themselves: a plain
`Select` leaves relation fields empty. Relation descriptors (`document.Comments`)
are immutable and concurrency-safe.

Decision table:

| Need                                                        | API                                                                                                                                                                                                                    |
| ----------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Children for rows you already fetched                       | `momo.Load(ctx, q, parents, rel, opts...)`: one statement per relation, not per parent                                                                                                                                 |
| Several relations for the same parents                      | `momo.LoadAll(ctx, q, parents, rel1, rel2.Scope(opts...), rel3.Then(nested...))`                                                                                                                                       |
| Filter, order and nest one relation                         | `rel.Scope(where, orderBy).Then(nested)` with `LoadWhere` and `LoadOrderBy` options                                                                                                                                    |
| Load as part of a query (`All`/`One`/`First`)               | a select query's `.Preload(rel, ...)`                                                                                                                                                                                  |
| Belongs-to with no extra round trip                         | `Preload(rel.Joined())`: LEFT JOIN in the parent statement                                                                                                                                                             |
| Only some children, ordered, top-N per parent, soft-deleted | `rel.Scope(opts...)` with `LoadWhere`, `LoadOrderBy`, `LoadPerParentLimit`, `LoadWithDeleted`, `LoadOnlyDeleted` (each generic over the child type, as in the example below; `Load` accepts the same options directly) |
| Children of children                                        | `rel.Then(nested...)`                                                                                                                                                                                                  |
| Filter parents by children                                  | `Where(rel.Has(scope...))`, `rel.HasNo(...)`, `rel.HasFrom(parentAlias, ...)`                                                                                                                                          |
| Order parents by child count                                | `OrderBy(rel.CountOrder(momo.DirDesc))`                                                                                                                                                                                |
| Children matching a parent query, no parent fetched         | `rel.From(parentQuery)` returns `SelectQuery[C]` (not many-to-many)                                                                                                                                                    |
| Add or remove links to EXISTING rows                        | `rel.Link(ctx, q, &parent, &c1, &c2)`, `rel.Unlink(...)`, `rel.Count(ctx, q, &parent)`                                                                                                                                 |
| Make the linked set exactly this one                        | `rel.Replace(ctx, q, &parent, children...)`                                                                                                                                                                            |
| Insert a parent and NEW children together                   | `momo.CreateGraph(ctx, q, &parent, rel1.Include(), rel2.Include())`                                                                                                                                                    |
| Get-or-insert on a unique key                               | `momo.FindOrCreate(ctx, q, []momo.ColumnRef{author.Name}, &a)` returns `(created bool, err)`                                                                                                                           |
| Insert or update by primary key                             | `momo.UpsertByPK(ctx, q, &m)`                                                                                                                                                                                          |

```go
docs, err := momo.Select[models.Document](db).
	Preload(
		document.Author.Joined(),
		document.Comments.Scope(
			momo.LoadWhere[models.Comment](comment.Approved.Eq(true)),
			momo.LoadOrderBy[models.Comment](comment.ID.Desc()),
			momo.LoadPerParentLimit[models.Comment](3)),
	).
	Limit(20).All(ctx)

withApproved := momo.Select[models.Document](db).
	Where(document.Comments.Has(comment.Approved.As("momo_r").Eq(true))) // child alias is momo_r

err = document.Tags.Link(ctx, db, &doc, &models.Tag{ID: 1}, &models.Tag{ID: 2})

doc := models.Document{Title: "Hello", Author: &models.Author{Name: "Ada"},
	Comments: []models.Comment{{Body: "first"}}}
err = momo.CreateGraph(ctx, db, &doc, document.Author.Include(), document.Comments.Include())
```

Rules and limits:

- `Load`/`LoadAll`/`Preload` clear the association first, so a parent with no
  children is empty (nil), never stale. NULL-key parents get an empty
  association. Parent order never changes.
- `Scope(...)` and `Then(...)` combine: both return a `RelationLoader` with
  `Scope` and `Then` methods, so
  `document.Comments.Scope(approvedOnly).Then(comment.Author)` (with
  `approvedOnly` a `momo.LoadWhere[models.Comment]` option) filters the comments
  and loads each one's author in one loader. The options
  apply to that relation's children only; nested loaders carry their own; the
  nested level runs over the surviving children only (one statement per level).
- `Joined()` is belongs-to only, and only when the child key is the child's
  primary key or a single unique column. Has-one, has-many, many-to-many give
  `ErrUnsupported`. `momo.Load(ctx, db, docs, X.Joined())` does not compile
  (`Joined()` returns a `Loader`, `Load` wants a `Relation`; cannot infer `C`),
  and `LoadAll` returns `ErrUnsupported`. Also `ErrUnsupported` with `Scope`,
  `Then`, `GroupBy`, `Having`, `ForUpdate`, `Each`, `Iter`. With `Columns` the
  select list must have one expression per mapped field (`ErrValidation`).
  Qualify raw column names in the parent's `Where` (ambiguity with the joined
  table).
- `LoadPerParentLimit(n)`: has-many and many-to-many only (belongs-to and has-one
  `ErrUnsupported`); `n < 0` is `ErrValidation`; 0 is unlimited. `LoadWhere`
  filters before the limit.
- `Has`/`HasNo` predicates must name the child column through the alias
  `momo_r`: `comment.Approved.As("momo_r").Eq(true)`; an unaliased column refers
  to the OUTER table.
- Has-one that matches several children for a parent: `ErrTooManyRows`.
- `Link`, `Unlink`, `Replace`, `Count` write SQL directly: model hooks, field
  hooks and `UpdatedAt` are NOT run. Use the model's update path when those must
  run. They never modify the values you pass.
- Many-to-many `Link` relies on a primary or unique key over the join columns.
  Join tables generated from `many2many` tags have one. A hand-written join table
  needs a unique key, or `Link` inserts duplicates.
- Has-one/has-many `Unlink` and `Replace` clear the foreign key: it must be
  nullable, else `ErrNotNullViolation`. Delete the children instead if they cannot
  exist without a parent.
- Belongs-to is not linkable: all four helpers return `ErrUnsupported`. Set the
  foreign key field on the parent and `Update` it.
- NULL parent key (or NULL child key on many-to-many) is `ErrValidation` for
  `Link`/`Unlink`/`Replace`; `Count` returns 0. `Link`/`Unlink` with no children
  are no-ops.
- `Replace` is a transaction (`*DB`) or savepoint (`*Tx`) and locks the parent row
  (`FOR UPDATE`), so concurrent `Replace` on one parent serialize. `Link`/`Unlink`
  take no lock and may interleave. On a `*Tx` the lock lasts until your
  transaction ends. A non-existent parent locks nothing.
- `Link`/`Unlink` needing several chunks are atomic on a `*DB` (one
  transaction); on a `*Tx` a later chunk failing leaves earlier chunks applied
  until you roll back.
- `CreateGraph`: one level deep; always INSERTs (never matches existing rows);
  belongs-to children first, has-one/has-many after the parent, many-to-many
  children inserted then linked; hooks, timestamps and defaults run. On a `*Tx` it
  runs in a savepoint. Nil parent, a model as its own child, or a NULL key that
  must be copied: `ErrValidation`. Has-many and many-to-many children go in as
  chunked bulk inserts (1000 children = one statement). A dialect without
  `RETURNING` gives `ErrUnsupported` before a transaction opens. DON'T list a
  many-to-many relation whose children are already stored: they are inserted
  again and fail with `ErrUniqueViolation`; use `Link` for existing rows.
- `FindOrCreate` needs a UNIQUE constraint on exactly the key columns. It
  overwrites your struct with the stored row when it exists; a soft-deleted
  row is not found, it fails with `ErrUniqueViolation`. Under REPEATABLE READ/SERIALIZABLE the re-select can return
  `ErrNotFound`: retry the transaction. Empty key or unmapped column:
  `ErrValidation`.
- Tag-declared relations need a single-column primary key on both models.
- A pipelined load buffers a whole level's child rows (about twice the memory of
  sequential). An unscoped load of a huge has-many is expensive: scope it.
- Many-to-many to self and join tables with extra columns are not supported by tag;
  use an explicit join model struct for extra columns.
- Parent keys are chunked under the bind limit automatically; `WithMaxParams(n)`
  lowers it for tests.

Details: [relations.md](relations.md). Example: [relations](../examples/relations/).

## 9. Vectors

```go
type Item struct {
	momo.Model `momo:"table:items"`
	ID         int64         `momo:"pk,identity"`
	Title      string        `momo:"size:200"`
	Category   string        `momo:"size:50,index"`
	Embedding  vector.Vector `momo:"dim:1536"` // dim = your embedding model's output size
}

func (i *Item) MomoConfig(c *momo.Config) {
	c.Index("items_embedding_hnsw", &i.Embedding).
		Method("hnsw").
		Opclass(&i.Embedding, "vector_cosine_ops"). // hnsw has NO default opclass: always set it
		With("m", "16").With("ef_construction", "64")
}
```

Column types (the Go type picks the SQL type; `dim` is required):

| Go type               | SQL            | Distance builders                                                     |
| --------------------- | -------------- | --------------------------------------------------------------------- |
| `vector.Vector`       | `vector(n)`    | `L2Distance`, `InnerProductDistance`, `CosineDistance`, `L1Distance`  |
| `vector.HalfVector`   | `halfvec(n)`   | same four                                                             |
| `vector.SparseVector` | `sparsevec(n)` | same four (`Dim`, zero-based strictly increasing `Indices`, `Values`) |
| `vector.Bits`         | `bit(n)`       | `HammingDistance`, `JaccardDistance`                                  |

A pointer field (`*vector.Vector`) is nullable (`momo.NullVectorColumn`).
Validation (before the database): length must equal `dim`; NaN and Inf rejected;
halfvec within 65504; max dim 16000 (`vector.MaxDim`). Failures wrap
`ErrValidation` (dimension mismatch, at `SQL()` or first execution) or
`vector.ErrInvalidVector` / `momo.ErrInvalidVector`.

Index rules (`MomoConfig`; validated by the generator, error wraps
`schema.ErrInvalidVectorIndex`):

| Method    | Types                           | Max dim for index                    | Opclass `<type>_<x>_ops`, x in                        |
| --------- | ------------------------------- | ------------------------------------ | ----------------------------------------------------- |
| `hnsw`    | vector, halfvec, sparsevec, bit | vector 2000, halfvec 4000, bit 64000 | `l2`, `ip`, `cosine`, `l1`; bit: `hamming`, `jaccard` |
| `ivfflat` | vector, halfvec, bit            | vector 2000, halfvec 4000, bit 64000 | `l2`, `ip`, `cosine`; bit: `hamming`                  |

- A `vector` column above 2000 dims cannot have an ANN index; store it as
  `HalfVector` (up to 4000 for an index).
- `hnsw` params: `m` 2..100 (16), `ef_construction` 4..1000 (64, at least 2*m).
  `ivfflat` REQUIRES `With("lists", "N")` (1..32768).
- An index covers one plain column; expression parts are rejected.
- Index opclass must match the distance function: `vector_cosine_ops` with
  `CosineDistance`, `vector_l2_ops` with `L2Distance`, `vector_ip_ops` with
  `InnerProductDistance`, `vector_l1_ops` with `L1Distance`. A mismatch silently
  runs a sequential scan.

Nearest-neighbour query; `Asc()` is nearest first:

```go
distance := momo.CosineDistance(item.Embedding, query) // query is vector.Vector of dim 1536
hits, err := momo.Select[models.Hit](q). // projection with Title string, Distance float64 (section 4); soft-deleted items are filtered automatically
	From(item.Table).
	Columns(item.Title, distance).
	Where(item.Category.Eq("news")).
	OrderBy(distance.Asc()). // distance only: see the tie-break note below
	Limit(10).
	All(ctx)

// Rows only (the soft-delete filter applies automatically):
items, err := momo.Select[models.Item](q).
	OrderBy(momo.CosineDistance(item.Embedding, query).Asc()).Limit(5).All(ctx)
```

- `Hit` is a projection (section 4); the generator emits its meta. The select
  list is positional, so the field name `Distance` only labels the computed
  column.
- A vector literal is `vector.Vector{0.1, 0.2, 0.3}` (`vector.Vector` is
  `[]float32`; its length must equal `dim`).
- `InnerProductDistance` is pgvector `<#>`: the NEGATIVE inner product, so
  ascending order is closest.
- Vector columns have no `Lt/Le/Gt/Ge/Between`; use a distance builder.
- Passing a dense vector to a bit builder (or the reverse) does not compile.
- `Where` only narrows candidates; it never reorders.
- Order a vector search by the distance alone. Adding a tie-break key such as
  `item.ID.Asc()` makes PostgreSQL sort on top of the index scan, and on some
  versions (seen on PostgreSQL 16) the planner then skips the HNSW or IVFFlat
  index entirely. Order among equal distances is unspecified. Check the plan
  with `Explain` (see [vector-search](../examples/vector-search/)).

Search settings (transaction-scoped; revert at commit or rollback; safe behind
PgBouncer):

```go
err := db.WithSearch(ctx, momo.SearchSettings{
	HNSWEfSearch:          100,                        // 1..1000 (default 40; at least your LIMIT)
	HNSWIterativeScan:     momo.IterativeRelaxedOrder, // pgvector >= 0.8: filtered searches
	HNSWMaxScanTuples:     20000,                      // pgvector >= 0.8
	HNSWScanMemMultiplier: 2,                          // pgvector >= 0.8; 1..1000 x work_mem
}, func(tx *momo.Tx) error {
	// run EVERY vector query for this search on tx, not db
	_, err := momo.Select[models.Item](tx).
		OrderBy(momo.CosineDistance(item.Embedding, query).Asc()).Limit(10).All(ctx)
	return err
})
// Inside an existing transaction: tx.SetSearch(ctx, settings)
```

Fields: `ExactSearch` (turns off index AND bitmap scans for the whole transaction:
exact results, but every other index lookup in that transaction also goes
sequential, so keep it short and dedicated), `HNSWEfSearch`,
`HNSWIterativeScan` (`IterativeOff`, `IterativeRelaxedOrder`,
`IterativeStrictOrder`), `HNSWMaxScanTuples`, `HNSWScanMemMultiplier` (float, 1..1000), `IVFFlatProbes`,
`IVFFlatIterativeScan` (`IterativeOff`, `IterativeRelaxedOrder` only),
`IVFFlatMaxProbes`. Zero leaves the server default. Out of range:
`ErrValidation`. Iterative/max-scan/scan-mem/max-probes fields on pgvector older than 0.8.0
or a missing extension: `ErrUnsupported`.

Pitfalls:

- Filtered search: with iterative scan off, an ANN index returns its nearest
  candidates and applies `Where` afterwards, so a selective filter can return FEWER
  than `Limit` rows even when enough match. Set `HNSWIterativeScan`
  (`IterativeRelaxedOrder`; results stay approximate and can still be short when
  the filter matches fewer rows than `HNSWMaxScanTuples`).
- Small-table seq scan: below a few thousand rows the planner prefers a
  sequential scan, so `Explain` shows no index use in tests. That is correct. To
  check index use on a small table, run in a transaction:
  `tx.SetLocal(ctx, "enable_seqscan", "off")` then `query.Explain(ctx)` and look for
  the index name. `Tx.SetLocal(ctx, name, value)` sets any run-time parameter; never
  pass an untrusted name.
- Opclass mismatch or missing opclass for hnsw: see above.
- Index-less exact search (`ExactSearch`) is for measuring recall or small tables.
- Binary wire codec is opt-in (`pgxdriver.ConfigureVectors(cfg)` before
  `pgxpool.NewWithConfig`, then `pgxdriver.WithVectorFormat(pgxdriver.VectorBinary)`
  with a describe-based exec mode). Default text transport is correct; skip unless
  profiling says otherwise. `NewPool` does not call `ConfigureVectors`.
- Hybrid (full text plus vector) search does not exist.

Details: [vectors.md](vectors.md). Example: [vector-search](../examples/vector-search/).

## 10. Errors

Momo errors are `*momo.Error{Kind, Op, Table, Column, Constraint, Err}`. Check
with `errors.Is(err, momo.ErrX)` (matches `Kind`) and reach the driver error with
`errors.As` (for example `*pgconn.PgError`). `e.Detail()` returns driver detail
that `Error()` hides. Never string-match error text.

```go
switch {
case errors.Is(err, momo.ErrNotFound):
case errors.Is(err, momo.ErrUniqueViolation):
	var me *momo.Error
	if errors.As(err, &me) { _ = me.Constraint } // which constraint
}
```

`me.Column` is empty for unique, foreign-key and check violations (PostgreSQL
reports a column only for NOT NULL). Use `Constraint` for those three.

| Error                    | When                                                                                                                                                                                                                                                                                                      | What to do                                          |
| ------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------- |
| `ErrNotFound`            | `One`, `First`, `Update.Model`, `Delete.Model` matched no row; `FindOrCreate` re-select miss. `All`/`Each`/`Iter` never return it                                                                                                                                                                         | map to 404 or create                                |
| `ErrTooManyRows`         | `One` matched more than one row; has-one load matched several children                                                                                                                                                                                                                                    | fix the filter or the data                          |
| `ErrUniqueViolation`     | unique key or primary key violated (SQLSTATE 23505)                                                                                                                                                                                                                                                       | conflict; `Constraint` names it                     |
| `ErrForeignKeyViolation` | FK violated (23503)                                                                                                                                                                                                                                                                                       | missing parent or children still exist              |
| `ErrCheckViolation`      | CHECK failed (23514)                                                                                                                                                                                                                                                                                      | business rule                                       |
| `ErrNotNullViolation`    | NULL into NOT NULL (23502); has-one/has-many `Unlink`/`Replace` on a NOT NULL FK                                                                                                                                                                                                                          | fix the data or nullability                         |
| `ErrSerialization`       | serialization failure or deadlock (40001, 40P01)                                                                                                                                                                                                                                                          | retry the whole transaction with backoff            |
| `ErrBusy`                | lock not available (55P03)                                                                                                                                                                                                                                                                                | retry later                                         |
| `ErrOptimisticLock`      | `Update.Model` on a versioned model with a stale version                                                                                                                                                                                                                                                  | reload, reapply, retry                              |
| `ErrUnfilteredMutation`  | `Update`/`Delete` without `Where` and without `AllRows()`                                                                                                                                                                                                                                                 | add a filter                                        |
| `ErrValidation`          | bad input detected before the database: wrong vector dim, `After` count, empty `Models`, writing a reserved/read-only column, plain `Delete` on a soft-delete model, negative `Limit`, `Link` with NULL key, bad `SearchSettings`, qualifier not in FROM                                                  | programmer or input error; fix the call             |
| `ErrUnsupported`         | dialect or driver cannot do it: `Each`/`Iter` with `Preload`; `Joined` shape; `LoadPerParentLimit` on to-one; belongs-to helpers; keyset on nullable key; `IndexedBy`; iterative search on pgvector < 0.8; simple protocol exec mode; non-standard isolation; transactions on a pinned conn; SQLite/MySQL | redesign the call; not retryable                    |
| `ErrAdoptedTx`           | `Commit` or `Rollback` on a `*Tx` from `AdoptTx`; the caller owns that transaction                                                                                                                                                                                                                        | finish the transaction with the caller's own handle |
| `ErrTxDone`              | `*Tx` already committed or rolled back                                                                                                                                                                                                                                                                    | do not reuse a finished `Tx`                        |
| `ErrLimitExceeded`       | configured limit exceeded (for example one row has more columns than the bind limit)                                                                                                                                                                                                                      | split or raise the limit                            |
| `ErrPinnedBusy`          | `Release` on a pinned connection that is still executing                                                                                                                                                                                                                                                  | wait and release again                              |
| `ErrInvalidVector`       | a vector value failed validation (same value as `vector.ErrInvalidVector`)                                                                                                                                                                                                                                | fix the vector                                      |

Driver errors Momo does not classify keep their original type: reach them with
`errors.As(err, &pgErr)`. Data exceptions (SQLSTATE class 22, for example
invalid text representation) hide the message because it may quote the value; the
`*pgconn.PgError` is still reachable.

Migration errors (package `migrate` and the CLI, exit codes in section 5):
`migrate.ErrChecksumMismatch`, `ErrDirty`, `ErrLocked`, `ErrDestructive`,
`ErrBranchConflict`, `ErrStagedRequired`, `ErrBaselineMismatch`, `ErrNoSnapshot`,
`ErrNoDown`; `migrate.ErrNewerFormat` (a snapshot written by a newer momo).

Rules:

- DO use `errors.Is`, not `==` (errors are wrapped).
- DON'T retry `ErrUnique`/`ErrValidation`/`ErrUnsupported`; DO retry
  `ErrSerialization` (whole transaction) and `ErrBusy` (with backoff).
- DON'T return raw `*momo.Error` text to API clients; it names tables and
  constraints.

## 11. Testing apps that use Momo

Test against a real PostgreSQL with pgvector, not a mock. CI of Momo itself runs
`pgvector/pgvector:0.8.7-pg16` and `0.8.7-pg17`.

Pattern used by the examples (the harness is internal to the Momo module; copy
the idea, not the package):

1. Read an admin URL from an env var (the examples use `MOMO_TEST_POSTGRES_URL`);
   `t.Skip` when it is unset (and `t.Fatal` when your CI sets a require flag).
2. Create a scratch database per test (`CREATE DATABASE t_<random>` over a plain
   pgx connection), connect `pgxdriver.NewPool` to it, drop it in `t.Cleanup`.
3. Apply your real migrations from code:

```go
import (
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

db := momo.New(pgxdriver.New(pool))
e, err := migrate.NewEngine(db.Driver(), os.DirFS("../migrations"), migrate.WithDialect(mpg.New()))
if err != nil { t.Fatal(err) }
if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil { t.Fatal(err) }
```

(The role needs permission to `CREATE EXTENSION vector`.) Pass the migrations
directory configured in `migrate.dir.postgres`. 4. Run the code under test against `db`; assert on results and on `errors.Is`. 5. Also run `momo generate --check` and `momo migrate validate` as tests or CI
steps.

What to mock and what not:

- DON'T mock `momo.DB`, `momo.Tx` or `momo.Querier`. `Querier` is sealed (it has
  unexported methods; you cannot implement it) and query builders are the code
  under test.
- DO hide Momo behind your own repository/service interface and fake THAT in unit
  tests of business logic.
- DO test queries, migrations, relations and transactions against real PostgreSQL.
- Use `WithClock(func() time.Time)` (`momo.New(drv, momo.WithClock(fn))`) for
  deterministic `created_at`/`updated_at`/soft-delete times.
- `WithMaxParams(n)` with a small `n` exercises chunking with little data.
- The `momo.Driver` interface can be faked, but the mock then proves nothing about
  SQL; prefer `Select...SQL()` golden checks for pure SQL-shape tests.

Examples: every `examples/*/example_test.go`, harness
[examples/internal/exampletest](../examples/internal/exampletest/exampletest.go).

## 12. Rules for AI agents

1. v0.1.0 is PostgreSQL + pgvector via pgx only. Never target SQLite or MySQL.
2. Install the CLI at the same version as the library
   (`go install github.com/azrtydxb/momo/cmd/momo@v0.1.0`).
3. Models embed `momo.Model` by value with `momo:"table:<name>"`; run
   `momo generate` after every model change and commit the generated files.
4. Never edit `*_gen.go`. Never hand-write `MomoMeta` for a table model or a
   projection.
5. Non-pointer field = NOT NULL; pointer = NULL-able. Use `SetNull()` only on
   pointer fields.
6. Use `Insert.Model` to store the struct as-is (zeros included); `Insert.Values`
   to fall back to database defaults. Use `Returning(...).Rows` to read identities.
7. Use `Update.Set(col.Set(v))` for partial updates, `Update.Model` for whole-row
   updates with a `version` column; handle `ErrOptimisticLock` by reload and
   retry. `Set` updates do not touch the version.
8. Never call `AllRows()` to get past `ErrUnfilteredMutation`; add a `Where`.
9. Soft-delete models: `SoftDelete`/`ForceDelete`, not `Delete`. Raw SQL and
   projections whose `From` is a subquery or a hand-built `TableRef` do not get
   the soft-delete filter: add `Where(<table>.DeletedAt.IsNull())` yourself. A
   projection read `From` a generated table (`Select[Card]`, the vector `Hit`)
   is filtered automatically; do not add the predicate.
10. Page with `OrderBy` + `After` and a unique tie-break; pass one value per
    effective ORDER BY key (primary key included).
11. Pass user input only as arguments to `momo.SQL`/`TypedSQL`/`Exec`/`Scalars`;
    qualify column names in raw fragments inside joins.
12. Inside `db.Tx`, use only the `tx` handle. Retry the whole transaction on
    `ErrSerialization` with jittered backoff; never retry inside it.
13. Check errors with `errors.Is` against the `momo.Err*` sentinels.
14. Load relations explicitly (`Load`, `LoadAll`, `Preload`); never assume a field
    is populated. `Scope` and `Then` chain on one relation:
    `rel.Scope(opts...).Then(nested...)`.
15. `Joined()` is belongs-to only; `Link`/`Unlink`/`Replace`/`Count` are not for
    belongs-to; helpers skip hooks and `UpdatedAt`.
16. `FindOrCreate` and `OnConflict` need a matching UNIQUE constraint; many-to-many
    join tables need a unique key. A partial unique index needs
    `OnConflict(cols).Where(indexPredicate)`; a conditional DO UPDATE uses
    `.DoUpdate(...).Where(...)`.
17. Vector columns need `dim:<n>`; hnsw/ivfflat indexes need an explicit opclass
    matching the distance function used in queries; ivfflat needs `lists`.
18. Run vector queries that need tuning inside `db.WithSearch` and use `tx`
    there; use iterative scan for filtered ANN searches; expect seq scans on small
    tables.
19. Migrations: `diff`, review SQL, commit all files, `apply`. Never edit an
    applied migration; undo with a new one. No rollback exists.
20. NOT NULL additions without a default: `diff --stage`, apply, backfill, check
    no NULLs remain, then `diff --enforce-not-null` (or give the column a
    default).
21. Declare `Concurrently()` on large-table indexes. `-- momo:destructive` is
    informational; only `diff --allow-destructive` guards drops.
22. CI must run `momo generate --check`, `momo migrate validate` and the
    `diff ci_probe | grep -qx 'no changes'` probe; `apply` also refuses forks and
    out-of-order versions, but fail the pull request earlier.
23. Run `momo migrate` over a direct connection, URL from the environment, never
    `--url` (or `--url -` for stdin). Use `sslmode=verify-full` for remote
    databases.
24. Behind PgBouncer keep the default exec mode; never select the simple
    protocol.
25. Test against real PostgreSQL with pgvector; do not mock `momo.Querier`.
