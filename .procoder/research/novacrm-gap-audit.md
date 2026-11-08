# NovaCRM port: momo gap audit

Date: 2026-11-07. Momo: branch `sprint-5` (v0.3.0 plus unreleased work). NovaCRM: `origin/main` of the NovaCRM-goplan checkout, read through git objects only. Existing issues #27 to #34 on azrtydxb/momo were read and are confirmed or sized below.

Sources I could not find: the branch `origin/docs/go-backend-migration` does not exist in the NovaCRM remote (remote branches with "docs" are only `docs/kuvryn-licensing-spec`). Nothing from a Go migration plan was read, so this audit is derived from the code only.

## 1. What NovaCRM actually uses

| Fact                                           | Count / evidence                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Prisma models                                  | 129 (`prisma/schema.prisma`, 3007 lines), 0 enums, 0 `@@map`/`@map` (table and column names are the Prisma names)                                                                                                                                                                                                                                                                                                                                     |
| Indexes / uniques / FKs with `onDelete`        | 147 `@@index`, 36 `@@unique`, 154 `onDelete`                                                                                                                                                                                                                                                                                                                                                                                                          |
| IDs                                            | 113 `String @id @default(cuid())` (client-side generated, no DB default), 16 other string ids, 6 composite `@@id`                                                                                                                                                                                                                                                                                                                                     |
| Non-PK sequences                               | 9 `Int @unique @default(autoincrement())` (`seq` columns, `Contact_seq_seq`)                                                                                                                                                                                                                                                                                                                                                                          |
| Column types                                   | 280 `DateTime` (`timestamp(3)`), 54 `Json`, 24 `Json?`, 14 `String[]`, 8 `Bytes`, 7 `BigInt`, 3 `@db.Date`, 0 `Decimal`, 2 `Unsupported` (`tsvector`, `vector`)                                                                                                                                                                                                                                                                                       |
| Schema management                              | No `prisma/migrations`. `prisma db push` plus 52 hand-written idempotent SQL files in `prisma/sql/` (5807 lines): 126 CREATE TABLE, 271 ALTER TABLE, 235 CREATE INDEX (20 partial, 1 GIN), 117 `DO $$` guard blocks, 32 UPDATE backfills, 7 NOT VALID FKs, 2 CHECKs, 1 plpgsql function + 1 trigger, 0 views/enums/RLS/citext/generated columns/partitions                                                                                            |
| Roles                                          | 47 `GRANT ... TO novacrm_app` lines in older files; newer files say the app user owns all tables. `CREATE EXTENSION vector` is a superuser-only manual step (CNPG)                                                                                                                                                                                                                                                                                    |
| Raw SQL in app code (non-test)                 | 12 files, 54 lines: `kuvryn/analyticsEngine.ts` 21, `kb/search.ts` 15, `kb/embed.ts` 4, and one each in webhooks/dispatch, platform/provisioning/worker, identity/grantSync, entitlements/recompute, documents/worker, documents/snapshots, catalog/products, accounts/members. 79 more raw-SQL lines are in test mocks                                                                                                                               |
| Transactions                                   | 162 `$transaction` (135 interactive, 2 array form), 1 isolation level (`Serializable`, `booking/booking.ts:321`), 0 LISTEN/NOTIFY, 0 `$extends`/`$use` middleware, 0 RLS, 0 multi-schema                                                                                                                                                                                                                                                              |
| Locking                                        | `FOR UPDATE` 2 sites (`accounts/members.ts`, `entitlements/recompute.ts`), `FOR UPDATE SKIP LOCKED` 4 claim queries (documents worker, grantSync, provisioning worker, webhooks dispatch), `FOR UPDATE OF d SKIP LOCKED` 1 (dispatch), `pg_advisory_xact_lock(hashtext(..))` 2 (`catalog/products.ts`, `documents/snapshots.ts`), `SET LOCAL statement_timeout` 1 (analytics)                                                                         |
| Prisma client features (non-test, line counts) | findMany 496, findUnique 464, create 206, findFirst 148, updateMany 111, upsert 83, deleteMany 28, groupBy 21, aggregate 8, createMany 12 (4 with skipDuplicates), `include` 207, `select` 623, `_count` selects 22, relation filters `some` 12 and `none` 4 (no `every`), `mode: "insensitive"` 14, `distinct` 5, `increment` 14, Prisma `cursor` 2, JSON `path` filters 2 (`history/routes.ts:132,135`), `Prisma.JsonNull/DbNull/InputJsonValue` 43 |
| Pool                                           | `connection_limit=5`, `pool_timeout=10` (`apps/api/src/db.ts`)                                                                                                                                                                                                                                                                                                                                                                                        |
| Tests                                          | vitest with `prisma` mocked (79 raw-SQL mock lines, `testing/renderedDocumentsDb.ts` fakes the SKIP LOCKED claim). 41 API modules. `prisma/seed.ts` is 987 lines                                                                                                                                                                                                                                                                                      |

