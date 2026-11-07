# momo gap audit: kuvryn-license SQL vs momo (sprint-4)

Sources read (no builds, no writes to either repo):
- SQL: `/Volumes/DATA/Development/kl-main` (worktree of kuvryn-license `main` @3ab99bf; `/Volumes/DATA/Development/kuvryn-license` is on `sdk-n1`, same data layer). Paths below are relative to `cmd/licensed/`.
- Cross-check: the in-progress port, `/Volumes/DATA/Development/kuvryn-license-momo` (branch `store/momo`). Where it had to fall back to raw SQL it says `RAW-SQL:`; those comments are used as evidence, not assumed.
- momo: `/Volumes/DATA/Development/momo` code, `docs/ai-guide.md` (AG), `docs/capabilities.md` (CAP), `llms.txt`.
- Issues #13 to #17 are open and already being implemented; they are listed and marked "in progress".

## 1. Summary

| Item | Count |
| --- | --- |
| `database/sql` call-site regex matches (`s.ex/qy/qr`, `tx.Exec`, `db.Exec`) | 273 |
| of which real SQL call sites (minus 3 wrapper definitions, 6 `r.URL.Query()` false hits) | 264 |
| call sites with the SQL literal in the call | 242 (SELECT 123, UPDATE 55, INSERT 42, DELETE 19, DDL 3) |
| call sites with built/variable SQL (dynamic WHERE, shared column lists, schema consts) | 22 |
| transactions (`db.Begin`) | 30; no isolation option, no savepoints, 4 `FOR UPDATE` |
| `.sql` files | 0 (DDL is Go string consts in `migrate.go`, `integrations_migrate.go`) |
| DDL: tables / indexes / `ADD COLUMN` steps / FKs / BIGSERIAL | 36 (+`schema_migrations`) / 11 / 28 / 4 / 5 |
| distinct constructs found (A1 to A40 and D1 to D10 in table 2) | 50 |
| covered by a momo API | 35 |
| workaround only (`TypedSQL`/`SQL`, or restructuring in Go) | 12 (A10, A15, A16, A19, A24, A25, A26, A30, A35, A36, A37, A40) |
| GAP (no way inside momo) | 3 (A14 INSERT...SELECT, D5 BIGSERIAL, D7 FK on composite-PK column) |

Of the 15 non-covered rows, 10 map to the in-progress issues: #13 (D7), #14 (D5), #15 (A15, A16), #16 (A14), #17 (A24, A25, A26, A30, A35). Five are not in any open issue: A10 tuple rows, A19 `COALESCE(col, col)`, A36 `CAST`, A37 `||` concat, A40 `lower/upper/length`.
Table 3 adds common Postgres constructs kuvryn does not use (count 0), so the next porter does not rediscover them one at a time.

Port evidence: `kuvryn-license-momo` replaces almost all raw SQL with momo builders. Remaining raw fragments: 6 `RAW-SQL:` markers (substr x2, column arithmetic x2, CASE, current_schema) plus 1 pre-marked INSERT...SELECT removal (`store_catalog.go:424`).
It also had to write its own `rows.go` (`R1`..`R10` tuple rows) which 67 queries use.

## 2. Constructs used by kuvryn-license and momo coverage

Legend: COVERED = public momo API, verified in code. WORKAROUND = only via raw SQL / restructuring. GAP = no way inside momo. `#N` = open issue, in progress.

### 2a. Statements and clauses

