# Changelog

## [0.17.0](https://github.com/azrtydxb/momo/compare/v0.16.1...v0.17.0) (2026-11-17)

_Adopting a database built by another tool no longer needs a hand diff: the baseline refusal and `momo migrate drift` name every differing object, and `drift --fix-sql` prints the SQL that repairs it. `drift` on a database without momo history now compares it with a snapshot instead of listing every table as unmanaged, so a script that reads that output sees a different report; see [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.17.0/docs/upgrading.md). [Changes since 0.16.1](https://github.com/azrtydxb/momo/compare/v0.16.1...v0.17.0)._

### Features

- **migrate:** the refusal of `Engine.Baseline` is a `*migrate.BaselineMismatchError`, still matching `ErrBaselineMismatch`, that names each differing object with its live and snapshot definitions. `momo migrate drift` on a database without applied migrations compares it with a snapshot of the migration directory (`--version N`, the latest by default) and names what is missing, changed or unmanaged ([#226](https://github.com/azrtydxb/momo/pull/226), closes [#225](https://github.com/azrtydxb/momo/issues/225))
- **migrate:** `momo migrate drift --fix-sql` and `Engine.DriftRepair` print the statements that bring a live database to that snapshot and apply nothing. Drops, type changes and the steps that depend on them are withheld unless `--allow-destructive` is given; every withheld, unrenderable or staged step is listed on stderr and the command exits 1 ([#226](https://github.com/azrtydxb/momo/pull/226))

### Performance Improvements

- **query:** eight more unexported `SelectQuery[T]` methods move onto non-generic code, and an insert no longer moves the whole query to the heap, three allocations fewer per insert than 0.16.1 ([#224](https://github.com/azrtydxb/momo/pull/224), refs [#217](https://github.com/azrtydxb/momo/issues/217))

## [0.16.1](https://github.com/azrtydxb/momo/compare/v0.16.0...v0.16.1) (2026-11-17)

_Packages that use momo's typed queries compile with about a third less memory: on a fixture of 100 models and 500 queries the Go compiler's peak falls from 9255 MiB to 6189 MiB. No API or behaviour changes; upgrading needs no other step. [Changes since 0.16.0](https://github.com/azrtydxb/momo/compare/v0.16.0...v0.16.1)._

### Performance Improvements

- **query:** the select builders, SQL rendering, compile, count, explain and join expansion, and the insert conflict, identity and upsert-default helpers, move off the generic `SelectQuery[T]` and `InsertQuery[T]` types, so the compiler no longer emits them once per model or row type. The generated SQL is unchanged on every dialect ([#222](https://github.com/azrtydxb/momo/pull/222), refs [#217](https://github.com/azrtydxb/momo/issues/217))

## [0.16.0](https://github.com/azrtydxb/momo/compare/v0.15.0...v0.16.0) (2026-11-16)

_Parallel migration branches merge without conflicts: `momo.sum` has a new format 2 that git merges line by line, and the derived schema folds each migration against its own base, so independent branches validate and dependent ones fail at validate, never at apply. Standalone PostgreSQL sequences, additive CHECK value lists and primary-key updates are now modelled. Upgrade every binary that reads a migration directory together, since an older one refuses a format 2 `momo.sum`; follow [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.16.0/docs/upgrading.md) and [docs/migrations.md](https://github.com/azrtydxb/momo/blob/v0.16.0/docs/migrations.md). [Changes since 0.15.0](https://github.com/azrtydxb/momo/compare/v0.15.0...v0.16.0)._

### ⚠ BREAKING CHANGES

- **migrate:** `momo.sum` is written in format 2, one sorted checksum line per file under a `# momo.sum format=2` header and no `h1:` directory hash; an older momo refuses it with `migrate.ErrNewerFormat`. `momo migrate hash --sum-format 1` writes the old format for a fleet that cannot upgrade at once ([#214](https://github.com/azrtydxb/momo/pull/214), closes [#205](https://github.com/azrtydxb/momo/issues/205))

### Features

- **migrate:** two branches that each add migrations merge without a conflict when their changes are independent. With `momo.sum merge=union` in `.gitattributes` git merges `momo.sum`, and `migrate hash` and `validate` warn when the rule is missing. Branches that touch the same object, a view over a changed table, a foreign key's target, a sequence a default calls, an enum, extension or schema another change uses, or a function, trigger or policy body conflict at `validate` with `ErrBranchConflict` ([#214](https://github.com/azrtydxb/momo/pull/214))
- **migrate:** `momo migrate rebase --renumber [--applied]` and `Engine.Renumber` move a pending branch that sorts before the newest applied version after it, writing the new files and `momo.sum` before removing the old ones; a migration it cannot move safely is named and refused before anything is written ([#214](https://github.com/azrtydxb/momo/pull/214))
- **migrate:** standalone PostgreSQL sequences, such as one a column default calls with `nextval`, are part of the schema model. `c.Sequence(name)` declares one with `Start`, `Increment`, bounds, `Cache` and `Cycle`; `migrate` creates, alters and drops it, `--from-db` writes it, and `drift` reports an undeclared one as `unmanaged: sequence`. SQLite, MySQL and MariaDB refuse it with `ErrUnsupported` ([#212](https://github.com/azrtydxb/momo/pull/212), closes [#207](https://github.com/azrtydxb/momo/issues/207))
- **schema:** `c.CheckIn(&m.Kind, vals...)` declares a CHECK over a list of values that `migrate diff` compares as a set, so adding a value is additive and removing one is flagged destructive by the `check-value-removal` lint rule; `--from-db` writes such constraints as `CheckIn` ([#216](https://github.com/azrtydxb/momo/pull/216), closes [#206](https://github.com/azrtydxb/momo/issues/206))
- **update:** `UpdateQuery.AllowKeyChange()` lets `Set` assign a primary-key column on every dialect; a duplicate key is `ErrUniqueViolation` and an `ON UPDATE CASCADE` child follows the new key. Without it a key assignment is refused as before ([#209](https://github.com/azrtydxb/momo/pull/209), closes [#204](https://github.com/azrtydxb/momo/issues/204))
- **importer:** the GORM, Bun and Ent importers write a composite index of the source model as a `c.Index` declaration, so a converted project no longer drops it on its next migration ([#198](https://github.com/azrtydxb/momo/pull/198))

### Bug Fixes

- **migrate:** the SQL splitter trims only the white space it scans and stops an identifier at a byte order mark, so a statement with non-ASCII white space is no longer split or rejoined differently from how PostgreSQL reads it ([#201](https://github.com/azrtydxb/momo/pull/201))

### Performance Improvements

- **momotest:** `momotest.WithTemplateClone()` builds the migrated schema once per server into a template database and gives each test its own clone, falling back to a schema per test when the role cannot create databases; without the option `momotest.New` is unchanged. `sqlitetest.NewTemplate` does the same for SQLite ([#219](https://github.com/azrtydxb/momo/pull/219), closes [#167](https://github.com/azrtydxb/momo/issues/167))

## [0.15.0](https://github.com/azrtydxb/momo/compare/v0.14.0...v0.15.0) (2026-11-16)

_Full-text search on every dialect: a model declares it once, `momo migrate` renders the structure each database needs, and `fts.Search`, `fts.Score` and `fts.Snippet` query it portably. `momo.HybridSearch` now also runs on SQLite and MariaDB. No code change is needed to upgrade; the cgo SQLite driver needs `-tags sqlite_fts5` for FTS5. See [docs/fulltext.md](https://github.com/azrtydxb/momo/blob/v0.15.0/docs/fulltext.md) and [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.15.0/docs/upgrading.md). [Changes since 0.14.0](https://github.com/azrtydxb/momo/compare/v0.14.0...v0.15.0)._

### Features

- A model declares one full-text index with the `search` tag or `c.FullText(name, cols...)`, chained with `Config`, `Weights`, `Tokenizer`, `Parser`, `Expression` or `Concurrently`; names outside a strict allowlist are refused by `momo generate` ([#166](https://github.com/azrtydxb/momo/pull/166), [#193](https://github.com/azrtydxb/momo/pull/193))
- **migrate:** PostgreSQL 12 or newer renders the declaration as a stored generated `tsvector` column with a GIN index; any change of the declaration recreates the column and index. Declared `Concurrently`, the column and the `CREATE INDEX CONCURRENTLY` go in separate migrations, so a full-text index on an existing table passes `migrate lint` ([#176](https://github.com/azrtydxb/momo/pull/176), [#183](https://github.com/azrtydxb/momo/pull/183))
- **migrate:** SQLite renders an external-content FTS5 table `<table>_fts` with insert, update and delete sync triggers, kept through table rebuilds, renames and dropped source columns. FTS5 needs `-tags sqlite_fts5` on the cgo driver; `driver/sqlitepure` includes it ([#173](https://github.com/azrtydxb/momo/pull/173))
- **migrate:** MySQL and MariaDB render a `FULLTEXT` index, with `WITH PARSER` on MySQL; a unique index, a non-text column, more than 16 columns and a non-InnoDB table are refused ([#171](https://github.com/azrtydxb/momo/pull/171))
- **migrate:** inspection reads the full-text structures back on every dialect, so a second `AutoMigrate` plans nothing and `Drift` and `Baseline` stay clean; `--from-db` emits the declaration. `AutoMigrate` now creates the SQLite sync triggers ([#185](https://github.com/azrtydxb/momo/pull/185))
- **migrate:** `migrate lint` warns on a generated `tsvector` column added to an existing PostgreSQL table (`fulltext-column-rewrite`) and on a `FULLTEXT` index added to an existing MySQL or MariaDB table (`fulltext-first-index`) ([#180](https://github.com/azrtydxb/momo/pull/180))
- **fts:** `fts.Search`, `fts.Score` and `fts.Snippet` over `fts.Web`, `fts.Plain` or `fts.Raw` text run on PostgreSQL and SQLite; the text is always a bound parameter and the score is higher-is-better ([#175](https://github.com/azrtydxb/momo/pull/175))
- **fts:** on MySQL and MariaDB the same calls render boolean-mode `MATCH ... AGAINST` built from parsed words only, so no typed character acts as an operator; `fts.Snippet` is `ErrUnsupported` there, and `fts.MySQLMatch` and `fts.MySQLRelevance` take a mode of your own ([#182](https://github.com/azrtydxb/momo/pull/182))
- **hybrid:** `momo.HybridSearch` runs on SQLite (FTS5 and sqlite-vec) and MariaDB 11.8 with the PostgreSQL fusion knobs and primary-key tie-breaks; MySQL has no vector distance function and fails with `ErrUnsupported` before sending SQL ([#188](https://github.com/azrtydxb/momo/pull/188))

## [0.14.0](https://github.com/azrtydxb/momo/compare/v0.13.0...v0.14.0) (2026-11-15)

_`Merge` and `BulkUpdate` now run on SQLite, MySQL and MariaDB, not only PostgreSQL. No code change is needed to upgrade, but code that relied on `ErrUnsupported` from these calls now takes the success path, and `Merge` on the emulated dialects has documented limits; see [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.14.0/docs/upgrading.md). [Changes since 0.13.0](https://github.com/azrtydxb/momo/compare/v0.13.0...v0.14.0)._

### Features

- `Merge` is emulated on SQLite, MySQL and MariaDB as one atomic sequence (a transaction, or a savepoint inside the caller's `Tx`): the matches and every new value are staged in a temporary table before any write, so the final table and the affected-row count equal PostgreSQL's `MERGE`, assignments that read another assigned column included. A target row matched by two source rows is `ErrTooManyRows` on every dialect, PostgreSQL included. `Returning`, `Rows`, `SET col = DEFAULT`, a model without a primary key and a pinned connection stay `ErrUnsupported` there; a target row committed by another writer mid-sequence is not seen, and the account needs the temporary-table privilege ([#163](https://github.com/azrtydxb/momo/pull/163))
- `BulkUpdate` runs on SQLite, MySQL and MariaDB, and the new `BulkUpdateKeys` matches on a composite key. Long lists are chunked to the dialect's parameter limit and run atomically, with one `updated_at` clock read for all chunks; the affected-row count is the number of matched rows on every dialect. `Returning` stays PostgreSQL and SQLite only ([#160](https://github.com/azrtydxb/momo/pull/160))

## [0.13.0](https://github.com/azrtydxb/momo/compare/v0.12.0...v0.13.0) (2026-11-14)

_More dialect parity: `DistinctOn`, `DELETE ... USING`, mixed `DEFAULT` rows, `DELETE ... RETURNING` and advisory session locks now work beyond PostgreSQL, where the database allows it. No code change is needed to upgrade, but code that relied on `ErrUnsupported` from these calls now takes the success path; see [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.13.0/docs/upgrading.md). [Changes since 0.12.0](https://github.com/azrtydxb/momo/compare/v0.12.0...v0.13.0)._

### Features

- `DistinctOn` runs on SQLite, MySQL and MariaDB through a `ROW_NUMBER()` window, with PostgreSQL's NULL placement written out for nullable sort columns; `ForUpdate` and `ForShare` with `DistinctOn` stay refused there ([#154](https://github.com/azrtydxb/momo/pull/154))
- `Delete(...).Using(...)` runs on SQLite as `DELETE ... WHERE EXISTS (...)`, and a multi-row insert whose rows leave different `dbdefault` columns unset is split into consecutive statements in one transaction or savepoint. A `Using` source named like the delete target is now `ErrValidation` on every dialect; alias it to join a table to itself ([#156](https://github.com/azrtydxb/momo/pull/156))
- `Delete(...).Returning(...)` runs on MariaDB 10.0.5 or later; MySQL keeps `ErrUnsupported` ([#151](https://github.com/azrtydxb/momo/pull/151))
- **advisory:** `Lock`, `TryLock`, `Unlock` and the text-key forms run on MySQL and MariaDB with `GET_LOCK`, scoped to the current database like a PostgreSQL advisory lock; the transaction-scoped and shared forms stay `ErrUnsupported` there ([#153](https://github.com/azrtydxb/momo/pull/153))

## [0.12.0](https://github.com/azrtydxb/momo/compare/v0.11.0...v0.12.0) (2026-11-14)

_The dialect parity release: `momo migrate` and `momo generate --from-db` now run on SQLite, MySQL and MariaDB as well as PostgreSQL, and query features that returned `ErrUnsupported` on some dialects now work on all of them. It also fixes four issues from the NovaCRM and Owl ports. Upgrading needs a code change only where `momo.Avg` is used as an uninstantiated function value, or where `--from-db --schema` is passed on MySQL or MariaDB; follow [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.12.0/docs/upgrading.md). [Changes since 0.11.0](https://github.com/azrtydxb/momo/compare/v0.11.0...v0.12.0)._

### ⚠ BREAKING CHANGES

- **agg:** `momo.Avg` is generic over any numeric column, so code that uses it as an uninstantiated function value must write `momo.Avg[float64]` ([#129](https://github.com/azrtydxb/momo/issues/129))
- **cli:** `momo generate --from-db --schema` is a usage error on MySQL and MariaDB, which ignored the flag before; `--schema` applies to PostgreSQL only ([#141](https://github.com/azrtydxb/momo/pull/141))

### Features

- **cli:** `momo migrate` runs every subcommand on SQLite, MySQL and MariaDB, with dialect-specific column types, URL forms, password redaction and a TLS warning for remote MySQL hosts ([#139](https://github.com/azrtydxb/momo/pull/139))
- **cli:** `momo generate --from-db` reads MySQL and MariaDB databases ([#134](https://github.com/azrtydxb/momo/pull/134)) and SQLite files, opened read-only through the pure-Go driver ([#141](https://github.com/azrtydxb/momo/pull/141))
- **gen:** the `momo.Model` tag options `autoincrement`, `without_rowid` and `strict` declare SQLite table options ([#141](https://github.com/azrtydxb/momo/pull/141))
- **agg:** `momo.SumInt`, `momo.SumNumeric` and `momo.AvgNumeric`, and `momo.Avg` over integer and `types.Numeric` columns ([#129](https://github.com/azrtydxb/momo/issues/129))
- **insert:** `Explicit` writes a caller-chosen value into an identity, serial or auto-increment column ([#127](https://github.com/azrtydxb/momo/issues/127))
- **sqlfn:** `sqlfn.Greatest`, `sqlfn.Least`, `sqlfn.Floor` and `sqlfn.Ceil`, and `jsonop.TypeOf`, on every dialect ([#131](https://github.com/azrtydxb/momo/issues/131))
- `ILike`, `NullsFirst`/`NullsLast` and aggregate `Filter` work on SQLite, MySQL and MariaDB ([#132](https://github.com/azrtydxb/momo/pull/132))
- On MySQL and MariaDB, inserts read generated keys back, `CreateGraph` and `FindOrCreate` work, and `Upsert` can target a secondary unique key ([#135](https://github.com/azrtydxb/momo/pull/135))

### Bug Fixes

- **fromdb:** `--check` compares the generated model body, not only the header hash, so a hand-edited model whose body equals what the database regenerates is current instead of stale, and a plain run only refreshes its header ([#128](https://github.com/azrtydxb/momo/issues/128))
- **jsonop:** `Contains` on MariaDB sent `CAST(? AS JSON)`, a syntax error there; it now passes the candidate as JSON text ([#142](https://github.com/azrtydxb/momo/pull/142))
- **timeop:** `AddInterval` and `SubInterval` over a bound time on MariaDB returned text the driver could not read as a time ([#142](https://github.com/azrtydxb/momo/pull/142))

## [0.11.0](https://github.com/azrtydxb/momo/compare/v0.10.0...v0.11.0) (2026-11-13)

_JSON null and SQL NULL are now kept apart, `updated_at` behaves the same on every write path, and `timeop` gains typed date arithmetic. It fixes three issues from the NovaCRM and Owl ports. Upgrading can need code changes where a JSON null was read as nil on PostgreSQL, or where a `DoUpdate` set `updated_at` explicitly; follow [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.11.0/docs/upgrading.md). [Changes since 0.10.0](https://github.com/azrtydxb/momo/compare/v0.10.0...v0.11.0)._

### ⚠ BREAKING CHANGES

- **types:** on PostgreSQL, a stored JSON `null` in a nullable column reads as a non-nil `*types.JSON[T]` with `IsNull()` true instead of a nil pointer; code that tested for nil must also test `IsNull()`. `types.JSON[T]` has a private field, so unkeyed literals such as `types.JSON[T]{v}` no longer compile ([#118](https://github.com/azrtydxb/momo/issues/118))
- **upsert:** an explicit `updated_at` in `DoUpdate` (a value, an expression or `SetExcluded`) is refused with `ErrValidation`, on `Upsert` and on `Insert().OnConflict().DoUpdate()` alike ([#120](https://github.com/azrtydxb/momo/issues/120))
- **update:** an `Update` with no assignments on a model with `updated_at`, including `Update.Model` with only nil `dbdefault` pointers, touches `updated_at` instead of failing with `ErrValidation` ([#120](https://github.com/azrtydxb/momo/issues/120))

### Features

- **types:** `JSON[T].IsNull`, `JSON[T].Set` and `types.JSONNull[T]()` tell a JSON null apart from SQL NULL, and a value read as JSON null is written back as JSON null on every dialect ([#118](https://github.com/azrtydxb/momo/issues/118))
- **timeop:** `timeop.AddDays`, `timeop.AddDaysExpr` and `timeop.DaysBetween` do date arithmetic on `types.Date` with typed results on PostgreSQL, MySQL, MariaDB and SQLite ([#119](https://github.com/azrtydxb/momo/issues/119))
- **update:** an empty `Update` on a model with `updated_at` is a touch, like Prisma's `update({ data: {} })` ([#120](https://github.com/azrtydxb/momo/issues/120))

### Bug Fixes

- **upsert:** the conflict branch of `Upsert` copied the struct's own `UpdatedAt`, so upserting a loaded record wrote its old timestamp back; every `DoUpdate` now sets `updated_at` to the statement clock, also with `Omit(updated_at)`, a `FromSelect` without the column, or an empty default set ([#120](https://github.com/azrtydxb/momo/issues/120))
- **insert:** a plain `Insert().OnConflict().DoUpdate()` now bumps `updated_at` like `Upsert` ([#120](https://github.com/azrtydxb/momo/issues/120))

## [0.10.0](https://github.com/azrtydxb/momo/compare/v0.9.0...v0.10.0) (2026-11-13)

_`momotest` now always migrates into the per-test schema, even when `public` of the same database is already migrated, so tests no longer run on (or drop) the real tables. Upgrading needs a change only where `postgres.New` is used as a function value; see [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.10.0/docs/upgrading.md). [Changes since 0.9.0](https://github.com/azrtydxb/momo/compare/v0.9.0...v0.10.0)._

### Features

- **migrate/postgres:** `postgres.WithHistorySchema` pins the `momo_migrations` history table to a schema instead of resolving it through the search path; `postgres.New` takes options ([#115](https://github.com/azrtydxb/momo/issues/115))

### Bug Fixes

- **momotest:** `WithMigrations` migrated nothing into the test schema when `public` already held `momo_migrations`, so tests ran on the real tables; the history table now lives in the test schema ([#115](https://github.com/azrtydxb/momo/issues/115))
- **momotest:** migrations run with only the test schema on the search path (plus `public` when `WithExtensions` lists extensions), so `DROP TABLE IF EXISTS` or `ALTER TABLE IF EXISTS` in a migration can no longer reach a table in `public` ([#115](https://github.com/azrtydxb/momo/issues/115))
- **momotest:** `New` fails with a clear error when migrations that create tables leave the test schema empty ([#115](https://github.com/azrtydxb/momo/issues/115))

## [0.9.0](https://github.com/azrtydxb/momo/compare/v0.8.0...v0.9.0) (2026-11-13)

_The `sqlfn` arithmetic helpers now accept `types.Numeric`, so a PostgreSQL `numeric` column generated by 0.8.0 can be updated with exact arithmetic without raw SQL. No code change is needed to upgrade; see [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.9.0/docs/upgrading.md). [Changes since 0.8.0](https://github.com/azrtydxb/momo/compare/v0.8.0...v0.9.0)._

### Features

- **sqlfn:** `Add`, `AddExpr`, `Sub`, `SubExpr`, `Mul`, `MulExpr`, `Div`, `DivExpr` and `NullIf` accept `types.Numeric` on PostgreSQL, where the value binds as an exact decimal. SQLite and MySQL store `types.Numeric` as TEXT, so these calls fail there with `ErrUnsupported` instead of losing exactness ([#114](https://github.com/azrtydxb/momo/issues/114))
- **types:** `types.NumericFromInt64` and `types.NumericFromFloat64` build a `types.Numeric` from a Go number ([#114](https://github.com/azrtydxb/momo/issues/114))

## [0.8.0](https://github.com/azrtydxb/momo/compare/v0.7.0...v0.8.0) (2026-11-12)

_This release fixes 26 issues found while porting Kuvryn Orbit and Owl to 0.7: query builder gaps, `--from-db` output, migration inspection, and new PostgreSQL helpers. Upgrading may need code changes, mostly where generated names change; follow [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.8.0/docs/upgrading.md). [Changes since 0.7.0](https://github.com/azrtydxb/momo/compare/v0.7.0...v0.8.0)._

### ⚠ BREAKING CHANGES

- **naming:** `ToSnake` keeps a plural initialism together, so `RoleIDs` maps to `role_ids` instead of `role_i_ds`. An untagged field or model name with a plural initialism gets a new column or table name. A migration diff drops the old column and adds the new one; pin the old name with a `column:` tag or use `--rename-column` ([#107](https://github.com/azrtydxb/momo/issues/107))
- **fromdb:** Go field names keep more initialisms in capitals, for example `TeamIds` becomes `TeamIDs` ([#107](https://github.com/azrtydxb/momo/issues/107))
- **fromdb:** singular table names ending in `-ss`, `-us` and some `-is`, `-as` and `-ns` words keep their last letter, for example `class` gives `Class` instead of `Clas` ([#100](https://github.com/azrtydxb/momo/issues/100))
- **fromdb:** a PostgreSQL `numeric` column is generated as `types.Numeric` instead of `string` ([#105](https://github.com/azrtydxb/momo/issues/105))
- **fromdb:** a NOT NULL column with a non-zero constant default is generated as a `dbdefault` pointer field ([#90](https://github.com/azrtydxb/momo/issues/90))
- **fromdb:** the policies and triggers of a skipped or unmodeled table are no longer written onto the first model ([#108](https://github.com/azrtydxb/momo/issues/108))
- **migrate:** an index attached to a parent partitioned index is no longer inspected as a partition index, so a 0.7.0 snapshot reports it missing until the models and snapshot are regenerated ([#86](https://github.com/azrtydxb/momo/issues/86))
- **migrate:** VIRTUAL generated columns are inspected as VIRTUAL on MySQL and SQLite, and a change between STORED and VIRTUAL needs a custom migration on MySQL and PostgreSQL ([#111](https://github.com/azrtydxb/momo/issues/111))
- **query:** `UpdateQuery.From` and `DeleteQuery.Using` are variadic, which breaks method values and method expressions of them ([#93](https://github.com/azrtydxb/momo/issues/93))
- **types:** `types.Date` has new fields, so a positional `types.Date{t}` literal no longer compiles ([#105](https://github.com/azrtydxb/momo/issues/105))

### Features

- **query:** `UpdateQuery.From` and `DeleteQuery.Using` take several sources, and `Returning` reads their columns, qualified ([#93](https://github.com/azrtydxb/momo/issues/93))
- **query:** `momo.SelectUnion` runs a union at the top level, and union members keep their own `ORDER BY` and `LIMIT` ([#99](https://github.com/azrtydxb/momo/issues/99))
- **query:** `momo.UnnestColumn` unnests an array column with LATERAL, and `agg` aggregates get `Distinct` ([#98](https://github.com/azrtydxb/momo/issues/98))
- **query:** `momo.OrderByPredicate` orders by a boolean condition ([#110](https://github.com/azrtydxb/momo/issues/110))
- **query:** `NullColumn.SetPtr` and `EqPtr` take a nil-able value ([#109](https://github.com/azrtydxb/momo/issues/109))
- **query:** nested `Has` and `HasNo` get one alias per level, and `momo.RelAlias` names an outer level ([#89](https://github.com/azrtydxb/momo/issues/89))
- **expressions:** row-value comparison with `momo.Row`, `sqlfn.Div` and `sqlfn.NullIf`, `sqlfn.CurrentDate`, infinity literals, the inline literal `momo.Lit`, and joins, `GroupBy`, `Having` and `OrderBy` on `SelectExprs` ([#97](https://github.com/azrtydxb/momo/issues/97))
- **jsonop:** `Root`, `DeleteKey`, `Concat` and `Set`, plus the `momo.ArrayOf` and `momo.ArraySubquery` array constructors ([#96](https://github.com/azrtydxb/momo/issues/96))
- **raw:** `momo.RawRows` returns the hooked cursor of a raw statement ([#104](https://github.com/azrtydxb/momo/issues/104))
- **raw:** `momo.WithNaming` makes a plain-struct `Query[T]` follow the naming preset ([#92](https://github.com/azrtydxb/momo/issues/92))
- **pgxdriver:** `pgxdriver.Listen` and `pgxdriver.Notify` for LISTEN/NOTIFY on a dedicated connection ([#102](https://github.com/azrtydxb/momo/issues/102))
- **advisory:** shared advisory locks ([#94](https://github.com/azrtydxb/momo/issues/94))
- **timeop:** sub-second intervals, `time.Duration` intervals and timestamp difference ([#95](https://github.com/azrtydxb/momo/issues/95))
- **types:** `types.Date` infinity and the new `types.Numeric` ([#105](https://github.com/azrtydxb/momo/issues/105))
- **dbdefault:** `dbdefault` on a pointer field of a NOT NULL column writes `false` and `0` ([#90](https://github.com/azrtydxb/momo/issues/90))
- **fromdb:** `introspect.inverse: true` writes the inverse `has_many` and `has_one` ([#91](https://github.com/azrtydxb/momo/issues/91))
- **fromdb:** stored generated columns become read-only `generated:` fields, and a `virtual` tag declares a VIRTUAL one ([#103](https://github.com/azrtydxb/momo/issues/103), [#111](https://github.com/azrtydxb/momo/issues/111))
- **migrate:** `migrate.ConfigFromFile` reads the `migrate` section of `momo.yaml` as library options, and the `State` constants are typed ([#101](https://github.com/azrtydxb/momo/issues/101))
- **momovet:** a `//momovet:ok <reason>` directive marks a raw-SQL call as vetted ([#106](https://github.com/azrtydxb/momo/issues/106))

### Bug Fixes

- **importer/history:** `Baseline` replays transactional migrations in a transaction and rolls back a failed commit ([#87](https://github.com/azrtydxb/momo/issues/87))
- **importer/history:** `WriteImport` marks its output with `-- momo:imported`, so `migrate validate` accepts it ([#88](https://github.com/azrtydxb/momo/issues/88))

## [0.7.0](https://github.com/azrtydxb/momo/compare/v0.6.0...v0.7.0) (2026-11-11)

_`momo generate --from-db` can be limited to a list of tables, writes only the functions those tables use, and names `-ses` tables correctly. `Query` and `QueryRows` read into a plain struct, and writes with `RETURNING` get `One`. Upgrading may need code changes: follow [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.7.0/docs/upgrading.md). [Changes since 0.6.0](https://github.com/azrtydxb/momo/compare/v0.6.0...v0.7.0)._

### ⚠ BREAKING CHANGES

- **query:** `Query` and `QueryRows` lose their `PT` type parameter, so explicit `Query[T, *T]` calls must drop it. A struct without generated metadata now compiles and is scanned by reflection ([#75](https://github.com/azrtydxb/momo/issues/75), [#83](https://github.com/azrtydxb/momo/pull/83))
- **fromdb:** `--from-db` writes only the functions the modeled tables use through their triggers, defaults, checks or policies. Set `introspect.functions: all` for the old output ([#74](https://github.com/azrtydxb/momo/issues/74), [#83](https://github.com/azrtydxb/momo/pull/83))
- **fromdb:** model names for tables ending in `-ses` change, for example `releases` gives `Release` instead of `Releas`. Pin the old name with `introspect.tables.<name>.name` ([#82](https://github.com/azrtydxb/momo/issues/82), [#83](https://github.com/azrtydxb/momo/pull/83))
- **fromdb:** an `introspect.skip` glob written as `schema.name` now takes effect ([#73](https://github.com/azrtydxb/momo/issues/73), [#83](https://github.com/azrtydxb/momo/pull/83))

### Features

- **fromdb:** `introspect.only` limits generation to a list of table globs. A glob that matches nothing is warned about, and an `only` list that matches no table fails the run ([#73](https://github.com/azrtydxb/momo/issues/73), [#83](https://github.com/azrtydxb/momo/pull/83))
- **fromdb:** `introspect.dbdefault: all` tags every NOT NULL column that has a default, not only computed ones ([#77](https://github.com/azrtydxb/momo/issues/77), [#83](https://github.com/azrtydxb/momo/pull/83))
- **fromdb:** `introspect.uuid: google` generates `github.com/google/uuid` types ([#76](https://github.com/azrtydxb/momo/issues/76), [#83](https://github.com/azrtydxb/momo/pull/83))
- **types:** `uuid.UUID`, `*uuid.UUID` and `[]uuid.UUID` from `github.com/google/uuid` map to `uuid` and `uuid[]` without a type tag, and array operators cast them ([#76](https://github.com/azrtydxb/momo/issues/76), [#83](https://github.com/azrtydxb/momo/pull/83))
- **query:** `InsertQuery`, `UpdateQuery` and `DeleteQuery` get `One`, which returns the single `RETURNING` row ([#78](https://github.com/azrtydxb/momo/issues/78), [#83](https://github.com/azrtydxb/momo/pull/83))
- **migrate:** `Config.OnlyTables` and `InspectOptions.Only` limit inspection and diff to a list of tables, and `schema.MatchObject` is the shared glob rule that `migrate.MatchObject` calls ([#83](https://github.com/azrtydxb/momo/pull/83))

### Bug Fixes

- **cli:** `momo generate -h` lists every `introspect` key ([#83](https://github.com/azrtydxb/momo/pull/83))
- **docs:** the registry docs say the SQL of a registered `Insert.Model` depends on the model's values, and how to pin it ([#79](https://github.com/azrtydxb/momo/issues/79), [#83](https://github.com/azrtydxb/momo/pull/83))
- **migrate:** the `Lock` doc comment says a discarded session's lock is released asynchronously, and the test that flaked on it now waits ([#83](https://github.com/azrtydxb/momo/pull/83))

## [0.6.0](https://github.com/azrtydxb/momo/compare/v0.5.0...v0.6.0) (2026-11-10)

_`momo generate` can now generate OpenAPI, gRPC and GraphQL servers over your models, and view models are read-only end to end. The guard on `UPDATE ... FROM` and `DELETE ... USING` is stricter. Upgrading may need code changes: follow [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.6.0/docs/upgrading.md). [Changes since 0.5.0](https://github.com/azrtydxb/momo/compare/v0.5.0...v0.6.0)._

### ⚠ BREAKING CHANGES

- **query:** `UpdateQuery.From` and `DeleteQuery.Using` refuse more statements with `ErrUnfilteredMutation`. The guard now follows `Or`, `Not` and `JoinSQL`: an `Or` branch, or a negated `And`, that does not reference the target table no longer passes ([#80](https://github.com/azrtydxb/momo/pull/80))
- **gen:** a model that embeds `momo.View` no longer produces a table, so `momo migrate diff` plans a `DROP TABLE` if a table of that name exists ([#80](https://github.com/azrtydxb/momo/pull/80))
- **openapi:** generated error bodies use the key `error`, and a create or PUT request body uses the new `<Model>Input` schema ([#80](https://github.com/azrtydxb/momo/pull/80))
- **gen:** a cancelled request to a generated server answers 499, `Canceled` or `CANCELED` instead of 500 or `Internal`, and a request past its deadline answers 504 or `DeadlineExceeded` ([#80](https://github.com/azrtydxb/momo/pull/80))

### Features

- **gen:** built-in `openapi`, `grpc` and `graphql` plugins, enabled in the `plugins` section of `momo.yaml`. They generate an OpenAPI 3.1 document with `net/http` handlers, a `.proto` file with a gRPC server, and gqlgen glue with batched relation loading ([#80](https://github.com/azrtydxb/momo/pull/80))
- **gen:** the `openapi` and `grpc` plugins take `ops`, `read_only` and an `on_error` hook. Generated servers map errors to fixed messages, and GraphQL limits depth, complexity, preloaded rows and body size ([#80](https://github.com/azrtydxb/momo/pull/80))
- **views:** `momo.RefreshMaterializedView` (PostgreSQL), and generated servers expose only list and get for a view model ([#80](https://github.com/azrtydxb/momo/pull/80))
- **docs:** the `cli` package is in the API reference, and a release also publishes `llms.txt` and `llms-full.txt` at the site root ([#80](https://github.com/azrtydxb/momo/pull/80))

### Bug Fixes

- **gen:** the plugin generators find a view model's primary key, and a view model that shares a table name with another model is reported ([#80](https://github.com/azrtydxb/momo/pull/80))
- **deps:** modernc.org/sqlite 1.61.1, google.golang.org/grpc 1.85.0, gqlgen 0.17.98 ([#47](https://github.com/azrtydxb/momo/pull/47), [#48](https://github.com/azrtydxb/momo/pull/48), [#49](https://github.com/azrtydxb/momo/pull/49))

## [0.5.0](https://github.com/azrtydxb/momo/compare/v0.4.0...v0.5.0) (2026-11-10)

_Momo now carries everything Kuvryn Orbit needs to move onto it: composite foreign keys, DML in CTEs, raw multi-column queries, database-defaulted columns and more row-lock modes. It also adds hybrid search, EXPLAIN and slow-query tooling, OpenTelemetry in the new `otelmomo` module, and a `cli` package for building your own `momo` binary. Upgrading needs a regenerate and one advisory call-site check: follow [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.5.0/docs/upgrading.md). [Changes since 0.4.0](https://github.com/azrtydxb/momo/compare/v0.4.0...v0.5.0)._

### ⚠ BREAKING CHANGES

- **gen:** `momo generate --from-db` adds the `dbdefault` tag to NOT NULL PostgreSQL columns with a computed default. Regenerated models stop writing Go zero values to those columns and read the database value back with `RETURNING` ([#69](https://github.com/azrtydxb/momo/issues/69))
- **gen:** `momo generate` wires methods named `BeforeInsert`, `BeforeUpdate`, `BeforeDelete` and `AfterLoad` into `Meta.Hooks`. A hook-named method with another signature now fails generation ([#70](https://github.com/azrtydxb/momo/pull/70))
- **advisory:** `advisory.XactLockString` and `advisory.TryXactLockString` take a trailing `...advisory.Option`, so assigning them to a variable of the old func type no longer compiles ([#67](https://github.com/azrtydxb/momo/issues/67))
- **cli:** the command line moved from `cmd/momo` into package `cli`. `go install github.com/azrtydxb/momo/cmd/momo@v0.5.0` still works ([#70](https://github.com/azrtydxb/momo/pull/70))

### Features

- **schema:** composite foreign keys through `Config.ForeignKey` with `ON DELETE` and `ON UPDATE` actions, unique expression indexes, `identity:always`, named foreign keys (`fk_name`, `Config.FKName`) and an explicit primary key column order (`Config.PrimaryKey`) ([#54](https://github.com/azrtydxb/momo/issues/54), [#55](https://github.com/azrtydxb/momo/issues/55), [#56](https://github.com/azrtydxb/momo/issues/56), [#59](https://github.com/azrtydxb/momo/issues/59), [#60](https://github.com/azrtydxb/momo/issues/60))
- **types:** `[]types.UUID` maps to `uuid[]` and works with the array operators; `tsvector` maps to `types.TSVector` in `--from-db` ([#57](https://github.com/azrtydxb/momo/issues/57), [#61](https://github.com/azrtydxb/momo/issues/61))
- **query:** `UpdateQuery.From`, `DeleteQuery.Using`, and `With` / `WithRecursive` on INSERT, UPDATE and DELETE, so a DML statement can be a CTE body on PostgreSQL ([#64](https://github.com/azrtydxb/momo/issues/64))
- **query:** raw multi-column queries with `momo.Query` and `momo.QueryRows`, and a query registry for listing and freezing SQL: `momo.RegisterQuery`, `momotest.QueryGolden` ([#63](https://github.com/azrtydxb/momo/issues/63), [#66](https://github.com/azrtydxb/momo/issues/66))
- **query:** `ForNoKeyUpdate` and `ForKeyShare` row locks, session advisory locks, `advisory.Hash64`, `timeop.ClockTimestamp` and `timeop.StatementTimestamp` ([#65](https://github.com/azrtydxb/momo/issues/65), [#67](https://github.com/azrtydxb/momo/issues/67), [#68](https://github.com/azrtydxb/momo/issues/68))
- **insert:** the `dbdefault` tag leaves a zero-valued column to its database default and reads the value back ([#69](https://github.com/azrtydxb/momo/issues/69))
- **vectors:** `momo.HybridSearch` combines full-text and vector ranking ([#70](https://github.com/azrtydxb/momo/pull/70))
- **observability:** `momo.Explain`, `momo.ExplainAnalyze`, a slow-query hook (`obs.NewSlowQuery`), and OpenTelemetry spans and a duration histogram in the separately released `otelmomo` module ([#70](https://github.com/azrtydxb/momo/pull/70))
- **momotest:** `momotest.AssertSQL` and `momotest.AssertUsesIndex` ([#70](https://github.com/azrtydxb/momo/pull/70))
- **migrate:** `momo migrate lint`, `rollback`, `rebase` and `drift --strict`, and history import from Atlas, goose, golang-migrate and Bun ([#70](https://github.com/azrtydxb/momo/pull/70))
- **cli:** build a custom `momo` binary with generator plugins through `cli.Main` ([#70](https://github.com/azrtydxb/momo/pull/70))
- **models:** view models: embed `momo.View`, and every write path fails with `ErrValidation` ([#70](https://github.com/azrtydxb/momo/pull/70))

### Bug Fixes

- **gen:** trigger `UPDATE OF` keeps its written column order, and `momo generate` no longer fails on its own stale `momo_gen.go` ([#58](https://github.com/azrtydxb/momo/issues/58), [#62](https://github.com/azrtydxb/momo/issues/62))
- **sqlite:** a NaN float is rejected with `ErrValidation` instead of being stored ([#70](https://github.com/azrtydxb/momo/pull/70))

## [0.4.0](https://github.com/azrtydxb/momo/compare/v0.3.0...v0.4.0) (2026-11-10)

_Momo now runs on MySQL, MariaDB and SQLite as well as PostgreSQL, and the root package is split into feature subpackages whose docs are checked against the code. Upgrading needs import changes: follow [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.4.0/docs/upgrading.md). [Changes since 0.3.0](https://github.com/azrtydxb/momo/compare/v0.3.0...v0.4.0)._

### ⚠ BREAKING CHANGES

- **packages:** feature APIs moved out of the root `momo` package into `arrayop`, `fts`, `sqlfn`, `jsonop`, `agg`, `timeop`, `advisory`, `vecsearch`, `vecop` and `obs`; advisory locks are free functions taking `*momo.Tx`. `docs/upgrading.md` lists every moved identifier with its replacement and a rewrite recipe ([#52](https://github.com/azrtydxb/momo/issues/52))
- **core:** `Error()` no longer includes raw server text, the simple protocol is refused, and typed `In` is capped at the bind limit ([#46](https://github.com/azrtydxb/momo/issues/46))
- **gen:** generated files must be regenerated once with `momo generate` after upgrading; `generate --check` reports the header change until then. The exported constant `gen.Version` is removed ([#24](https://github.com/azrtydxb/momo/issues/24))

### Features

- **drivers:** `driver/mysqldriver` for MySQL 8.4, 9.7 and MariaDB 11.8; `driver/sqlitepure`, a pure-Go SQLite driver without cgo; sqlite-vec typed vectors on the cgo SQLite driver ([#46](https://github.com/azrtydxb/momo/issues/46))
- **migrate:** `migrate/mysql` and `migrate/sqlite` (rebuild-based ALTER) with AutoMigrate, Go migrations recorded in history, externally managed extensions and role verification ([#46](https://github.com/azrtydxb/momo/issues/46))
- **importer:** read GORM, Bun and Ent models into momo schemas ([#46](https://github.com/azrtydxb/momo/issues/46))
- **query:** full-text search, array operators, typed JSON operators, date/time expressions, unnest sources, window functions, FILTER, DISTINCT ON, LATERAL, ordered aggregates, advisory transaction locks, FOR UPDATE OF, `LoadCounts` for many parents and PostgreSQL `$n` in raw SQL ([#27](https://github.com/azrtydxb/momo/issues/27) to [#45](https://github.com/azrtydxb/momo/issues/45))
- **write:** `Copy`, `Merge` (PostgreSQL 15+), `BulkUpdate`, `Truncate`, `InsertQuery.Unnest`, and Go-side id generators (cuid, uuid, ulid) ([#46](https://github.com/azrtydxb/momo/issues/46))
- **tx:** a per-transaction setup hook, implicit transactions for RLS tenant scoping, and read replicas ([#46](https://github.com/azrtydxb/momo/issues/46))
- **momotest:** a public test kit for applications built on momo ([#46](https://github.com/azrtydxb/momo/issues/46))
- **docs:** generated capability matrix, type-mapping table and API reference; every backticked identifier in the docs must resolve, and an exported API change needs a docs change ([#52](https://github.com/azrtydxb/momo/issues/52))

### Security

- encode errors are redacted and sensitive arguments masked, LIKE uses an explicit escape, the migration history table is schema-qualified and its owner checked ([#46](https://github.com/azrtydxb/momo/issues/46))

## 0.3.0 (2026-11-07)

_Momo now carries a real PostgreSQL application end to end: kuvryn-license runs fully on it, its queries all on the typed API. Supported backend: PostgreSQL with pgvector (pgx driver). [Changes since 0.2.0](https://github.com/azrtydxb/momo/compare/v0.2.0...v0.3.0)._

### ⚠ BREAKING CHANGES

- **expr:** `momo.As` is generic and takes a typed expression; `Aliased` is `Aliased[T]` with unexported fields. Wrap a bare SQL fragment in `TypedSQL` ([#21](https://github.com/azrtydxb/momo/issues/21))
- **expr:** `Column[T].Excluded` returns `ExcludedExpr[T]`, which still implements `Expression[T]` ([#21](https://github.com/azrtydxb/momo/issues/21))
- **migrate:** the PostgreSQL inspector reports serial columns as identity `serial` instead of `by_default`; a model that declares `pk,identity` for a BIGSERIAL column must switch to the `serial` tag (see the upgrade note in docs/migrations.md) ([#14](https://github.com/azrtydxb/momo/issues/14))
- **migrate:** `ApplyOptions.ForceUnlock` and `momo migrate apply --force-unlock` are removed; they were a no-op on every dialect ([#12](https://github.com/azrtydxb/momo/issues/12))

### Features

- **pgxdriver:** run momo inside a transaction the caller already opened, and hand legacy code the transaction behind a momo one: `AdoptTx`, `SQLTx`, `PgxTx`, `UnwrapSQLTx`, `UnwrapPgxTx` ([#11](https://github.com/azrtydxb/momo/issues/11))
- **gen:** foreign keys on a column that is part of a composite primary key ([#13](https://github.com/azrtydxb/momo/issues/13))
- **schema:** `serial` tag for smallserial, serial and bigserial ids, recognised by the inspector ([#14](https://github.com/azrtydxb/momo/issues/14))
- **query:** select-list aliases usable from subqueries and CTEs ([#15](https://github.com/azrtydxb/momo/issues/15)), and tuple result rows `Row1` to `Row10` with no per-shape projection struct ([#19](https://github.com/azrtydxb/momo/issues/19))
- **insert:** `INSERT ... SELECT` through `Insert[T].FromSelect`, including FROM-less selects guarded by `WHERE NOT EXISTS` ([#16](https://github.com/azrtydxb/momo/issues/16))
- **expr:** typed `Substr`, `Case`, column arithmetic in SET and upserts, `CurrentSchema`, `Func`, `CoalesceExpr`, `Cast`, `Lower`, `Upper`, `Length` and `Concat`, usable in `Where` and `OrderBy` ([#17](https://github.com/azrtydxb/momo/issues/17), [#20](https://github.com/azrtydxb/momo/issues/20))
- **migrate:** `--enforce-not-null` for staged columns, concurrent indexes split into a no-transaction migration, lock-safe UNIQUE and CHECK, chain and order checks on apply, `momo migrate rehash`, URL redaction ([#12](https://github.com/azrtydxb/momo/issues/12))

## 0.2.0 (2026-11-06)

Conditional upserts, the AI coder guide and runnable examples. Supported backend: PostgreSQL with pgvector (pgx driver). [Changes since 0.1.0](https://github.com/azrtydxb/momo/compare/v0.1.0...v0.2.0).

### Features

- **insert:** WHERE on ON CONFLICT target and DO UPDATE ([#9](https://github.com/azrtydxb/momo/issues/9)) ([ffae95a](https://github.com/azrtydxb/momo/commit/ffae95a6c8eed9e551758d7f5fa41181af9aa00f))

### Documentation

- Guide for AI coding agents (`docs/ai-guide.md`, `llms.txt`), capability matrix, supported versions, migration safety and recovery guide, and nine runnable examples tested in CI ([#7](https://github.com/azrtydxb/momo/issues/7))

### Bug Fixes

- **schema:** keep `E'…\'…'` escape-string literals intact when normalising expressions for migration diffs ([#9](https://github.com/azrtydxb/momo/issues/9))

## 0.1.0 (2026-11-06)

First release. Supported backend: PostgreSQL with pgvector (pgx driver), including PgBouncer. SQLite and MySQL are not usable yet; only their dialect metadata exists.

### Features

- **dialect:** add MySQL dialect metadata (flavours and capability set); the MySQL backend is not usable yet ([d19c341](https://github.com/azrtydxb/momo/commit/d19c34165c2d286c969d431430c9eb65e6f49ed6))
- **errors:** add the sentinel set, Detail accessor and vector and migrate sentinels ([da81d21](https://github.com/azrtydxb/momo/commit/da81d211f24383c4a2e8f275751bf75ee8cdb6fd))
- **momo:** keyset pagination with deterministic tie-breaking ([0421baf](https://github.com/azrtydxb/momo/commit/0421bafaa7902e44d2493d0f4d0fad7f42d4716c))
- **relations:** eager loading, predicates, associations, graph writes ([#5](https://github.com/azrtydxb/momo/issues/5)) ([c744711](https://github.com/azrtydxb/momo/commit/c74471152e515379901eb92213513c2c44e50208))
- **schema:** base snapshot types with format_version and JSON round-trip ([81950d4](https://github.com/azrtydxb/momo/commit/81950d40076f41766c68293a3886caf1847fb971))
- **schema:** normalize types and expressions and compare schemas canonically ([c0db748](https://github.com/azrtydxb/momo/commit/c0db748c0a5a0ad28a806aea18e1256a30c959bb))
- **schema:** read snapshots by format version and refuse newer formats ([5de8c39](https://github.com/azrtydxb/momo/commit/5de8c395aa58e2ec387eb17226448055f3c844ee))
- sprint 2 — generator, PostgreSQL driver, migrations, pgvector ([#4](https://github.com/azrtydxb/momo/issues/4)) ([8f62dc9](https://github.com/azrtydxb/momo/commit/8f62dc9ac67d9d0da9e19f960804613f4c9fd974))

### Bug Fixes

- **momo:** keep the soft-delete filter with restricted columns and align Meta with the contract ([634869f](https://github.com/azrtydxb/momo/commit/634869f78f7e04d8f013015a32369386fef54ef1))
- **momo:** MySQL distinct-from and clone variadic slices ([8c6657f](https://github.com/azrtydxb/momo/commit/8c6657f8ea596e0b695969f37f447c640ae342c1))
- **momo:** parenthesise raw parts of And/Or ([aa00889](https://github.com/azrtydxb/momo/commit/aa0088946b7af4c36e86fd241c23399abc2d3a6b))
- **momo:** qualify columns of schema-qualified tables by table name ([ff1c4a3](https://github.com/azrtydxb/momo/commit/ff1c4a3e103cf2f0285d95e547867a31d03f38bc))
- **momo:** render internal bytes without discarding an always-nil error ([28f4378](https://github.com/azrtydxb/momo/commit/28f437854fa86115c7c66d9ef73ecd8f21e7313c))
- **momo:** share transaction done state with savepoint scopes ([28d90ac](https://github.com/azrtydxb/momo/commit/28d90ac172673566ae3956dd83256732963bf2ab))
- **momo:** validate select inputs instead of silently ignoring them ([4cfad2c](https://github.com/azrtydxb/momo/commit/4cfad2cf9510a20c74fec832e3eb8236b3084884))
- **schema:** keep type suffix after precision when normalising ([4803895](https://github.com/azrtydxb/momo/commit/48038953b13ccb7963789a92372e7a1c1f45ddd8))
- **testdb:** fail when search_path cannot be set ([e17281c](https://github.com/azrtydxb/momo/commit/e17281cdacee0286e4518169c2f2a89adf2c3e1e))
- **testdb:** report ensureVector errors to every caller, keep password out of DO ([d67c1e6](https://github.com/azrtydxb/momo/commit/d67c1e6ea166d23ff786cf1872bb0e0ed6324633))