Conclusion: the raw SQL surface is small (12 sites) and every one of them can run today through momo `Raw`/`Scalars`/`Exec` inside a `Tx`. The real port cost is the 129-table model layer and Prisma-shaped schema conventions, then tests.

## 2. Gaps already filed (#27 to #34): confirmed and sized

| Issue                                              | Verdict                                                  | NovaCRM evidence                                                                                                                                                                                                                   | Momo today                                                                                                                                                | Size                                                                                          | Port-blocking                                                                                                                                                                                           |
| -------------------------------------------------- | -------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| #27 `generate --from-db`                           | Confirmed; the largest single item                       | 129 tables, 147 indexes, 36 uniques, 154 FKs                                                                                                                                                                                       | `Engine.Inspect` returns a Go `schema.Schema`; `baseline` demands hand-written models that match exactly; stories 229/230 open                            | L (split in 3: emit models, emit relations from FKs, emit init snapshot)                      | Yes (practically; hand-writing 129 models is not credible)                                                                                                                                              |
| #28 Prisma naming preset                           | Confirmed                                                | 280 `DateTime` as `timestamp(3)`; every table PascalCase, column camelCase; names `<T>_pkey`, `<T>_<c>_fkey`, `<T>_<c>_key`, `<T>_<c>_idx`                                                                                         | `naming.ByName` knows momo/gorm/bun/ent; `time.Time` is `timestamptz`; `column:` and `type:` tags work per field                                          | M                                                                                             | Yes (baseline must diff clean). Note: if #27 emits explicit `column:`/`type:` tags, #28 is only needed for new models; schedule #28 before #27 ships or fold the default-timestamp-type option into #27 |
| #29 dimensionless `vector`                         | Confirmed; already pulled into sprint 5 (commit 4398651) | `KbChunk.embedding Unsupported("vector")`, filtered by `model` and `dims` (`kb/search.ts:191-212`, `kb/embed.ts`)                                                                                                                  | Sprint 5                                                                                                                                                  | S                                                                                             | No (in flight). Check it landed with a `CosineDistance` test. The optional partial/expression HNSW index is not needed by NovaCRM today (no vector index in `prisma/sql`)                               |
| #30 full-text search                               | Confirmed, but not blocking                              | `KbArticle.searchTsv` (trigger `kb_search_tsv`, GIN index), `websearch_to_tsquery('english')`, `ts_rank_cd`, `ts_headline` in `kb/search.ts:103-119,192` (2 queries)                                                               | No `tsvector` mapping; GIN index and function/trigger declarable through `MomoConfig` (`Index().Method("gin")`, `Function`, `Trigger`); queries via `Raw` | M                                                                                             | No (2 raw queries). The column can be declared with a `type:tsvector` tag plus `read_only`; verify that scanning it is not needed (select only through raw)                                             |
| #31 `text[]` and array operators                   | Confirmed                                                | 14 `String[]` columns; `has/hasSome` 7; one raw `ARRAY[..] && a."products"` (`kb/search.ts:66`)                                                                                                                                    | Declarable by hand; no generator mapping; no `@>`, `<@`, `&&`, `ANY` predicates                                                                           | S to M                                                                                        | No for runtime; yes for #27 (the from-db emitter must emit `[]string` + `type:text[]`)                                                                                                                  |
| #32 advisory locks and `FOR UPDATE OF`             | Confirmed                                                | 2 advisory sites, 1 `FOR UPDATE OF d SKIP LOCKED`; 4 plain `SKIP LOCKED` claims and 2 `FOR UPDATE` row locks are already covered by `SelectQuery.ForUpdate(opt...)`                                                                | `ForUpdate`/`NOWAIT`/`SKIP LOCKED` yes; no `OF`; advisory via `Exec`                                                                                      | S                                                                                             | No                                                                                                                                                                                                      |
| #33 `unnest`, window, FILTER, DISTINCT ON, LATERAL | Confirmed; real but spread out                           | `analyticsEngine.ts`: `unnest(text[],float8[])` FX CTE (l.286), `LEFT JOIN LATERAL` (l.71, 189), `COUNT(*) FILTER` (l.191-193); `webhooks/dispatch.ts:190` `row_number() OVER (PARTITION BY ..)`; `kb/search.ts:197` `DISTINCT ON` | CTE, recursive CTE, subqueries, `Union`, `FromSubquery`, `RawSubquery` yes; no user-facing window API                                                     | M to L (split: unnest/generate_series source S, FILTER S, DISTINCT ON S, window M, LATERAL M) | No. The analytics engine builds whitelisted dynamic SQL strings and would stay on `Raw` even with these APIs                                                                                            |
| #34 `momotest` kit                                 | Confirmed; I rate it blocking in practice                | Current suite mocks `prisma` everywhere (79 raw mock lines); 41 modules need real-Postgres tests; `seed.ts` 987 lines                                                                                                              | `docs/ai-guide.md` section 11 shows the pattern; `internal/testdb` and `migrate/migratetest` are internal; `momotest` is in "Planned, not implemented"    | M                                                                                             | Soft yes: without it each of 41 modules re-invents a harness and the first module ports slowly                                                                                                          |