| # | Construct | kuvryn usage (count, example) | momo coverage | Notes |
| --- | --- | --- | --- | --- |
| A1 | SELECT with WHERE / ORDER BY (model rows) | ~120; `store_catalog.go:124` | COVERED `Select[T]`.`Where/OrderBy` select.go:82,101,157; AG 6 (l.516-527) | |
| A2 | INSERT one row | ~35; `admin_store.go:295` | COVERED `Insert[T].Model/.Values` insert.go:76,93,113; AG l.567-572 | `Model` writes all columns, `Values` only named |
| A3 | Multi-row VALUES | 1; `store_products.go:34` (seed) | COVERED `Insert.Models/.ValuesRows` insert.go:103,133 | chunked under bind limit, CAP l.84 |
| A4 | UPDATE SET const/param | ~50; `store_console.go:385` | COVERED `Update[T].Set(col.Set(v))` update.go:51, expr.go:120 | no WHERE refused unless `AllRows` |
| A5 | UPDATE SET col = other col / col = col | 3; `signing_keys.go:197`, `store.go:171`, `store_catalog.go:469` | COVERED `col.SetExpr(other.AsExpr())` expr.go:123,60 | port: `signing_keys.go:194`, `store.go:96` |
| A6 | DELETE with WHERE | 19; `provisioning.go:176` | COVERED `Delete[T].Where` delete.go:35,40 | |
| A7 | Row count of write | ~12; `jobs.go:28` | COVERED `Result.RowsAffected` result.go:6 | |
| A8 | INNER JOIN, multi-column ON | 4+; `store_console.go:116` | COVERED `Join(src, on...)` select.go:126; CAP l.90 | |
| A9 | LEFT JOIN, multi-column ON, 3-4 joins in one query | 4 literal + 4 dynamic; `admin_store.go:438`, `store_events.go:61` | COVERED `LeftJoin` select.go:134 | nullable right side must scan into pointers (select.go:128-133) |
| A10 | Reading non-model shapes: tuples, counts, `MAX(x)`, 1-3 column lists | all non-model SELECTs; 67 queries in the port | WORKAROUND. A projection struct per shape with generated `MomoMeta` (AG 4 "Projections" l.310-356). No tuple or scalar select. | Port wrote `rows.go` `R1`..`R10` with hand-built `Meta`. Not covered by an open issue. |
| A11 | Scalar subquery in select list (correlated) | 9 in 3 queries; `admin_store.go:703-707`, `store_products.go:55`, `store_nodes.go:64` area | COVERED `Columns(subquery)`; `SelectQuery` is an `Expr` in parentheses select.go:94; test integration_subquery_test.go:107-109, correlation integration_joins_test.go:107 | port chose grouped queries instead; not a momo limit |
| A12 | `IN (subquery)` | 7; `store_console.go:81`, `webhooks.go:283` | COVERED `col.InQuery(q)` expr.go:110; port `store_console.go:95` | |
| A13 | Correlated `EXISTS` in UPDATE WHERE | 1; `store_catalog.go:481` | COVERED `momo.Exists(subquery)` expr.go:458 inside `Update.Where` | port `store_catalog.go:498` |
| A14 | `WHERE NOT EXISTS` in INSERT...SELECT | 1; `store_catalog.go:428` | GAP #16 (in progress) | port split into `Exists` read + insert (not atomic; `store_catalog.go:424`) |
| A15 | Derived table in JOIN with aliased aggregate column | 1; `store_nodes.go:64` | WORKAROUND, #15 (in progress). `FromSubquery(q, alias)` select.go:862 exists; select list is positional, cannot name `MAX(ts) AS m`. | port rewrote as CTE with a column list: `With(name,q,cols...)` select.go:354, `store_nodes.go:84` |
| A16 | `ORDER BY <alias>` (`... AS day ... ORDER BY day`) | 1; `store.go:521` | WORKAROUND, #15 related: order by the expression, `momo.Asc(expr)` expr.go:241 | port `store.go:437` |
| A17 | Aggregates `COUNT(*)`, `SUM`, `MAX`, `MIN` | 28 / 7 / 4 / 2; `entitlements.go:261` | COVERED `CountAll/Sum/Max/Min` expr.go:542,555,568,565; `Select.Count` select.go:806 | `Sum` is NULL over no rows (expr.go:553) |
| A18 | `COALESCE(agg_or_col, const)` | 11; `entitlements.go:261`, `store.go:191` | COVERED `Coalesce(expr, fallback)` expr.go:571; port `entitlements.go:260` | |
| A19 | `COALESCE(col, other_col)` | 1; `store_console.go:855` (`COALESCE(l.site_name, r.license_id)`) | WORKAROUND: `Coalesce` takes a value, not an expression (expr.go:571). Use `TypedSQL` or do it in Go. | not in an open issue |
| A20 | GROUP BY (incl. expression), no HAVING | 4 (2 literal + 2 dynamic) | COVERED `GroupBy(exprs...)` select.go:144; `Having` select.go:150 (0 uses) | group by a `substr()` needs A24 |
| A21 | `SELECT DISTINCT` | 2; `store_products.go:167` | COVERED `Distinct()` select.go:115 | |
| A22 | ORDER BY col / aggregate, DESC | ~27; `store_console.go:116` (`ORDER BY COUNT(*) DESC`) | COVERED `col.Desc()`, `Agg.Desc()` expr.go:117,539 | `NULLS FIRST/LAST` unused; `Order.NullsFirst` expr.go:214 |
| A23 | LIMIT ? / OFFSET ? (bound) | 5 / 1; `store_events.go:61`, `admin_store.go` audit | COVERED `Limit/Offset` select.go:271,281; keyset `After` select.go:183 | |
| A24 | `substr(col,1,10)` in select/GROUP BY/ORDER BY | 2 statements; `store.go:521`, `store_console.go:81` | WORKAROUND, #17 (in progress). `TypedSQL[string]("substr(...)")` expr.go:602; port `store.go:437` | |
| A25 | `CASE WHEN ... THEN ... ELSE ... END` | 5; `entitlements.go:201`, `store_nodes.go:234`, `store_products.go:107`, `store_v2.go:37,60` | WORKAROUND, #17 (in progress). Port keeps one as `TypedSQL` (`store_v2.go:43`), the rest moved to Go | CASE in `SET` and in `DO UPDATE SET` also needs `SetExpr` |
| A26 | Column arithmetic in SET (`quota_exams = quota_exams + ?`) | 2; `store_console.go:220,311` | WORKAROUND, #17 (in progress). `SetExpr(TypedSQL[int64]("quota_exams + ?", n))` AG l.601; port `store_console.go:199` | no `col.Add(n)`; race-free only through raw expression |
| A27 | `ON CONFLICT (cols) DO NOTHING` / without target | 6 / 2; `provisioning.go:181`, `store.go:433` | COVERED `OnConflict(cols...).DoNothing()` insert.go:179,211 (no columns allowed for DoNothing) | |
| A28 | `ON CONFLICT ... DO UPDATE SET c = excluded.c` | 6; `store_console.go:946`, `store_v2.go:302` | COVERED `DoUpdate(col.SetExcluded())` insert.go:220, expr.go:134 | |
| A29 | `DO UPDATE ... WHERE (existing OR holder)` (lease lock) | 1; `jobs.go:21` | COVERED `InsertQuery.Where(preds)` insert.go:239; AG l.670-678; port `jobs.go:28` | |
| A30 | `DO UPDATE SET c = tbl.c + excluded.c` | 1; `admin_store.go:109` | WORKAROUND, #17 (column arithmetic). `SetExpr(TypedSQL(...))`; port `admin_store.go:126` | `col.Excluded()` expr.go:141 only feeds comparisons |
| A31 | `FOR UPDATE` row lock | 4; `entitlements.go:119`, `store.go:330,375`, `store_v2.go:60` | COVERED `ForUpdate()` select.go:291; CAP l.118; port `entitlements.go:123` | `NOWAIT`, `SKIP LOCKED` accepted, 0 uses |
| A32 | Lock via no-op `UPDATE x SET state = state` | 1; `signing_keys.go:197` | COVERED, same as A5 | |
| A33 | `LIKE ... ESCAPE '\'` on `lower(col)` | 5 in one dynamic query; `admin_store.go:572-596` | COVERED with a semantic change: `ILike` + `EscapeLike` expr.go:472, AG l.534 | no `lower()`/`upper()` builder (A40); PG default escape is `\` so output matches |
| A34 | `IN (...)` list, `<>`, `IS NULL`, `=''` | ~20 | COVERED `In/Ne/IsNull` expr.go:104,75,80 | empty `In` list is a build error, check before call |
| A35 | `information_schema` + `current_schema()` | 1; `migrate.go:98` | WORKAROUND, #17 (in progress). Port: `TableRef{Schema:"information_schema"}` + `TypedSQL("current_schema()")`, `migrate.go:102-107` | only needed by the hand-rolled `addColumn`; disappears with `momo migrate` |
| A36 | `CAST(SUM(x) AS BIGINT)` | 1; `store_console.go:99` | WORKAROUND: no `Cast`; unneeded here (PG `SUM(integer)` is bigint, `Sum[int64]` scans) | |
| A37 | `a \|\| b` string concat | 1; `migrate.go:172` (one-off backfill) | WORKAROUND `TypedSQL`; no concat builder | port dropped it (init migration) |
| A38 | Dynamic WHERE assembly | 6 functions; `admin_store.go:566-620`, `store_console.go:43` | COVERED variadic `Where(preds...)`, builders are immutable values (AG l.510) | |
| A39 | Transactions: `Begin` / `Commit` / `Rollback`, default isolation | 30; `store.go:230` | COVERED `db.Tx(ctx, opts, fn)` db.go:401, `db.Begin` db.go:317; AG 7 l.736-817; CAP l.109 | no `FOR UPDATE` inside needs `tx` as `Querier` |
| A40 | String functions `lower/upper/length` | 5 (all inside A33) | WORKAROUND: no builders; `TypedSQL` | folded into A33 by using `ILike` |


### 2b. DDL in migrations (Go string consts, `migrate.go`, `integrations_migrate.go`)

| # | Construct | kuvryn usage | momo coverage | Notes |
| --- | --- | --- | --- | --- |
| D1 | `TEXT/INTEGER/BOOLEAN NOT NULL` | ~250 columns | COVERED tags + Go types, `type:integer` AG l.217; TYPE_MAPPING.md | `int32` fields map to `integer`; port used `int64` + `type:integer` |
| D2 | `DEFAULT ''`, `DEFAULT FALSE`, `DEFAULT 7` | ~120 | COVERED `default:<sql>` AG l.209 | |
| D3 | Nullable columns (`approved_at TEXT`) | ~12 | COVERED pointer types AG l.181-185 | |
| D4 | Single and composite `PRIMARY KEY` | 36 tables; 13 composite | COVERED multiple `pk` tags AG l.203 | relations by tag need single column |
| D5 | `BIGSERIAL PRIMARY KEY` | 5 tables: `audit_log`, `quota_grant`, `quota_request`, `syncs`, `telemetry` | GAP #14 (in progress) | only `GENERATED BY DEFAULT AS IDENTITY` (AG l.204). Port hand-edited `migrations/20261107042733_init.up.sql` to `bigserial`. |
| D6 | FK to a single-column PK (`leases`, `syncs`) | 2 | COVERED `belongs_to,fk:` AG l.272; `on_delete:` AG l.278 | |
| D7 | FK whose column is part of a composite PK (`exams`, `nodes`) | 2 | GAP #13 (in progress) | port hand-edited the init SQL (`init.up.sql` header l.7-8) |
| D8 | Composite `UNIQUE`, composite non-unique index | 1 / 11 | COVERED `c.Unique`, `c.Index` AG l.228-229 | |
| D9 | `CREATE ... IF NOT EXISTS`, `ADD COLUMN` guarded by `information_schema` | 34 + 28 | COVERED by `momo migrate diff/apply`; existing prod DB: `momo migrate baseline` CAP l.172-184 | the app's own `schema_migrations` runner is replaced by momo's |
| D10 | Runtime migrate on start-up | `OpenStore` | COVERED `Engine.Apply` (CAP l.175); port `migrate.go:80-99` | do not run through PgBouncer transaction pooling (CAP l.167) |

## 3. Constructs kuvryn does not use (count 0), momo status

Verified by grep of `*.go` and docs. These are the likely next "missing basics".

| Construct | momo status | Evidence |
| --- | --- | --- |
| Window functions (`OVER`) | GAP (workaround `TypedSQL`) | CAP l.124; used only inside `LoadPerParentLimit` |
| `FILTER (WHERE ...)` on aggregates | GAP (workaround `TypedSQL`) | grep: no `Filter` builder |
| `DISTINCT ON` | GAP (workaround `TypedSQL` on `Columns`) | grep: none |
| `UPDATE ... FROM` | GAP | `UpdateQuery` has `Set/Where/Model/AllRows/Returning` only (update.go:51-81) |
| `DELETE ... USING` | GAP | delete.go:40-61: no `Using` |
| `INTERSECT` / `EXCEPT` | GAP | only `Union(all, qs...)` select.go:905 |
| `UNION` / `UNION ALL` | COVERED | select.go:905 |
| `LATERAL` | GAP | only internal catalog query |
| `generate_series`, `unnest` | WORKAROUND `TableRef` + `TypedSQL` | no builder |
| `now()`, `current_timestamp`, interval arithmetic | WORKAROUND `TypedSQL`; app time via `WithClock` (CAP l.213) | no `Now()` expression |
| `CAST` / `::type` | GAP (see A36) | |
| `lower/upper/length/substr/concat/trim` | GAP, #17 covers `substr` only | A24, A37, A40 |
| `GREATEST/LEAST/NULLIF` | GAP | grep: none |
| `ANY(array)`, array ops, `@>` | WORKAROUND `SQL` (operators `@>` are protected, raw_args) | `In(slice)` is the portable form |
| JSON operators (`->`, `->>`, `@>`) | WORKAROUND `SQL` | only the `types.JSON[T]` column type, types/json.go |
| `RETURNING` | COVERED | insert.go:159, update.go:75, delete.go:55 |
| `ON CONFLICT ON CONSTRAINT` | GAP | `OnConflict` takes columns only |
| `MERGE` | GAP (raw) | CAP l.106 |
| Locking `SKIP LOCKED` / `NOWAIT` / `FOR SHARE` | COVERED | CAP l.118-120 |
| Recursive CTE, CTE | COVERED | CAP l.121-122 |
| Savepoints, isolation levels, read-only tx | COVERED | AG l.754-762 |
| Advisory locks (app level) | WORKAROUND `momo.Exec`/`Scalars` | only the migration lock exists |
| LISTEN / NOTIFY | GAP | no pgx notification API |
| Sequences (`nextval`, `setval`) | WORKAROUND `momo.Exec`; upsert docs say call `setval` yourself (upsert.go:26) | |
| `COPY` | COVERED at driver level (`CopierOf`), no typed builder | CAP l.107 |
| Prepared statements | n/a (pgx unnamed statements by default, CAP l.50) | |
| Generated columns (`GENERATED ALWAYS AS`) | GAP in tags | `schema.Column.Generated` exists (schema/schema.go:170) but the AG tag table (l.200-219) has no tag |
| Enums, views, triggers, functions, policies, partitions | COVERED via `MomoConfig` AG l.242 | |
| Extensions other than `vector` | not declarable from models (reported as `unmanaged`) | migrations.md l.141 |
| Raw fallbacks | `momo.Exec`, `Scalars`, `SQL`, `TypedSQL`, `RawSubquery` | AG l.703-724 |

## 4. Ranked gap list (call sites blocked, highest first)

Open issues are marked. Proposed APIs follow the existing style (builder methods, typed `Expression[T]`, immutable values).

| Rank | Gap | Sites blocked | Status | Proposed momo API |
| --- | --- | --- | --- | --- |
| 1 | Ad hoc result rows (tuples, aggregates, column lists) without declaring a projection struct per shape | 67 queries in port (A10) | not filed | `momo.Rows2[A,B]`/`R1..R10` generic projections shipped in the core, or `momo.SelectCols(q).From(t).Columns(a, b).All(ctx) ([]Tuple2[A,B], error)` |
| 2 | `CASE WHEN` expression | 5 statements (A25) | #17 in progress | `momo.Case[string]().When(pred, expr).Else(expr)` returning `Expression[T]` usable in `Columns`, `SetExpr`, `DoUpdate` |
| 3 | `BIGSERIAL` / `serial` ids | 5 tables (D5) | #14 in progress | tag `serial` / `bigserial` next to `identity` (emits `bigserial`, sequence owned by column) |
| 4 | Column arithmetic in SET and upsert | 3 statements (A26, A30) | #17 in progress | `col.Add(n)`, `col.Sub(n)`, `col.AddExpr(other.Excluded())` returning `Assignment` |
| 5 | `substr` and string functions | 2 statements (A24; also GROUP BY/ORDER BY) | #17 in progress | `momo.Substr(col, from, n)`, `Lower`, `Upper`, `Concat`, `Length` returning `Expression[string]` |
| 6 | FK on a column that is part of a composite PK | 2 tables (D7) | #13 in progress | `belongs_to,fk:LicenseID` accepted when `LicenseID` is one `pk` of a composite key; no change to the PK |
| 7 | Alias select-list expressions for subqueries/CTEs | 1 join + 1 ORDER BY alias (A15, A16) | #15 in progress | `momo.As(expr, "m")` honoured in `FromSubquery` column naming and `OrderBy(momo.Ref("m"))` |
| 8 | `INSERT ... SELECT` (incl. `WHERE NOT EXISTS`) | 1 (A14) | #16 in progress | `Insert[T](q).Columns(...).FromSelect(subquery).OnConflict(...)` |
| 9 | `COALESCE(col, col)` with an expression fallback | 1 (A19) | not filed | `momo.CoalesceExpr(a, b Expression[T], more ...Expression[T])` |
| 10 | `CAST` | 1 (A36) | not filed | `momo.Cast[T](expr, "bigint")` |
| 11 | `current_schema()` / catalog helpers | 1 (A35) | #17 in progress | `momo.CurrentSchema() Expression[string]` |
| 12 | `||` concat | 1 one-off (A37) | not filed | `momo.Concat(a, b, ...)` (same as rank 5) |

Not blocking kuvryn, but missing basics that will surface in the next port (table 3): `UPDATE ... FROM`, `DELETE ... USING`, `INTERSECT`/`EXCEPT`, window functions, `FILTER`, `DISTINCT ON`, `now()`/interval, `ANY(array)`, `GREATEST/LEAST/NULLIF`, `ON CONFLICT ON CONSTRAINT`, generated-column tag, `LISTEN/NOTIFY`.

## 5. Notes on accuracy

- Counts come from a regex over non-test `.go` files in `cmd/licensed`; strings built by concatenation were read by hand (admin_store.go:309-620-726, store_nodes.go:118, store_console.go:43-59, webhooks.go:154-290). Construct counts for dynamic queries are approximate.
- "COVERED" means the API exists in code and, where noted, is exercised by momo tests or by the port. The port's own `RAW-SQL:` markers confirm A24, A25, A26, A30, A35 independently.
- Soft-delete, optimistic lock, hooks and relations (momo features) are not used by kuvryn-license and not evaluated here.
