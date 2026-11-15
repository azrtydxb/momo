# fulltext-search

Status: complete

## Problem

Momo has full-text search on PostgreSQL only. Package `fts` (`fts/fts.go`) wraps `to_tsvector`, the four tsquery builders, `@@`, `ts_rank`, `ts_rank_cd` and `ts_headline`; `types.TSVector` and `types.TSQuery` map the column types; `momo.HybridSearch` fuses a text leg with a vector leg; and `docs/fulltext.md` shows the trigger-plus-GIN recipe through `MomoConfig` (`c.Index(...).Method("gin")`, `c.Function`, `c.Trigger`). Every `fts` builder fails the build with `ErrUnsupported` on SQLite and MySQL, and `docs/capabilities.md` lists full text as "no" for both.

What is missing:

- SQLite has no support at all. A SQLite user searching text has to drop to raw SQL against an FTS5 virtual table. `migrate/sqlite/inspect.go` deliberately leaves virtual tables and their shadow tables out of inspection, so momo cannot create, diff or even see an FTS5 table.
- MySQL and MariaDB have only a partial migration path: `migrate/mysql/render.go` renders `CREATE FULLTEXT INDEX` for an index with `Method: "fulltext"` (and `gen/schema.go` skips such indexes in its index-column check). There is no query builder for `MATCH ... AGAINST`, no tag or config shorthand beyond a raw `Method("fulltext")`, and no documentation.
- PostgreSQL has the query side, but the schema side is manual: the user writes the trigger function body by hand. There is no declarative "this column is searchable" that momo turns into a generated tsvector column or an expression GIN index.
- An application that runs on more than one dialect (for example SQLite in tests and PostgreSQL in production, which the `testopen.Each` pattern encourages) cannot write a search once.

Why now: the three dialect families are all supported for migrations and the typed query builder, and full-text search is the most common feature an application still has to leave momo for.

## Users

- Application authors on PostgreSQL, who already use `fts` and want the index and generated column declared on the model instead of a hand-written trigger.
- Application authors on SQLite (embedded, desktop, test suites) who want a search box over a table without raw SQL.
- Application authors on MySQL or MariaDB who want `MATCH ... AGAINST` as a typed predicate with the FULLTEXT index managed by `momo migrate`.
- Authors of multi-dialect code and test suites who want one search call that runs against every dialect they deploy to, for the common subset of features.
- Maintainers of momo: the migration engine, the `--from-db` generator and `momo migrate validate` must keep a zero diff on a database that carries these structures.

## In scope

Full-text search is in scope for all three dialect families: PostgreSQL, SQLite and MySQL/MariaDB. The API is a portable layer for the common subset plus first-class dialect builders (decision Q1); the portable layer covers all-words, phrase, exclusion, prefix, ranking and snippets, and anything beyond that is dialect-specific.

- [S-1] Query predicates: a match predicate usable in Where on PostgreSQL, SQLite and MySQL/MariaDB, rendering `@@` with tsvector/tsquery, FTS5 `MATCH`, and `MATCH(cols) AGAINST(? ...)` respectively.
- [S-2] Rank expressions: a relevance expression usable in OrderBy and Columns on each dialect (ts_rank, ts_rank_cd, bm25, and the MATCH relevance value), with the documented rule that scores are only comparable within one dialect and one query.
- [S-3] Query text handling: user input is always a bound parameter, and a documented translation (or deliberate non-translation) from the user's query text to each dialect's syntax, so that raw user input never raises a driver syntax error unless the caller chose the raw syntax.
- [S-4] Schema declaration: a way to declare on the model that a column (or set of columns) is searchable, and the `schema.Index` / `schema.Table` representation that carries it (`schema/schema.go`).
- [S-5] PostgreSQL migrations: from the declaration, render a GIN index (on a stored tsvector generated column, or on an expression), with the text search configuration pinned, diffing and applying with no drift.
- [S-6] SQLite migrations: from the declaration, create, alter, drop and keep in sync an FTS5 virtual table for the model table, with the sync mechanism decided in Q4 (external content with triggers).
- [S-7] MySQL/MariaDB migrations: from the declaration, render `FULLTEXT` indexes (single and multi-column), with `WITH PARSER ngram` where requested.
- [S-8] Inspection: `momo migrate` inspection and `generate --from-db` recognise these structures on each dialect and report zero diff against the declaration; the FTS5 virtual table stops being invisible, while its shadow tables stay hidden.
- [S-9] Query features the common subset covers: phrase, prefix, boolean exclusion, and (per dialect) a snippet or highlight expression.
- [S-10] Documentation: `docs/fulltext.md` covers all three dialects with a comparison table, and `docs/capabilities.md` and `docs/migrations.md` are updated, with the limits below written down.
- [S-12] Hybrid search on SQLite and MySQL/MariaDB: `momo.HybridSearch` runs there too, fusing a text leg (FTS5 `MATCH` ordered by bm25 on SQLite, `MATCH ... AGAINST` on MySQL/MariaDB) with a cosine-distance vector leg by reciprocal rank fusion, with the same `HybridSpec` knobs and tie-breaks as on PostgreSQL. The vector leg exists on SQLite (sqlite-vec) and MariaDB 11.8; MySQL 9.x has no distance function, so hybrid there is `ErrUnsupported`.
- [S-11] Test coverage: unit goldens on every dialect, and integration tests on PostgreSQL, SQLite and MySQL/MariaDB that run real searches (CI only; nothing is run locally).

## Out of scope