## 3. New gaps (not in #27 to #34)

### N1. Go-side ID generation (cuid, uuid text, ulid)

- Evidence: 113 of 129 primary keys are `String @id @default(cuid())`. The database has no default for them, so Prisma generated the id in the client. Existing rows hold cuid strings.
- Momo today: `FieldHook.Default` exists (`fields.go`) but only for hand-written `Meta.Fields` or `WithFieldHooks[T]`. It never replaces a zero value; a `Model(&m)` insert writes every column not in `Omit` (doc comment, `fields.go`). So every insert path must `Omit("id")` or set the id by hand. No tag or `MomoConfig` method declares a generator. `default:<sql>` accepts only SQL (`gen_random_uuid()::text` would work but changes the existing schema and gives uuid-shaped, not cuid-shaped, ids).
- Proposal: a tag `idgen:cuid|uuid|ulid|nanoid` (and `MomoConfig.IDGen(&m.ID, momo.CUID)`), emitted into `Meta.Fields` as a `Default` that also fires when the field holds its Go zero value (new opt-in `ZeroDefault bool` on `FieldHook`, or `FieldHook.DefaultIfZero`). Generator functions are pluggable: `momo.WithIDGenerator("cuid", func() string)`.
- Size: S to M. Port-blocking: no (workaround exists), but it touches 113 models and every insert. Issue: new.

### N2. Introspection overrides and re-sync for `--from-db` (extends #27)

- Evidence: Prisma semantics are not recoverable from the database: `cuid` ids (N1), `@updatedAt` (54 columns, plain `timestamp(3)` NOT NULL with no default), `Json` payload shapes (54 columns), `seq` serial columns (9; look like `integer DEFAULT nextval(..)`), `Unsupported` columns. The schema keeps changing in Prisma (`prisma db push`, new SQL files nearly daily) while the Go port is built, so models must be regenerable.
- Momo today: nothing (no from-db at all).
- Proposal: in `momo.yaml`, `introspect:` section with per-table/per-column rules (`updated_at: ["updatedAt"]` to add the `updated_at` tag, `id_gen: cuid` for `id` text PKs, `json: {Customer.metadata: "map[string]any"}`, `skip_tables`, `type_overrides`), and a re-run mode that rewrites only generated model files and reports drift against hand edits instead of overwriting them.
- Size: M. Port-blocking: yes with #27 (without it the generated models need manual edits that the next run overwrites). Issue: new (companion to #27).

### N3. JSON/JSONB operators

- Evidence: 54 `Json` columns; Prisma `path:` filters at `history/routes.ts:132,135` (`after->>'via'`, `after->>'agent'`); jsonb functions in 19 lines of the SQL files (backfills only).
- Momo today: `types.JSON[T]` maps to `jsonb`; the dialect flag `JSON` exists in `internal/suite/caps.go` ("operators arrive with the JSON and array operator task"). No builders for `->`, `->>`, `#>>`, `@>`, `?`, or comparisons on extracted text. Reading nullable JSON needs `*types.JSON[T]` (plain `types.JSON` errors on NULL).
- Proposal: `momo.JSONText(col, "via").Eq("x")`, `JSONPath(col, "a", "b")`, `JSONContains(col, v)`, `JSONHasKey(col, "k")`; docs for SQL NULL vs JSON null (Prisma `DbNull` vs `JsonNull`, 43 uses).
- Size: S. Port-blocking: no (2 call sites; `Raw` or the generic `Func` helper with jsonb_extract_path_text works). Issue: new.

### N4. Relation counts for many parents in one query

- Evidence: 22 `_count: { select: { rel: true } }` selects (e.g. `projects/projects.ts:44`); 50 `_count` lines in total.
- Momo today: `Relation.Count(ctx, q, p)` counts for one parent (N+1 over a list); `Relation.CountOrder` orders by a correlated count; `Load*` loads rows, not counts. A projection with a correlated `COUNT(*)` subquery works by hand.
- Proposal: `momo.LoadCounts(ctx, q, parents, rel) (map[key]int64, error)` (one grouped query per bind-limit chunk) or `Select.WithCount(rel, "member_count")` into a projection column.
- Size: S to M. Port-blocking: no. Issue: new.

### N5. Typed date/time and misc SQL expression helpers, composable raw fragments

- Evidence: `analyticsEngine.ts` uses `date_trunc('month', ..)`, `to_char(.., 'YYYY-MM')`, `EXTRACT(EPOCH FROM (b - a))`, `NULLIF`, `AVG(CASE ..)`, `COALESCE`, `::float8` casts. Dynamic WHERE assembly with `Prisma.join` in `kb/search.ts` (15 lines).
- Momo today: generic the generic `Func` helper covers `date_trunc`, `to_char`, `NULLIF`, `GREATEST`, `LEAST`; `Cast`, `Case`, `CoalesceExpr`, `Add/Sub/Mul` exist. Not expressible without raw: `EXTRACT(field FROM x)` syntax, `AT TIME ZONE`, interval literals/arithmetic, `now()` as a typed `Expression[time.Time]`. `Raw` accepts `Expr` arguments inline and `In(...)` lists but has no join-of-fragments helper (`Prisma.join(parts, " AND ")`).
- Proposal: `momo.Extract` (field, expr), `AtTimeZone`, `Interval`, `Now()`, `DateTrunc(unit, expr)`, `ToChar`, `Nullif`, `Greatest/Least`; `momo.JoinSQL(sep string, parts ...Expr) Expr`.
- Size: S to M. Port-blocking: no (analytics engine stays raw by design). Issue: new.

### N6. Migration coexistence and cutover (Prisma `db push` plus SQL files to momo)

- Evidence: the schema is currently changed by `prisma db push` and idempotent files with 117 `DO $$ ... IF NOT EXISTS` guards; `CREATE EXTENSION vector` is done by hand by a superuser (`2026-10-08-knowledge-3-search.sql`, lines 4676-4712 of the concatenated audit file). The app user is not a superuser and may lack `CREATE` on the database. 47 `GRANT` lines exist; newer files have none.
- Momo today (from `docs/migrations.md`): `baseline` runs model DDL on the target in a scratch schema and needs `CREATE` on the database (`ErrCanonicalizePrivilege`). Generated migrations emit `CREATE EXTENSION IF NOT EXISTS` for vector columns; no way to say "extension is managed outside, do not emit". No `migrate new --custom` (story 098, hand-written files work but are manual) and no data-migration step (backfills are hand-written SQL, documented). Schema model has no roles/grants. `drift` reports `Unmanaged` objects but there is no documented dual-writer (Prisma still pushing) procedure.
- Proposal: (a) `migrate.extensions: external` or `-- momo:requires-superuser` handling and a check that a pre-existing extension never needs privileges; (b) a written cutover playbook (freeze `db push`, baseline, hand off DDL ownership), with `momo migrate drift --ignore-unmanaged` style filtering; (c) `migrate new --custom` (story 098).
- Size: M (docs and verification spike S, flags S). Port-blocking: not for the first modules (the DB already exists), yes before Go owns DDL. Issue: new.

### N7. Roles, grants and sequences in the schema model (low priority)

- Evidence: 47 `GRANT ... TO novacrm_app` lines and 3 `GRANT USAGE ON SEQUENCE` in old SQL files; newer files explicitly have no grants.
- Momo today: `schema.Schema` has no grants or standalone sequences; they pass through hand-written migrations only.
- Proposal: do nothing now. Record as non-goal unless the deployment reintroduces a separate app role.
- Size: M if ever built. Port-blocking: no. Issue: none (documented non-goal).

### N8. Seed and fixture helpers