- Search engines outside the database (Elasticsearch, Meilisearch, Typesense, Bleve): no connector.
- Language detection, custom stemmers, synonym dictionaries, thesauri and custom FTS5 tokenizers written in C: the user configures these in the database; momo only names an existing configuration or tokenizer.
- Making relevance scores comparable across dialects: not possible, and not attempted.
- Typo tolerance and fuzzy search beyond what the dialect's own full text gives (PostgreSQL `pg_trgm` similarity is a separate feature).
- A portable tsvector value type on SQLite or MySQL: `types.TSVector` stays a PostgreSQL type (TEXT elsewhere, as today).
- A vector leg on MySQL 9.x: the server stores `VECTOR` values but has no distance function (`docs/vectors.md`), so this change adds none. Weighted fusion other than RRF, and fusing more than two legs, are also out.
- Changing existing `fts` behaviour on PostgreSQL. Existing builders keep their names, signatures and SQL (the goldens in `fts/fts_test.go` pin them).
- Writing any library code before this spec is approved.

## Constraints

- Backward compatible: every current `fts` export keeps its signature and rendered SQL on PostgreSQL. The `ErrUnsupported` behaviour of the PostgreSQL builders on other dialects stays; only what the portable layer adds works elsewhere.
- Security: query text, configuration names, tokenizer names and options are bound parameters wherever the dialect allows it. Where a dialect forces an identifier or literal into the SQL (FTS5 table and column names, `WITH PARSER`, a tokenizer clause in `CREATE VIRTUAL TABLE`), the value is validated against a strict allowlist or quoted with the dialect's identifier quoting. Sensitive-column masking in hook events (`FlagSensitive`) keeps working as it does in `fts` today.
- Dialect versions: PostgreSQL 12 or newer for stored generated columns (11 already has `websearch_to_tsquery`); SQLite with the FTS5 module compiled in (the cgo `driver/sqlitedriver` and the pure-Go `driver/sqlitepure` must each be checked in CI for FTS5 and bm25); MySQL 9.x and MariaDB 11.8, the versions CI runs. Anything unavailable is `ErrUnsupported` at build time or `ErrValidation`, never a raw driver string.
- No new dependency in the root module. A change that adds one takes the nested go.sum diff from CI.
- Verification only on CI. Nothing is built or tested on the laptop.
- Migrations stay forward-only and diff-clean: re-running `diff` after `apply` yields zero changes on every dialect.

## Interfaces

The shape below follows the decisions Q1 to Q10 recorded under Decisions. The identifiers are proposals; none exist yet.

### Go query API

PostgreSQL-specific builders stay in package `fts` unchanged. The portable layer would be a small set of new functions in the same package that take a target and a query value and render per dialect.

```go
// Query is the user's search text plus how to read it.
type Query struct{ /* text, syntax, config */ }

func Web(text string) Query     // websearch style: "quoted phrase", -exclude, OR
func Plain(text string) Query   // all words ANDed, punctuation ignored
func Raw(text string) Query     // dialect syntax, passed through (escape hatch)

// Config is an optional per-call text search configuration (PostgreSQL
// regconfig). It is ignored by dialects that fix the configuration in the index.
func (q Query) Config(name string) Query

// Search is the predicate: rows whose searchable columns match q.
func Search(target Target, q Query) momo.Predicate

// Score is the relevance expression: higher is better on every dialect.
func Score(target Target, q Query) Expr[float64]

// Snippet is the highlighted excerpt expression (PostgreSQL ts_headline,
// SQLite snippet()/highlight(), none on MySQL).
func Snippet(target Target, q Query, opts SnippetOpts) Expr[string]
```

A Target is what the predicate searches. It is built from generated column descriptors, so a typo is a compile error:

- a target over plain columns (for example the title and body descriptors): on MySQL this renders `MATCH(title, body) AGAINST(...)` and needs a FULLTEXT index over exactly those columns in that order; on PostgreSQL it renders `to_tsvector(config, title || ' ' || body) @@ ...` (matching an expression index) or uses the declared generated column; on SQLite it selects through the FTS5 table (Q1, Q3, Q4).
- a target over a declared search column, which is the PostgreSQL stored tsvector.

Usage, identical on all three dialects:

```go
q := fts.Web(userInput)
hits, err := momo.Select[Hit](db).
    From(momo.TableRef{Name: "articles"}).
    Columns(articleID, fts.Score(target, q).As("score")).
    Where(fts.Search(target, q)).
    OrderBy(fts.Score(target, q).Desc()).
    Limit(20).
    All(ctx)
```

Dialect-specific escape hatches remain first class: the existing PostgreSQL `fts.Match`, `fts.Rank` and `fts.Headline`, plus proposed sibling constructors for the other two dialects: an FTS5 match, a bm25 expression with per-column weights, and a MySQL match taking a mode (natural language, boolean, natural language with query expansion). Each fails the build with `ErrUnsupported` on the wrong dialect, the same pattern as `available()` in `fts/fts.go`.

Proposed translation of the web-style query (Q5):

- Plain words: PostgreSQL `websearch_to_tsquery` (AND); FTS5 `"go" "orm"` (implicit AND); MySQL BOOLEAN MODE `+go +orm`.
- Quoted phrase: PostgreSQL phrase operator `<->`; FTS5 and MySQL keep the quoted string.
- `a OR b`: PostgreSQL `a | b`; FTS5 `"a" OR "b"`; MySQL `a b` (OR is the MySQL default).
- `-spam`: PostgreSQL `!spam`; FTS5 `NOT "spam"` (valid only after a term); MySQL `-spam`.
- `pre*`: not produced by PostgreSQL websearch syntax (needs `pre:*` in `to_tsquery` syntax); FTS5 `"pre"*`; MySQL `pre*`.

Every term is escaped for the target dialect so that FTS5 operators (`NEAR`, `:`, `^`, parentheses) and MySQL operators (`+ - > < ( ) ~ * "` and `@`) in user input cannot change the query meaning or raise a syntax error.