- Evidence: `prisma/seed.ts` is 987 lines; tests need rows (41 modules).
- Momo today: none. `Upsert`, `Insert.Models`, `CreateGraph` cover the work but not factories or ordering of 129 tables.
- Proposal: fold into #34 as `momotest.Seed`/`Load(fixtures)` (transactional, FK-ordered). No separate issue.
- Size: S inside #34. Port-blocking: no.

## 4. Covered by momo today (no work)

Mapped one-to-one, checked in `docs/capabilities.md` or code:

- `findMany/findFirst/findUnique/create/update/delete/updateMany/deleteMany`: `Select`, `Insert`, `Update`, `Delete` (WHERE-less mutations refused unless `.AllRows()`).
- `upsert` (83): `Upsert`, `OnConflict`; `createMany skipDuplicates` (4): `OnConflict` do nothing; chunked insert over 65535 binds.
- `include` (207) and `select`: `Preload`/`Load` (nested, pipelined) and projections; relation filters `some`/`none` (`relpred.go`, exists/not exists; `every` is unused).
- `groupBy` (21), `aggregate` (8), `distinct` (5), `increment` (14, `Add` with `SetExpr`), `mode: insensitive` (14, `ILike` with `EscapeLike`), `OR`/`NOT` predicates.
- `$transaction` interactive (162): `db.Tx`, savepoints, `Serializable` plus `ErrSerialization` retry, read-only; `SET LOCAL statement_timeout`: `Tx.SetLocal(ctx, "statement_timeout", "5000")`.
- `FOR UPDATE`, `SKIP LOCKED`, `NOWAIT`: `ForUpdate(opt...)`.
- `@updatedAt`: `updated_at` tag; `@default(now())`: `default:now()`; `CURRENT_TIMESTAMP` defaults are normalized (`schema/normalize.go`).
- `autoincrement()` on non-PK `seq`: `serial` tag works on any integer column (not limited to the PK).
- Composite PKs (6), `onDelete`/`onUpdate` (`on_delete:`, `on_update:`), partial unique indexes (`Index/Unique(..).Where(..)`), CHECK constraints (`Check`), GIN index (`Method("gin")`), functions and triggers (`Function`, `Trigger`), NOT VALID FKs and `VALIDATE`, `CREATE INDEX CONCURRENTLY`.
- `BigInt` (`int64`), `Bytes` (`[]byte`), `@db.Date` (`types.Date`), `Json` (`types.JSON[T]`, pointer for nullable), `vector` (`vector.Vector`, search settings, 6 metrics).
- pgbouncer/transaction pooling (default exec mode), TLS, pool size (pgx `pool_max_conns` in the DSN).
- Adopting an existing database: `baseline`, `drift`, `validate`, hand-written migrations with checksums.

Not used by NovaCRM, so no gap: enums, citext, generated columns, views/materialized views, RLS, LISTEN/NOTIFY, multi-schema, `$extends`/middleware, soft-delete extension, trigram.

## 5. Ranked sprint-6 proposal

Port blockers first.

| Rank | Item                                                            | Issue     | Size             | Blocking                   | Notes                                                                                                                           |
| ---- | --------------------------------------------------------------- | --------- | ---------------- | -------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| 1    | `generate --from-db`: models, relations from FKs, init snapshot | #27       | L (3 stories)    | Yes                        | Do this first; everything else is measured against a diff-clean baseline of the NovaCRM database                                |
| 2    | Introspection overrides and re-sync                             | N2 (new)  | M                | Yes                        | Land with #27 so generated models carry `updated_at`, id generator and JSON type decisions                                      |
| 3    | Prisma naming preset and default timestamp type                 | #28       | M                | Yes                        | Needed for new models and for `baseline` diffing clean (`timestamp(3)`, `_fkey/_key/_idx`)                                      |
| 4    | `momotest` kit with seed helper                                 | #34 (+N8) | M                | Soft yes                   | Unblocks real-Postgres tests for 41 modules                                                                                     |
| 5    | Go-side ID generation                                           | N1 (new)  | S to M           | No, but touches 113 models | Cheap and removes a footgun on every insert                                                                                     |
| 6    | `[]string` to `text[]` and array predicates                     | #31       | S to M           | Needed by #27's emitter    | Do with #27                                                                                                                     |
| 7    | Migration coexistence verification and cutover playbook         | N6 (new)  | S spike, M total | Before Go owns DDL         | Verify `CREATE EXTENSION IF NOT EXISTS` as a non-superuser when the extension exists and `baseline` privileges on the CNPG role |
| 8    | Advisory locks and `FOR UPDATE OF`                              | #32       | S                | No                         | Quick win; removes 3 of 12 raw sites                                                                                            |
| 9    | Close out dimensionless vector (#29)                            | #29       | S                | No                         | Confirm tests landed in sprint 5                                                                                                |

Defer to sprint 7: #30 FTS (M), #33 set-returning/window/FILTER/LATERAL (L), N3 JSON ops (S), N4 relation counts (S to M), N5 expression helpers (S to M). All have raw-SQL workarounds and a small call-site count in NovaCRM.

Suggested sprint-6 cut: ranks 1 to 6 plus the S spike from rank 7; take rank 8 and 9 if capacity allows.

## 6. New issues (filed as #35 to #40, in this order)

1. Title: `IDs: Go-side id generators (cuid, uuid, ulid) declared in the model`
   Body: Prisma databases generate `String @id @default(cuid())` ids in the client; NovaCRM has 113 such PKs with no DB default. `FieldHook.Default` never replaces a zero value, so every `Model` insert needs `Omit("id")`.
   Ask: tag `idgen:cuid|uuid|ulid` and `MomoConfig` form, firing when the field is zero; `WithIDGenerator(name, fn)` to plug a generator in.

2. Title: `Codegen: --from-db overrides and re-run without clobbering hand edits`
   Body: Prisma semantics (`updatedAt`, cuid ids, `Json` shapes, `Unsupported` columns) cannot be read from the database, and the Prisma schema keeps changing during a port.
   Ask: a `momo.yaml` `introspect:` section (column rules, type overrides, skip lists) and a re-run mode that updates generated files and reports conflicts with hand edits.

3. Title: `Migrations: externally managed extensions and a cutover playbook for Prisma databases`
   Body: NovaCRM's `CREATE EXTENSION vector` is a manual superuser step and its app role may lack `CREATE` on the database (needed by `baseline`); `db push` keeps changing the schema until cutover.
   Ask: verify and document `CREATE EXTENSION IF NOT EXISTS` as a non-superuser, add an "extension managed outside" setting, document a dual-writer cutover (baseline, drift with unmanaged objects), and ship `migrate new --custom` (story 098).

4. Title: `Query: JSON/JSONB operators (->>, #>>, @>, ?) as typed predicates`
   Body: `types.JSON[T]` maps to jsonb but no operator builder exists (dialect flag `JSON` is metadata only). NovaCRM filters `after->>'via'` in two places and has 54 Json columns.
   Ask: `JSONText`, `JSONPath`, `JSONContains`, `JSONHasKey`, plus docs on SQL NULL vs JSON null for nullable columns.

5. Title: `Relations: load child counts for many parents in one query`
   Body: `Relation.Count` is per parent (N+1) and `CountOrder` only orders. NovaCRM has 22 `_count: { select: ... }` sites.
   Ask: `momo.LoadCounts(ctx, q, parents, rel)` (grouped, chunked under the bind limit) or `Select.WithCount(rel, alias)` into a projection.

6. Title: `Expressions: Extract, AtTimeZone, Interval, Now, DateTrunc, and JoinSQL for raw fragments`
   Body: `Func[T]` covers `date_trunc`, `to_char`, `NULLIF`; `EXTRACT(EPOCH FROM ..)`, `AT TIME ZONE`, interval arithmetic and `now()` still need raw SQL. There is no `Prisma.join` equivalent for composing dynamic WHERE fragments.
   Ask: typed helpers for those and `momo.JoinSQL(sep, parts...) Expr`.

## 7. Accuracy notes

- Counts are line counts from `git grep` on `origin/main` excluding `*.test.ts` and `testing/` unless noted; they are good for ranking, not exact call-site totals. The grep for `cursor:` also matches local variables; the real Prisma cursor use is 2 sites.
- `FOR UPDATE SKIP LOCKED` claim queries were counted by file (documents worker, grantSync, provisioning worker, dispatch); the `SKIP LOCKED` hit total of 15 in the grep includes comments and the test mock.
- The momo verdicts come from `docs/capabilities.md`, `docs/ai-guide.md`, `docs/TYPE_MAPPING.md` and code reads of `fields.go`, `companion.go`, `schema/schema.go`, `gen/schema.go`, `search.go`, `relpred.go`, `expr*.go`, `raw_args.go`. Nothing was compiled or run.
- Not verified: whether `CREATE EXTENSION IF NOT EXISTS` for an installed extension succeeds for a non-superuser (believed yes in PostgreSQL, needs a test); whether a `tsvector` column scans into `string`; whether pgx handles `timestamp(3)` into `time.Time` without a codec (expected yes).