### Schema declaration

Proposed tags and config, read statically by the generator like `c.Index` and `c.Generated` (`meta_companion.go`) today.

```go
type Article struct {
    momo.Model `momo:"table:articles"`
    ID    int64  `momo:"pk,identity"`
    Title string `momo:"search"`          // tag: searchable column
    Body  string `momo:"search"`
}

func (a *Article) MomoConfig(c *momo.Config) {
    // explicit form, with options; one search declaration per table (Q8)
    c.FullText("articles_search", &a.Title, &a.Body).
        Config("english").             // PostgreSQL regconfig; ignored elsewhere
        Weights("A", "B").             // PostgreSQL setweight; FTS5 bm25 column weights
        Tokenizer("porter unicode61"). // SQLite FTS5 tokenizer
        Parser("ngram")                // MySQL WITH PARSER
}
```

The search tag is shorthand for the config call with defaults. The declaration is the single source; each dialect's renderer decides what structures follow (below). Internally it would become a new field on `schema.Index` or `schema.Table` (Q2). Existing `Index.Method` values `gin` and `fulltext` stay valid for hand-declared indexes.

### Migrations

PostgreSQL (S-5). For a declaration over columns c1..cn with config `cfg`:

- a stored generated tsvector column, declared as `GENERATED ALWAYS AS (setweight(to_tsvector('cfg', coalesce(c1,'')), 'A') || ...) STORED` (the field is read-only, as `GeneratedVirtual` and `Generated` fields are today), plus
- `CREATE INDEX <name> ON <table> USING gin (<that column>)`.

The generated column replaces the hand-written trigger of `docs/fulltext.md`. The generation expression must be immutable, so the configuration is a literal `regconfig`, not a parameter. The alternative without an extra column is an expression index `USING gin (to_tsvector('cfg', coalesce(c1,'') || ' ' || coalesce(c2,'')))`, which the search predicate must match textually (Q3).

SQLite (S-6). From the declaration, `migrate/sqlite` emits `CREATE VIRTUAL TABLE <table>_fts USING fts5(c1, c2, content='<table>', content_rowid='<pk>', tokenize='...')` and, when the external-content approach is chosen (Q4), three triggers (after insert, after delete, after update) that issue the FTS5 `'delete'` command and re-insert. Initial population is `INSERT INTO <table>_fts(<table>_fts) VALUES('rebuild')`. The virtual table is represented per Q2. Dropping the declaration drops the triggers, then the virtual table. A change of columns, tokenizer or content table cannot be altered in place: the engine drops, recreates and rebuilds. The index is derived data, so this is not destructive, but the diff marks it as a rebuild. FTS5 with `content_rowid` needs an integer primary key (or the rowid); a table with a composite or non-integer key is `ErrValidation` at diff time naming the table.

MySQL/MariaDB (S-7). `CREATE FULLTEXT INDEX <name> ON <table> (c1, c2) [WITH PARSER ngram]` (the existing renderer, extended with the parser). Columns must be CHAR, VARCHAR or TEXT; any other type is `ErrValidation`. Adding the first FULLTEXT index to an InnoDB table adds a hidden `FTS_DOC_ID` column and rebuilds the table; the diff documents this and `migrate/lint.go` gets a warning for it (Q6).

### Inspection and `--from-db` (S-8)

- PostgreSQL: a GIN index over a tsvector column, or over a `to_tsvector(...)` expression, is recognised and mapped back to the search declaration with the config read from the expression. A tsvector generated column maps to a read-only `*types.TSVector`. A GIN index on a trigger-maintained tsvector column with no generation expression stays a plain `Method("gin")` index, so the current recipe keeps diffing clean.
- SQLite: `migrate/sqlite/inspect.go` today skips virtual tables and shadow tables. It would instead read `CREATE VIRTUAL TABLE ... USING fts5(...)` from `sqlite_master`, parse the column list, `content`, `content_rowid` and `tokenize` options, report it as the full-text description of its content table, and keep omitting the shadow tables (`_data`, `_idx`, `_content`, `_docsize`, `_config`). A virtual table that is not FTS5 stays invisible. The sync triggers are inspected as ordinary triggers.
- MySQL: `INDEX_TYPE = 'FULLTEXT'` (already selected in `qIndexesMySQL` and `qIndexesMariaDB` in `migrate/mysql/inspect.go`) maps to `Method: "fulltext"` and to the full-text description; a parser name is read from `SHOW CREATE TABLE`.
- `generate --from-db` emits the config call (or the search tag when defaults suffice), and the `fulltext`/`spatial` skip in `gen/schema.go` still applies to the index-column check.

### Hybrid search on SQLite and MySQL/MariaDB (S-12)

Vector backends that exist today (`docs/vectors.md`, `docs/capabilities.md`):

- PostgreSQL: pgvector, cosine through `vecop.Cosine`; the existing `momo.HybridSearch` (`select_hybrid.go`).
- SQLite: sqlite-vec v0.1.6 through `driver/sqlitedriver` (cgo, not Windows); float32 `vector.Vector` stored as a BLOB, `vec_distance_cosine`, brute-force scan (no ANN index), `SQLiteVectorType` for the column.
- MariaDB 11.8: `VECTOR(N)` column (from 11.7), Euclidean and cosine distance, `VECTOR INDEX`.
- MySQL 9.x: `VECTOR` storage only; distance is `ErrUnsupported`, so there is no vector leg.

Fusion method: the existing reciprocal rank fusion, unchanged. Each leg picks its own candidates (`TextCandidates` by text rank, `VectorCandidates` by cosine distance), ranks them, and a row scores the sum of `1/(RRFK + rank)` over the legs it appears in. Defaults (100 candidates, `RRFK` 60, limit 1 to 1000) and the tie-break (score then primary key ascending, within each leg and in the final order) are the same as on PostgreSQL, so the same `HybridSpec` gives the same fused order for the same per-leg rankings on every dialect. The statement is one SQL statement of CTEs with `row_number()` and a full outer join emulation, as on PostgreSQL. MySQL 8.0, MariaDB 11.8 and SQLite 3.25 or newer have CTEs and window functions. SQLite and MySQL have no `FULL OUTER JOIN`, so the fusion is a `UNION ALL` of the two legs grouped by primary key, summing the per-leg terms.

Per-dialect legs:

- SQLite: text leg is the FTS5 table of the model's search declaration, `MATCH` with the translated web-style query, ordered by bm25 ascending (best first) then primary key; vector leg is `vec_distance_cosine(col, ?)` ordered ascending then primary key, limited to the candidate count.
- MariaDB: text leg is `MATCH(cols) AGAINST(? IN BOOLEAN MODE)` ordered by the MATCH relevance descending then primary key; vector leg is the cosine distance builder ordered ascending, which the `VECTOR INDEX` can serve.
- MySQL 9.x: `ErrUnsupported` at build time naming the missing vector distance (the text leg alone is a plain search, not hybrid).

API: `HybridSpec` keeps its fields. `Query` is read as the web-style query on every dialect (on PostgreSQL it is already `websearch_to_tsquery` input). `TextConfig` stays required on PostgreSQL and is ignored on the other dialects (a value there is not an error). `Text` must be a column covered by the model's search declaration (or, on PostgreSQL, any text column, as now). `Vector` is the dense vector column. The column types are `vector.Vector` on all dialects. `HybridSearch` stops returning `ErrUnsupported` on SQLite and MariaDB; it keeps doing so on MySQL, and on any dialect where the text declaration or the vector backend is missing.

## Data

Where the index data lives, per dialect:

- PostgreSQL: the tsvector lives in the table (a stored generated column, or the user's trigger-maintained column) and is TOASTed with the row. The GIN index is a separate relation in the same schema as the table, with a pending list flushed by vacuum or `gin_pending_list_limit`. Without a stored column (expression index), only the index holds the lexemes and queries recompute `to_tsvector` for the recheck.
- SQLite: an FTS5 virtual table `<table>_fts` plus its shadow tables in the same database file: `_data` (index segments), `_idx`, `_docsize` (needed for bm25), `_config`, and `_content` only when the table is not external-content. With external content, the text is not duplicated: FTS5 reads it back from the base table by `content_rowid`. The index shares the file, the backup and the transactions of the base table.
- MySQL/MariaDB: the InnoDB FULLTEXT index is stored in hidden auxiliary tables (`fts_*_index_*`, doc-id and deleted tables) in the tablespace, plus a hidden `FTS_DOC_ID` column on the base table (added automatically when absent). New rows are held in an in-memory cache and flushed at commit or when `innodb_ft_cache_size` fills. MyISAM FULLTEXT is not supported; a table engine other than InnoDB is a diff-time `ErrValidation`.
- Ownership: all of it is derived data owned by the database and rebuildable from the base table. Momo stores nothing about it outside the schema snapshot and migration files; the snapshot records the declaration (columns, config, tokenizer, parser, weights) and nothing about index contents.

## Edge cases

- Language and stemming. PostgreSQL has a per-query and per-index regconfig, and a query using a different config than the index cannot use it (it silently scans). The declaration pins the config; the search predicate takes the config from the declaration when it is not overridden, and a different override is `ErrValidation` at build time when the target is a declared search column (Q3). SQLite fixes the tokenizer in the virtual table (`porter`, `unicode61`, `trigram`); there is no per-query override. MySQL has no language config in InnoDB beyond the parser (`ngram`, or MeCab on MySQL only) and a stop word table.
- Stop words and minimum word length. PostgreSQL drops stop words by config (`english` has them, `simple` has none). SQLite `unicode61` has no stop words. MySQL InnoDB removes stop words from a default list unless `innodb_ft_enable_stopword=0` or `innodb_ft_user_stopword_table` is set, and ignores tokens shorter than `innodb_ft_min_token_size` (3 by default) and longer than `innodb_ft_max_token_size` (84). A search for "go" or "to be or not to be" returns zero rows on MySQL by design. These are server settings applied at index build time, so changing one needs an index rebuild. Momo documents this and never claims parity.
- Phrase queries. PostgreSQL phrase uses `<->` (adjacent after stop-word removal, so "the cat" can become a one-word query). FTS5 phrase is the quoted string with exact token adjacency. MySQL phrase needs BOOLEAN MODE `"..."`; in NATURAL LANGUAGE mode quotes mean nothing, which is why the web-style query selects BOOLEAN MODE on MySQL (Q7).
- Prefix queries. FTS5 `pre*` needs no extra index but is faster with a `prefix='2 3'` option; PostgreSQL needs `pre:*` in `to_tsquery` syntax, not websearch syntax; MySQL BOOLEAN MODE `pre*` works only as a trailing star. A prefix shorter than the minimum token size matches nothing on MySQL.
- Boolean operators. All three accept AND, OR and NOT in some form with different defaults: MySQL BOOLEAN MODE ORs bare terms; FTS5 implicitly ANDs; PostgreSQL websearch ANDs. The web-style query makes AND the explicit default everywhere. A query of only negations ("-spam") matches nothing on MySQL, is a syntax error in FTS5 (NOT needs a left operand) and matches nothing on PostgreSQL (with a NOTICE); the portable search returns zero rows for it on every dialect rather than an error.
- Ranking not comparable across dialects. `ts_rank` and `ts_rank_cd` are positive floats that increase with relevance; FTS5 `bm25()` is negative and lower (more negative) is better; MySQL `MATCH` returns a non-negative float whose scale depends on the index. A portable score would negate bm25 so higher is better everywhere, but magnitudes stay incomparable: a threshold such as `score > 0.3` is meaningless across dialects and across queries (Q6).
- NULLs. A NULL column contributes nothing: PostgreSQL needs `coalesce(col, '')` inside the generated expression (otherwise `||` makes the whole tsvector NULL and the row never matches), FTS5 indexes nothing for NULL, MySQL ignores NULL in MATCH columns. A row whose searchable columns are all NULL never matches.
- Empty or whitespace-only query. Not sent to the database: the search predicate renders a false predicate and the score renders 0, avoiding dialect-specific errors (FTS5 `MATCH ''` is a syntax error; PostgreSQL raises a NOTICE and returns no rows).
- Updates and FTS5 sync. With external content, an UPDATE or DELETE on the base table that is not mirrored by the FTS5 `'delete'` command leaves stale tokens, and a later delete can corrupt the index. Momo-generated triggers (Q4) sync every write path including raw SQL, because they are database triggers. Bulk loads that disable triggers need the `'rebuild'` command afterwards; this is documented, not automatic. `INSERT OR REPLACE` on the base table does not fire delete triggers for the replaced row unless `recursive_triggers` is on, so the upsert docs must warn about it; `ON CONFLICT DO UPDATE` is safe because it fires the update trigger. Contentless tables cannot be updated or deleted by row at all.
- Transactions. MySQL InnoDB FULLTEXT sees only committed data: a row inserted or updated in an open transaction is not found by `MATCH` until that transaction commits, so "insert then search inside one `db.Tx`" returns nothing. It cannot be worked around, so it is documented as a limit and pinned by a test. FTS5 and PostgreSQL index changes are transactional and visible inside the transaction; `ROLLBACK` removes them with the base-table writes. MariaDB InnoDB FULLTEXT has the same limit as MySQL.
- Size limits: a PostgreSQL tsvector is limited to 1 MB per value (an oversized value is a database error, not silent truncation); a MySQL FULLTEXT index covers at most 16 columns.
- Collation and case. FTS5 `unicode61` is case-insensitive and removes diacritics by default; PostgreSQL uses the dictionary's folding (`english` lowercases but keeps diacritics unless `unaccent` is added); MySQL follows the column collation. The same text therefore matches differently across dialects; documented, not normalised.
- Repeated use of one call: each use of a search expression in one statement binds its own arguments, the existing `fts` limit with GROUP BY and DISTINCT documented in `docs/fulltext.md`; the portable score carries the same note.
- Hybrid, text rank scales. RRF uses ranks, not scores, so bm25 (negative, lower is better), MATCH relevance and `ts_rank_cd` need no normalisation to fuse; this is why RRF is kept as the only method. Ties within a leg break by primary key, so the fused order is stable across runs.
- Hybrid, cut-off ties. The vector leg orders by distance alone, so when rows tie on distance at the candidate cut-off, which become candidates is not defined; sqlite-vec brute force has no ANN recall loss, MariaDB `VECTOR INDEX` is approximate, so the same data can fuse differently on the two.
- Hybrid, empty legs. A query that matches no text (or is empty after escaping) leaves the text leg empty and the result is the vector leg alone with scores `1/(RRFK + rank)`; a NULL embedding or a row whose cosine distance is NULL or NaN (a zero vector) is skipped by the vector leg. A row in only one leg still appears.
- Hybrid, MySQL FULLTEXT limits. The text leg inherits the stop-word and minimum-token-size limits of MATCH, and InnoDB FULLTEXT sees only committed rows, so a row inserted in the same transaction appears in the vector leg only.
- Hybrid, soft deletes and keys. Soft-deleted rows are excluded from both legs, as on PostgreSQL; the model needs a single-column primary key (the FTS5 content rowid needs an integer one, S-6).
- Hybrid, dimension and types. A query vector whose length differs from the column dimension is `ErrValidation`, as on PostgreSQL; on SQLite the column's length CHECK (`SQLiteVectorType`) keeps stored vectors the right size.
- Soft deletes and tenancy: the search predicate composes with the soft-delete filter and tenant scope like any predicate. Soft-deleted rows stay in the index; the filter on the base table hides them.

## Failure modes

- SQLite without the FTS5 module: `migrate apply` would fail with the driver's "no such module: fts5". Momo probes for FTS5 (`pragma_compile_options` or a trial statement) and returns `ErrUnsupported` naming FTS5 instead of the raw driver text. A MySQL table that is not InnoDB: diff-time `ErrValidation`.
- Wrong dialect for a dialect-specific builder: `ErrUnsupported` at build time (existing pattern), no SQL sent.
- Invalid arguments: empty config, unknown tokenizer, normalization out of range, a target that is not declared searchable, an FTS5 target on a table without an integer primary key: `ErrValidation` at build time naming the offending value.
- Query syntax error with the raw query form: surfaces as the driver error wrapped in the usual `momo.Error`. With the web or plain forms it cannot happen by construction (covered by a fuzz test over random text).
- Missing index: PostgreSQL silently scans when the query config differs from the index config, or when the tsvector expression text differs (a slow query, not an error); MySQL `MATCH` without a FULLTEXT index over exactly those columns fails with error 1191, mapped to `ErrValidation`; FTS5 on a table that was never created fails with "no such table", mapped to `ErrUnsupported` naming the table. A lint rule for the PostgreSQL case is asked for under Q6.
- Index out of sync (SQLite external content with a missing or bypassed trigger): queries return stale or missing rows, and a delete of a base row can raise `database disk image is malformed` on the next FTS5 delete. Recovery is the `'rebuild'` command, and `'integrity-check'` detects it. `momo migrate drift` reports a missing or altered sync trigger as drift.
- Migration that cannot be applied in place: a changed tokenizer on SQLite, a changed config on PostgreSQL (the generated column expression changes, forcing a column rewrite and an index rebuild under an `ACCESS EXCLUSIVE` lock), a parser change on MySQL (index rebuild with table copy). The diff marks these and `migrate lint` flags them; on PostgreSQL index creation can use `CONCURRENTLY` with `-- momo:no-transaction` through the existing `Concurrently` index option.
- Large rebuilds: a first FULLTEXT or GIN index on a big table blocks or is slow; documented, with `-- momo:no-transaction` and `CONCURRENTLY` as the mitigations.
- Hybrid without the pieces: `HybridSearch` on MySQL 9.x, on SQLite without sqlite-vec loaded, on MariaDB before 11.7, or against a text column with no search declaration returns `ErrUnsupported` (or `ErrValidation` for the missing declaration) naming the missing piece; it never degrades silently to a text-only or vector-only search.
- Hybrid statement size: the fused statement binds the query text, the query vector, the candidate counts, `RRFK` and the limit; the vector bind is one parameter, so `MaxParams` is not a concern. A SQLite build that lacks window functions is not supported (SQLite 3.25 or newer is required, which every driver in `driver/` ships).
- Replica lag: MySQL FULLTEXT on a replica sees data only after replication commit, the same as any other data.

## Acceptance criteria

The criteria are written against the recommended design. An answer to an open question that changes the design changes the criterion it feeds, and the criterion is rewritten when the question is resolved. The test names are proposals for the tests to be written, one per criterion.

- [ ] [S-1] On PostgreSQL, SQLite and MySQL/MariaDB, the portable search predicate over a declared target, used in Where with a web-style query, returns exactly the rows containing all the query words from a fixed seed of at least six rows. Proved by `TestFullTextSearchAllDialects` (integration, per dialect); fails if any dialect returns a row missing a word or omits a row that has all of them.
- [ ] [S-1] A dialect-specific predicate on the wrong dialect (the FTS5 match on PostgreSQL, the MySQL match on SQLite, `fts.Match` on MySQL) fails the build with `ErrUnsupported` and sends no SQL. Proved by `TestFullTextWrongDialectUnsupported`; fails if any of them builds SQL or returns another error kind.
- [ ] [S-1] Rendered SQL and argument list of the portable search predicate are pinned for PostgreSQL, SQLite, MySQL and MariaDB. Proved by `TestFullTextSearchGolden`; fails if any dialect's SQL text or argument order changes.
- [ ] [S-2] The portable score used in OrderBy descending returns the row with the most occurrences of the query term first on each dialect, with bm25 negated so that higher is better everywhere. Proved by `TestFullTextScoreOrdering` (integration, per dialect); fails if bm25 is not negated or the order is inverted on any dialect.
- [ ] [S-2] `docs/fulltext.md` states that scores are comparable only within one dialect and one query, and no portable API takes a numeric score threshold. Proved by `TestFullTextScoreNotNormalised` (an API-surface test) and the docs check job; fails if a portable function accepts a threshold or the docs statement is removed.
- [ ] [S-3] The text `a OR b -c "d e" NEAR( (` and a fuzz corpus of random strings passed through the web-style query never cause a syntax error from the database on any dialect. Proved by `FuzzFullTextWebQuery` plus `TestFullTextWebQueryNeverErrors` (integration); fails if any input yields a driver syntax error.
- [ ] [S-3] The query text is always a bound parameter: no unit golden contains user text in the SQL string, and the raw query form is the only path that passes dialect syntax through. Proved by `TestFullTextQueryIsBound`; fails if user text appears in rendered SQL.
- [ ] [S-3] An empty and a whitespace-only query make the search predicate return zero rows and the score return 0 on each dialect without sending a MATCH. Proved by `TestFullTextEmptyQuery`; fails if a MATCH with empty text is sent or any row is returned.
- [ ] [S-9] A quoted phrase matches only rows with the words adjacent and in order, and a prefix query matches rows with words starting with the prefix, on each dialect; a MySQL prefix shorter than `innodb_ft_min_token_size` matches nothing. Proved by `TestFullTextPhraseAndPrefix` (integration, per dialect); fails if a phrase matches non-adjacent words or a prefix misses a row.
- [ ] [S-9] A `-word` exclusion removes the rows containing the word on each dialect, and a query of only negations returns zero rows and no error. Proved by `TestFullTextExclusion`; fails if an excluded row is returned or a negation-only query errors.
- [ ] [S-9] The snippet expression returns the text with matched terms marked on PostgreSQL and SQLite, and fails the build with `ErrUnsupported` on MySQL. Proved by `TestFullTextSnippet`; fails if MySQL builds SQL or the marks are absent.
- [ ] [S-4] The search tag and the explicit config call produce the same `schema.Table` entry, and the generator rejects the tag on a non-text field and a config call naming an unknown field with a diagnostic naming the field. Proved by `TestGenerateFullTextDeclaration` in `gen`; fails if the two forms differ or no diagnostic names the field.
- [ ] [S-4] The schema snapshot JSON round-trips the full-text description (columns, config, weights, tokenizer, parser), and a snapshot written before this change still loads. Proved by `TestSchemaFullTextSnapshotCompat` in `schema`; fails if a field is lost or an old snapshot errors.
- [ ] [S-5] On PostgreSQL, `migrate diff` for a model with a declaration emits a stored generated tsvector column with `coalesce` over each source column and a GIN index on it; `apply` runs it; a second `diff` is empty. Proved by `TestFullTextPostgresMigrationRoundTrip` (integration); fails if the second diff is non-empty or a NULL source column makes the vector NULL.
- [ ] [S-5] Changing the declared config from `english` to `simple` produces a diff that alters the generated column and rebuilds the index, and `migrate lint` flags it. Proved by `TestFullTextConfigChangeDiff`; fails if no change or no lint finding is produced.
- [ ] [S-5] The existing hand-written trigger and GIN recipe of `docs/fulltext.md` still diffs, applies and inspects with no drift. Proved by the existing `TestTSVectorMigrationRoundTrip` (`migrate/postgres/tsvector_roundtrip_test.go`), unchanged and green.
- [ ] [S-6] On SQLite, `migrate diff` for a model with a declaration emits an FTS5 virtual table with the declared columns, content table, content rowid and tokenizer, plus the sync triggers if external content is chosen, and a second `diff` after `apply` is empty. Proved by `TestFullTextSQLiteMigrationRoundTrip` (integration on real SQLite); fails if the second diff is non-empty.
- [ ] [S-6] After `apply` on SQLite, an insert, an update and a delete of a base row each change what the search returns, with no manual rebuild. Proved by `TestFullTextSQLiteSync`; fails if a stale or missing row appears after any of the three writes.
- [ ] [S-6] After an update then a delete of base rows, `INSERT INTO t_fts(t_fts) VALUES('integrity-check')` succeeds. Proved by `TestFullTextSQLiteIntegrityAfterUpdateDelete`; fails if the update trigger orders the FTS5 delete and insert commands wrongly and the index corrupts.
- [ ] [S-6] A declaration on a SQLite table with a composite or non-integer primary key fails `migrate diff` with `ErrValidation` naming the table. Proved by `TestFullTextSQLiteNeedsIntegerKey`; fails if SQL is emitted or the table is not named.
- [ ] [S-6] Dropping the declaration drops the triggers first and then the virtual table, and `diff` after `apply` is empty. Proved by `TestFullTextSQLiteDrop`; fails if the order is reversed or a shadow object remains.
- [ ] [S-6] The cgo and `sqlitepure` SQLite drivers are each tested for FTS5 and bm25; a driver without FTS5 gives `ErrUnsupported` naming FTS5, not the raw driver message. Proved by `TestFullTextSQLiteDriverSupport` per driver (or a recorded skip with reason); fails if the raw "no such module" text leaks.
- [ ] [S-7] On MySQL and MariaDB, `migrate diff` emits `CREATE FULLTEXT INDEX` (with `WITH PARSER ngram` when declared) for a model with a declaration, and a second `diff` after `apply` is empty. Proved by `TestFullTextMySQLMigrationRoundTrip` on both servers; fails if the parser is lost or the second diff is non-empty.
- [ ] [S-7] A FULLTEXT declaration over a non-text column, a unique FULLTEXT, and a FULLTEXT on a non-InnoDB table each fail with `ErrValidation`. Proved by `TestFullTextMySQLRenderValidation` in `migrate/mysql`; fails if any of the three renders SQL (the unique case is already in `createIndex`).
- [ ] [S-7] The MySQL match builder renders `MATCH(cols) AGAINST(? IN ... MODE)` in natural-language and boolean modes with the query bound, and a MATCH over a column list with no matching index surfaces as `ErrValidation`. Proved by `TestFullTextMySQLMatchGolden` and `TestFullTextMySQLMissingIndex`; fails if the mode text or the error kind differs.
- [ ] [S-8] Inspection reports the full-text structures on each dialect and the diff against the declaration is empty: PostgreSQL GIN over a tsvector column and over a `to_tsvector` expression, SQLite FTS5 table with its options, MySQL FULLTEXT index with a parser. Proved by `TestFullTextInspectRoundTrip` (integration, per dialect); fails if inspection misses a structure or reports a diff.
- [ ] [S-8] On SQLite, shadow tables of an FTS5 table stay excluded from inspection and `generate --from-db`, and a non-FTS5 virtual table is still ignored. Proved by `TestSQLiteInspectIgnoresShadowTables` plus the existing virtual-table tests in `migrate/sqlite`; fails if a shadow table or a non-FTS5 virtual table is reported.
- [ ] [S-8] `generate --from-db` on each dialect emits the search declaration for a database carrying these structures, and the emitted models compile and diff clean against that database. Proved by `TestGenerateFromDBFullText` in `gen/fromdb`; fails if no declaration is emitted or the diff is non-empty.
- [ ] [S-11] On MySQL and MariaDB a row inserted in an open `db.Tx` is not returned by the search inside that transaction and is returned after commit; on PostgreSQL and SQLite it is returned inside the transaction; a rollback removes it everywhere. Proved by `TestFullTextTransactionVisibility` (integration, per dialect); fails if visibility differs from this table on any dialect.
- [ ] [S-11] A search for a two-character word returns no rows on MySQL with `innodb_ft_min_token_size=3`, even for rows containing it. Proved by `TestFullTextMySQLMinTokenSize` (skipped with a recorded reason if CI overrides the setting); fails if rows are returned.
- [ ] [S-11] A row whose searchable columns are all NULL is never returned on any dialect, and a row with one NULL and one text column is found by the text column. Proved by `TestFullTextNulls` (integration, per dialect, including the PostgreSQL `coalesce` case); fails if a NULL source hides a row that has text or a NULL row matches.
- [ ] [S-11] When a searchable column is marked `FlagSensitive`, the query text and config arguments appear as `RedactedArg` in hook events on every dialect. Proved by `TestFullTextHooksSeeMaskedArgsPortable`, extending the existing `TestFullTextHooksSeeMaskedArgs`; fails if a raw argument reaches a hook.
- [ ] [S-11] Every existing `fts` unit golden and integration test passes unchanged. Proved by the existing `TestFullTextGolden` and `TestFullTextSearch` staying green on the PR; fails if a PostgreSQL builder's SQL changes.
- [ ] [S-10] `docs/fulltext.md` contains a per-dialect comparison table (index type, query syntax, ranking direction, stop words, minimum token length, phrase, prefix, snippet, transaction visibility), and the full-text row of `docs/capabilities.md` shows support on all three dialects. Proved by the docs check job (`docscheck` fragments and `internal/gendocs`); fails if a fragment does not compile or the generated capabilities table is stale.
- [ ] [S-10] `docs/migrations.md` describes the declaration, the structures each dialect creates, the SQLite rebuild command and the MySQL `FTS_DOC_ID` table rebuild. Proved by the docs check job; fails if the section is missing or a fragment fails to compile.

- [ ] [S-12] On SQLite (sqlite-vec) and MariaDB 11.8, `momo.HybridSearch` over a fixed seed returns rows ordered by the RRF score, where a row in both legs outranks a row in one, and the scores equal the sum of `1/(RRFK + rank)` computed independently in the test from each leg's ranking. Proved by `TestHybridSearchFusionSQLiteMariaDB` (integration, both dialects); fails if the order or any score differs from the independent computation.
- [ ] [S-12] Ties break by primary key ascending in each leg and in the final order, and the result is identical for the same data inserted in two different orders. Proved by `TestHybridSearchTieBreakPortable`; fails if the order depends on insertion order.
- [ ] [S-12] A query that matches no text returns the vector leg alone with scores `1/(RRFK + rank)`, and a row with a NULL embedding is never returned from the vector leg. Proved by `TestHybridSearchEmptyTextLegPortable`; fails if the result is empty or a NULL-embedding row appears through the vector leg.
- [ ] [S-12] `HybridSearch` on MySQL 9.x returns an error wrapping `ErrUnsupported` that names the missing vector distance, and sends no SQL; it never returns a text-only result. Proved by `TestHybridSearchMySQLUnsupported`; fails if any statement is sent or another error kind is returned.
- [ ] [S-12] Hybrid on a text column with no search declaration returns `ErrValidation` naming the column on SQLite and MariaDB, and the `HybridSpec` validation of candidates (1 to 1000), `RRFK` (at least 1), `Limit` (1 to 1000) and query vector dimension gives the same errors as on PostgreSQL. Proved by `TestHybridSpecValidationPortable`; fails if any case builds SQL or returns a different kind.
- [ ] [S-12] The rendered SQL and argument list of the hybrid statement are pinned for SQLite and MariaDB, and the existing PostgreSQL statement is unchanged. Proved by `TestHybridSearchGoldenPortable` plus the existing `select_hybrid_test.go` goldens staying green; fails if the PostgreSQL SQL text changes.
- [ ] [S-12] Soft-deleted rows are excluded from both legs on SQLite and MariaDB. Proved by `TestHybridSearchSoftDeletePortable`; fails if a soft-deleted row is returned through either leg.
- [ ] [S-12] `docs/vectors.md` and `docs/fulltext.md` describe hybrid on the three dialects, and `docs/capabilities.md` shows hybrid as supported on SQLite and MariaDB and unsupported on MySQL. Proved by the docs check job; fails if the generated capabilities table is stale or a fragment does not compile.

## Decisions

Decision (2026-11-15, the user, via AskUserQuestion; Q2 to Q10 below were answered the same way). Q1: a portable layer (search predicate, score, snippet, and web/plain/raw query constructors) for the common subset, plus first-class dialect builders for everything beyond it. See Interfaces, Go query API.

Q2 (decided): the declaration is a full-text description on `schema.Index` (Method `"fulltext"` plus config, weights, tokenizer, parser). The SQLite FTS5 table and triggers are derived by the SQLite renderer, not stored as tables in the snapshot. See Interfaces, Schema declaration and Migrations.

Q3 and Q10 (decided): PostgreSQL gets a stored generated tsvector column plus GIN by default, with an expression index as an option. PostgreSQL 12 or newer is required for the declarative feature only; the existing `fts` builders keep working on older versions. The config is pinned as a literal in the generated expression. See Constraints and Interfaces, Migrations.

Q4 (decided): SQLite uses an external-content FTS5 table (`content='<table>', content_rowid='<pk>'`) with momo-generated insert, update and delete triggers. See Interfaces, Migrations and Edge cases.

Q5 (decided): the portable layer translates a websearch-style syntax per dialect with escaping, and offers the raw form as the escape hatch. See Interfaces, Go query API.

Q6 (decided): the portable score is higher-is-better (bm25 negated), comparable only within one query on one dialect, with no numeric threshold and weights as a per-column option. `migrate lint` warns (not errors) for adding the first InnoDB FULLTEXT index, for a changed PostgreSQL config, and for a PostgreSQL search whose config differs from the index. See Edge cases and Failure modes.

Q7 (decided): the portable web-style query uses BOOLEAN MODE on MySQL/MariaDB; natural language mode and query expansion are reachable only through the MySQL-specific builder.

Q8 (decided): one search declaration per table, covering one or more text columns. Extra independent indexes on the same table are allowed only through the explicit dialect forms (`Method("fulltext")`).

Q9 (decided, overriding the proposal to defer it): hybrid (text plus vector) search is in scope for SQLite and MySQL/MariaDB in this feature, as fusion over the FTS5 or MATCH text leg and the existing vector builders. See In scope [S-12], Interfaces, Hybrid search on SQLite and MySQL/MariaDB, and the criteria citing S-12.

## Open questions

<!-- All ten questions are resolved; see Decisions above. -->

