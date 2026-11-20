# Full-text search

Momo searches text on PostgreSQL, SQLite and MySQL or MariaDB. A model declares
once which text columns are searchable; `momo migrate` creates the structure
each database needs for that declaration, and one set of calls in package `fts`
runs the search on all of them. Package `fts` also keeps the builders that
write PostgreSQL full-text SQL by hand (`fts.TSMatch`, `fts.TSRank`,
`fts.TSHeadline`) and two builders for `MATCH ... AGAINST` on MySQL and MariaDB
(`fts.MySQLMatch`, `fts.MySQLRelevance`). The capabilities table in
[capabilities](capabilities.md) shows the `dialect.Dialect.FullText` flag per dialect.

The three databases do not search alike. The portable calls cover the common
subset: all words, phrases, exclusion, prefixes, a relevance score and an
excerpt. The [comparison table](#how-the-dialects-differ) and the
[limits](#limits) list what differs. Momo documents the differences and does
not hide them.

## Declare a search

A model declares one search index per table, either with the `search` tag on
each text field or with `c.FullText` in `MomoConfig`.

<!-- docscheck:fragment -->

```go
type Article struct {
	momo.Model `momo:"table:articles"`
	ID         int64  `momo:"pk,identity"`
	Title      string `momo:"search"`
	Body       string `momo:"search"`
}
```

The tag names the index `<table>_search` and covers the tagged fields in field
order. It carries no options. PostgreSQL needs a text search configuration, so
on PostgreSQL write the explicit form, which takes the options a dialect
honours:

<!-- docscheck:fragment -->

```go
func (a *Article) MomoConfig(c *momo.Config) {
	c.FullText("articles_search", &a.Title, &a.Body).
		Config("english").
		Weights("A", "B").
		Tokenizer("porter unicode61")
}
```

| Option       | PostgreSQL                                         | SQLite                                      | MySQL and MariaDB                          |
| ------------ | -------------------------------------------------- | ------------------------------------------- | ------------------------------------------ |
| `Config`     | required: the text search configuration, `english` | ignored                                     | ignored                                    |
| `Weights`    | `A` to `D` per column, written with `setweight`    | read at query time as `bm25` column weights | no effect                                  |
| `Tokenizer`  | ignored                                            | the FTS5 `tokenize` clause                  | ignored                                    |
| `Parser`     | ignored                                            | ignored                                     | `ngram` or `mecab` (MySQL only, see below) |
| `Expression` | an expression index instead of a generated column  | ignored                                     | ignored                                    |

An option a dialect ignores is dropped before the declaration is compared, so
changing it is no migration there. A weight changed on SQLite changes the next
query and no schema.

`momo generate` refuses the tag on a column that is not text, a second
declaration on one table, a column the table does not have, and a
configuration, tokenizer or parser outside a strict list of names:

- `Config` is a plain identifier such as `english` or `simple`.
- `Weights` has one letter from `A`, `B`, `C`, `D` per column and cannot be
  combined with `Expression`.
- `Tokenizer` is `unicode61` (optionally followed by `remove_diacritics` and
  0, 1 or 2), `ascii` or `trigram`, each optionally prefixed with `porter `.
- `Parser` is `ngram` or `mecab`.

The declaration is the only thing the schema snapshot records. Every dialect
derives its own structures from it, so a snapshot written before the
declaration existed loads unchanged.

## What each dialect creates

### PostgreSQL

PostgreSQL 12 or newer is needed, for stored generated columns. A declaration
becomes a stored generated tsvector column named `<index>_tsv` and a GIN index
on it:

```text
ALTER TABLE articles ADD COLUMN articles_search_tsv tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('english', coalesce("title", '')), 'A') ||
    setweight(to_tsvector('english', coalesce("body", '')), 'B')) STORED;
CREATE INDEX articles_search ON articles USING gin (articles_search_tsv);
```

The configuration is a literal inside the expression, which PostgreSQL requires
for an immutable generation expression. `coalesce` keeps a NULL column from
making the whole vector NULL, so a row with one NULL column is found by the
other. Without `Weights` the `setweight` calls are left out. The column is
read-only: leave it out of inserts and updates, and the model needs no field
for it.

With `Expression` there is no extra column. The index is a GIN index on
`to_tsvector('english', coalesce(title, '') || ' ' || coalesce(body, ''))`, and
a search renders that same expression. `Expression` cannot be combined with
`Weights`.

Any change to the columns, the configuration, the weights or `Expression`
cannot alter the expression in place. The migration drops the index, drops and
adds the generated column, which rewrites the table under an
`ACCESS EXCLUSIVE` lock, and creates the index again. `momo migrate lint`
reports it as `fulltext-column-rewrite`. The column name is
`<index>_tsv`; a model column of that name, an index name that makes the name
longer than 63 bytes and a source column the table lacks are `ErrValidation`.

The migration builds the index with a plain `CREATE INDEX`, which blocks writes
while it builds. A declaration with `.Concurrently()`, for example
`c.FullText("articles_search", &a.Title).Config("english").Concurrently()`,
splits the work instead: the generated column and the drop of an old index stay
in the transactional migration, and `CREATE INDEX CONCURRENTLY` goes into the
migration that follows it with `-- momo:no-transaction`. SQLite and MySQL have
no concurrent build and ignore `Concurrently()` on a full-text index, as they do
on any index. See
[migrations](migrations.md#3b-full-text-search-structures).

A tsvector column that a trigger maintains, as in [the column](#the-column), is
not a declaration. It stays a plain `Method("gin")` index and diffs as before.

### SQLite

A declaration becomes an external-content FTS5 table named `<table>_fts` and
three triggers, `<table>_fts_ai`, `<table>_fts_ad` and `<table>_fts_au`, that
run after an insert, a delete and an update of the table:

```text
CREATE VIRTUAL TABLE "articles_fts" USING fts5("title", "body", content='articles', content_rowid='id', tokenize='porter unicode61');
INSERT INTO "articles_fts"("articles_fts") VALUES('rebuild');
```

The text is not stored twice: FTS5 reads it back from the table by rowid. The
`'rebuild'` command that fills the new FTS5 table runs as part of the migration.
The update trigger issues the FTS5 `'delete'` command before the insert; the
reverse order corrupts the index.

- The table needs a rowid table whose primary key is one `INTEGER` column. A
  composite or other key, a `WITHOUT ROWID` table, a declared column the table
  lacks, and a column called `rank` or `rowid` are `ErrValidation` naming the
  table.
- A change of the columns or the tokenizer drops the FTS5 table, creates it
  again and fills it with `'rebuild'`. Removing the declaration, or the table, drops the
  triggers first and then the FTS5 table. Renaming the table re-creates both
  under the new name.
- `INSERT OR REPLACE` and `ON CONFLICT REPLACE` delete the replaced row without
  firing the delete trigger unless `PRAGMA recursive_triggers = ON`, which
  leaves stale tokens in the FTS5 table. A bulk load that runs with the
  triggers disabled has the same effect. Repair the index from the table with
  `INSERT INTO articles_fts(articles_fts) VALUES('rebuild')`;
  `INSERT INTO articles_fts(articles_fts) VALUES('integrity-check')` tests it.
  `ON CONFLICT DO UPDATE` is safe: it fires the update trigger.
- The SQLite build needs the FTS5 module. The cgo driver `driver/sqlitedriver`
  compiles it in only with the build tag `-tags sqlite_fts5` (or
  `CGO_CFLAGS=-DSQLITE_ENABLE_FTS5`); the pure-Go driver `driver/sqlitepure`
  includes it. A migration on a build without FTS5 fails with `ErrUnsupported`
  naming FTS5, and a search whose FTS5 table does not exist fails with
  `ErrUnsupported` naming the table.

### MySQL and MariaDB

A declaration becomes an InnoDB `FULLTEXT` index:

```text
CREATE FULLTEXT INDEX articles_search ON articles (title, body) WITH PARSER ngram;
```

- The columns must be `CHAR`, `VARCHAR` or one of the `TEXT` types, at most 16
  of them, and the index cannot be unique. A violation is `ErrValidation`.
- The table must be InnoDB. Momo creates only InnoDB tables, and `AutoMigrate`
  refuses a declaration on a table the database holds with another storage
  engine.
- `WITH PARSER` is written only when a parser is declared, and the parser is a
  MySQL feature: MariaDB InnoDB has no full-text parser plugin. A parser on
  MariaDB is `ErrUnsupported` when the engine knows the server is MariaDB. The
  offline `momo migrate` commands open no database and cannot tell MariaDB from
  MySQL, so they render the parser, and MariaDB rejects it when the migration
  is applied. `mecab` needs the MeCab plugin on the MySQL server.
- The first `FULLTEXT` index of an InnoDB table adds a hidden `FTS_DOC_ID`
  column and rebuilds the table, which can take long and block writes on a
  large one. `momo migrate lint` reports every `CREATE FULLTEXT INDEX` on an
  existing table as `fulltext-first-index`.
- Changing the parser rebuilds the index with a table copy.

## Search

The portable calls are in the package `github.com/azrtydxb/momo/fts`. They read
the declaration of the model, so a typo in a column is a compile error and a
model without a declaration is `ErrValidation`.

| Call             | Result                                                                   |
| ---------------- | ------------------------------------------------------------------------ |
| `fts.Over`       | the `fts.Target`: the declared columns of model T, in the declared order |
| `fts.OverVector` | a tsvector column you maintain yourself, with its configuration          |
| `fts.Search`     | a predicate: rows whose searchable columns match the query               |
| `fts.Score`      | a float expression: the relevance of a row, higher is better             |
| `fts.Snippet`    | a string expression: an excerpt with the matched words marked            |
| `fts.Web`        | a query read like a search box                                           |
| `fts.Plain`      | a query whose words must all match                                       |
| `fts.Raw`        | a query in the dialect's own syntax                                      |

<!-- docscheck:fragment -->

```go
target := fts.Over[Article](articleTitle, articleBody)
q := fts.Web(userInput)
score := fts.Score(target, q)

hits, err := momo.Select[Hit](db).
	From(momo.TableRef{Name: "articles"}).
	Columns(articleID, score.As("score"),
		fts.Snippet(target, q, fts.SnippetOpts{Column: articleBody}).As("excerpt")).
	Where(fts.Search(target, q)).
	OrderBy(score.Desc()).
	Limit(20).
	All(ctx)
```

`fts.Over` takes exactly the declared columns in the declared order; any other
list is `ErrValidation` naming both. On PostgreSQL the search runs over the
generated column `<index>_tsv` (or over the declared expression); on SQLite it
selects through the FTS5 table `<table>_fts`; on MySQL and MariaDB it renders
`MATCH(title, body) AGAINST (? IN BOOLEAN MODE)` over the `FULLTEXT` index.
`fts.OverVector` targets a tsvector column that a trigger or a hand-written
generated column maintains, and works on PostgreSQL only; other dialects fail
the build with `ErrUnsupported`.

The query text is always a bound parameter. The configuration, the tokenizer
and the table names are never taken from input. A search that uses a column
marked sensitive masks its arguments in hook events, as the other `fts`
builders do. Each use of a call in one statement binds its own arguments, so
the limits for `GROUP BY` and `SELECT DISTINCT` described under
[Expressions](#expressions) apply to `fts.Score` as well.

### Query syntax

`fts.Web` reads quoted phrases, `-exclusions`, an uppercase `OR` between words
and a trailing `*` for a prefix. It never raises a syntax error in the
database, whatever the user types: every word is cut down to letters, digits,
underscores and apostrophes, punctuation inside a word splits it (`well-known`
reads as the phrase "well known"), and a bare `AND`, `NOT` or `NEAR` is
dropped. `fts.Plain` reads every word as a term of one conjunction and ignores
punctuation. `fts.Raw` passes the text to the database in its own syntax and
leaves its correctness to you; a malformed text is a database error. The text
is still bound.

| Web query      | PostgreSQL                | SQLite               | MySQL and MariaDB |
| -------------- | ------------------------- | -------------------- | ----------------- |
| `red fox`      | both words                | `"red" "fox"`        | `+red +fox`       |
| `"red fox"`    | the phrase operator `<->` | `"red fox"`          | `+"red fox"`      |
| `red OR fox`   | either word               | `("red") OR ("fox")` | `(+red) (+fox)`   |
| `fox -hen`     | `fox` and not `hen`       | `"fox" NOT "hen"`    | `+fox -hen`       |
| `fo*`          | the prefix `fo:*`         | `"fo"*`              | `+fo*`            |
| `fts.Raw` text | `to_tsquery` text         | an FTS5 query        | boolean-mode text |

On PostgreSQL a `fts.Web` query without a prefix term is bound as given to
`websearch_to_tsquery`, which reads it by its own rules: a lowercase `or` is an
OR too, and a `-word` binds to its own OR group. A query with a prefix term is
rewritten by the shared parser instead and rendered as `to_tsquery` text with
every word quoted; that reading takes only an uppercase `OR` and applies every
exclusion to the whole query, as SQLite and MySQL do. `fts.Plain` is
`plainto_tsquery` on PostgreSQL.

A query without a positive word (empty, only blanks, or only exclusions) is not
sent to the database: `fts.Search` renders a false predicate, `fts.Score` renders 0
and `fts.Snippet` renders NULL. A negation-only query therefore returns no rows
and no error on every dialect.

`fts.Query.Config` sets the PostgreSQL configuration for one query. On a target
from `fts.Over` it must equal the declared configuration, because a different
one cannot use the index; another name is `ErrValidation` at build time. On
`fts.OverVector` it replaces the configuration given there. SQLite and MySQL fix
the configuration in the index and ignore it.

### Score

`fts.Score` is `ts_rank_cd` on PostgreSQL, the negated `bm25` on SQLite (FTS5
reports `bm25` as a negative number where lower is better) and the `MATCH`
value on MySQL and MariaDB. Higher is better on every dialect, and a row the
query does not match scores 0.
Scores are comparable only within one dialect and one query; no portable function takes a score threshold. Use a score to
order rows, never to cut them off, and never compare it with a score from
another query or another database. On SQLite the declared weights `A` to `D`
are `bm25` column weights of 1, 0.4, 0.2 and 0.1.

### Snippet

`fts.Snippet` excerpts the column named in `fts.SnippetOpts.Column`, which must
be one of the declared columns, and marks the matched words with `<b>` and
`</b>` unless `fts.SnippetOpts.Open` and `fts.SnippetOpts.Close` say otherwise.
It is `ts_headline` on PostgreSQL and `snippet()` on SQLite, where
`fts.SnippetOpts.Ellipsis` marks the cut ends (default `...`) and
`fts.SnippetOpts.Tokens` sets the length in words (1 to 64, default 16). MySQL
and MariaDB have no excerpt function: `fts.Snippet` fails the build with
`ErrUnsupported` there. Neither database escapes the text, so escape the
excerpt before showing it as HTML.

### Raw text and the MySQL modes

<!-- docscheck:fragment -->

```go
pgq := fts.Raw("fox:* & !hen").Config("english") // to_tsquery text
litq := fts.Raw(`fox NOT hen`)                    // FTS5 query
myq := fts.Raw("+fox -hen")                       // boolean-mode text
```

On MySQL and MariaDB `fts.Search` and `fts.Score` always use boolean mode with
a string built from the parsed words, so no typed character acts as an
operator. For natural-language mode or query expansion, which have no portable
form, use `fts.MySQLMatch` and `fts.MySQLRelevance` with a mode of your own.
The text is passed as given, so in `fts.Boolean` mode you own its operators.

<!-- docscheck:fragment -->

```go
cols := []momo.Column[string]{articleTitle, articleBody}
rel := fts.MySQLRelevance(cols, userInput, fts.NaturalLanguage)

hits, err := momo.Select[Hit](db).
	From(momo.TableRef{Name: "articles"}).
	Columns(articleID, rel.As("relevance")).
	Where(fts.MySQLMatch(cols, userInput, fts.NaturalLanguage)).
	OrderBy(rel.Desc()).
	All(ctx)
```

The columns must be exactly the columns of one `FULLTEXT` index, in its order.
A `MATCH` without such an index is `ErrValidation`, not the server's error 1191.
Both builders fail the build with `ErrUnsupported` on PostgreSQL and SQLite.

## How the dialects differ

| Feature                | PostgreSQL                                               | SQLite                                         | MySQL and MariaDB                        |
| ---------------------- | -------------------------------------------------------- | ---------------------------------------------- | ---------------------------------------- |
| Index type             | GIN on a stored generated tsvector column                | external-content FTS5 table and sync triggers  | InnoDB `FULLTEXT` index                  |
| Query syntax           | `@@` with a tsquery                                      | FTS5 `MATCH`                                   | `MATCH ... AGAINST (? IN BOOLEAN MODE)`  |
| Ranking direction      | `ts_rank_cd`, higher is better                           | `bm25` is lower-is-better, `fts.Score` negates | the `MATCH` value, higher is better      |
| Stop words             | by configuration (`english` has them, `simple` has none) | none with the `unicode61` tokenizer            | the InnoDB list, set by server variables |
| Minimum token length   | none                                                     | none                                           | `innodb_ft_min_token_size`, 3 by default |
| Phrase                 | the `<->` operator                                       | a quoted string, exact adjacency               | a quoted string in boolean mode          |
| Prefix                 | `:*` in tsquery text                                     | `"pre"*`                                       | a trailing `*`                           |
| Snippet                | `ts_headline`                                            | `snippet()`                                    | `ErrUnsupported`                         |
| Transaction visibility | a write is visible in its own transaction                | a write is visible in its own transaction      | only committed rows                      |

## Limits

- Stop words. PostgreSQL drops the stop words of the configuration, so a phrase
  such as "the cat" can reduce to one word. MySQL InnoDB drops the words of its
  default stop word list unless `innodb_ft_enable_stopword` is off or a user
  list is set. A required term that is a stop word matches nothing, so a
  search that needs `the` or `it` finds nothing. SQLite `unicode61` has no
  stop words.
- Minimum token length. InnoDB does not index a word shorter than
  `innodb_ft_min_token_size` (3 by default), so a search for a two-letter word
  finds nothing, even in rows that contain it. A prefix shorter than that still
  matches the longer indexed words that start with it. The server settings are
  applied when the index is built, so changing one needs an index rebuild.
- Committed rows only. On MySQL and MariaDB a row inserted or updated in an open
  transaction is not found by a search, in the same transaction or any other,
  until the transaction commits. Inserting and then searching inside one
  `db.Tx` returns nothing. PostgreSQL and SQLite index changes are
  transactional: the search sees them inside the transaction, and a rollback
  removes them.
- MySQL boolean mode has no prefix phrase. A required prefix phrase such as
  `well-kn*` is rendered as `+well +kn*`, so it also matches rows that hold
  the words apart. An excluded prefix phrase is rendered as the exact phrase
  without the wildcard (`-"well kn"`), so it never excludes more than the
  phrase.
- A PostgreSQL tsvector is limited to 1 MB per value. A longer value is a
  database error, not a silent cut.
- A MySQL `FULLTEXT` index covers at most 16 columns.
- Collation and case. FTS5 `unicode61` folds case and removes diacritics by
  default. A PostgreSQL configuration lowercases but keeps diacritics unless
  `unaccent` is added to it. MySQL follows the column collation. The same text
  can therefore match differently on each dialect.
- NULL. A NULL column contributes nothing, and a row whose searchable columns
  are all NULL never matches.
- A search that cannot run fails with a momo error, not the driver's text: a
  missing FTS5 build or table is `ErrUnsupported`, a missing `FULLTEXT` index
  or a configuration that differs from the index is `ErrValidation`.

## Inspection and migration tools

`momo migrate diff` compares the declaration, with the structures each dialect
derives from it, against the snapshot and writes the SQL above. A second `diff`
after `apply` is empty. `momo migrate validate` and `AutoMigrate` apply the same
checks. `momo migrate drift` and `generate --from-db` read the database back:

- PostgreSQL: a stored generated column and its GIN index, written in the shape
  momo renders, read back as the declaration with its configuration, columns and
  weights; so is a GIN index on a `to_tsvector` expression of that shape. Any
  other tsvector column or expression stays a plain index.
- SQLite: an external-content FTS5 table is read from `sqlite_master` with its
  columns and tokenizer. The shadow tables of an FTS5 table stay hidden, and a
  virtual table of another module, or an FTS5 table with an option the
  declaration has no field for, is not reported as a declaration. The sync
  triggers are ordinary triggers: `momo migrate drift` reports a missing or
  altered one.
- MySQL and MariaDB: an index of type `FULLTEXT` is read as the declaration, and
  on MySQL its parser comes from `SHOW CREATE TABLE`. The storage engine of a
  table that is not InnoDB is read too, which `AutoMigrate` uses to refuse a
  declaration on it.

`generate --from-db` writes the `search` tag when it says everything (a
declaration on SQLite, MySQL or MariaDB named `<table>_search`, with no option)
and `c.FullText` otherwise. It writes no field for the generated tsvector
column of PostgreSQL and none for the sync triggers of SQLite, because the
declaration implies them. A full-text index the models cannot declare is
reported as a warning and left out.

## Hybrid search

`momo.HybridSearch` fuses a full-text leg and a cosine-distance vector leg with
reciprocal rank fusion. It runs on PostgreSQL, on SQLite (FTS5 and sqlite-vec,
so the cgo driver) and on MariaDB 11.8. MySQL has no vector distance function,
so on MySQL the call fails with `ErrUnsupported` before any SQL is sent; it
never falls back to a text-only search.

On SQLite and MariaDB the text leg searches the structure of the model's
declaration, the FTS5 table ordered by `bm25` or a boolean-mode `MATCH` over
all declared columns. `HybridSpec.Text` must be one of the declared columns, or
the call is `ErrValidation`. `HybridSpec.TextConfig` is ignored there, and
`HybridSpec.Query` is read like a `fts.Web` query. A query without a positive
word leaves the text leg empty and the result is the vector leg alone. On
MariaDB the text leg inherits the stop word, minimum token length and
committed-rows limits above, so a row inserted in the same transaction appears
through the vector leg only. See [hybrid search](vectors.md#hybrid-search) for
the knobs, the tie-breaks and the PostgreSQL leg.

## The column

PostgreSQL full-text search keeps a column type and the typed builders it had
before the portable calls. Every builder in this part returns `ErrUnsupported`
when the statement is built for SQLite or MySQL; use the calls above on those
dialects.

A model field of type `types.TSVector` maps to a `tsvector` column (and a
`types.TSQuery` field to `tsquery`). Both are codecs, so the generator and
`migrate` pick the column type from the Go type, diff it and read it back from
the database with no drift. The Go value is the text form PostgreSQL prints,
for example `'cat':3 'fat':2`. A nullable column is a `*types.TSVector`.

A declaration with `c.FullText` needs no such field: momo derives a generated
column. To maintain the column yourself, leave it nil on insert and update (the
application normally never sets it: the trigger overwrites it). Keep it current
with a trigger declared in `MomoConfig`, and index it with GIN:

<!-- docscheck:fragment -->

```go
type Article struct {
	momo.Model `momo:"table:kb_articles"`
	ID         int64            `momo:"pk,identity"`
	Title      string
	Body       string
	SearchTsv  *types.TSVector
	Embedding  vector.Vector `momo:"dim:1536"`
}

func (a *Article) MomoConfig(c *momo.Config) {
	c.Index("kb_articles_search_gin", &a.SearchTsv).Method("gin")
	c.Function(schema.Function{
		Name: "kb_articles_tsv", Returns: "trigger", Language: "plpgsql", Volatility: "VOLATILE",
		Body: "BEGIN NEW.search_tsv := setweight(to_tsvector('english', coalesce(NEW.title, '')), 'A') || " +
			"to_tsvector('english', coalesce(NEW.body, '')); RETURN NEW; END;",
	})
	c.Trigger(schema.Trigger{
		Table: "kb_articles", Name: "kb_articles_tsv_trg", Timing: "BEFORE",
		Events: []string{"INSERT", "UPDATE"}, ForEachRow: true, Function: "kb_articles_tsv()",
	})
}
```

## Expressions

The builders live in the package `github.com/azrtydxb/momo/fts`. They used to be
functions of the root package; see [upgrading](upgrading.md) for
the name table.

The text search configuration (`english`, `simple`, ...) is a string. It is
bound as a parameter and cast to regconfig, so it cannot carry SQL; an empty
one fails the build with `ErrValidation`.

| Builder                | SQL                                            | Result type                |
| ---------------------- | ---------------------------------------------- | -------------------------- |
| `fts.TSMatch`          | `vec @@ query`                                 | `Predicate`                |
| `fts.TSVector`         | `to_tsvector(config, text)`                    | `fts.Expr[types.TSVector]` |
| `fts.TSWebsearchQuery` | `websearch_to_tsquery(config, text)`           | `fts.Expr[types.TSQuery]`  |
| `fts.TSPlainQuery`     | `plainto_tsquery(config, text)`                | `fts.Expr[types.TSQuery]`  |
| `fts.TSPhraseQuery`    | `phraseto_tsquery(config, text)`               | `fts.Expr[types.TSQuery]`  |
| `fts.TSQuery`          | `to_tsquery(config, text)`                     | `fts.Expr[types.TSQuery]`  |
| `fts.TSRank`           | `ts_rank(vec, query [, normalization])`        | `fts.Expr[float64]`        |
| `fts.TSRankCD`         | `ts_rank_cd(vec, query [, normalization])`     | `fts.Expr[float64]`        |
| `fts.TSHeadline`       | `ts_headline(config, text, query [, options])` | `fts.Expr[string]`         |

The query text, the configuration and the headline options are always bound
parameters. The normalization argument of the rank functions is the optional
PostgreSQL flag mask (0 to 63) and renders as a literal. `fts.TSWebsearchQuery`
accepts any user input without raising a syntax error, so it is the builder for
a search box; `fts.TSQuery` expects tsquery syntax and a malformed query is a
database error.

An `fts.Expr` has `As`, `Asc`, `Desc` and the comparison methods of any scalar
expression, so it works in Where, OrderBy and Columns:

<!-- docscheck:fragment -->

```go
q := fts.TSWebsearchQuery("english", userInput)
rank := fts.TSRankCD(articleSearch, q)

hits, err := momo.Select[Hit](db).
	From(momo.TableRef{Name: "kb_articles"}).
	Columns(articleID, rank.As("rank"),
		fts.TSHeadline("english", articleBody, q, "StartSel=<b>, StopSel=</b>, MaxFragments=2")).
	Where(fts.TSMatch(articleSearch, q)).
	OrderBy(rank.Desc()).
	Limit(20).
	All(ctx)
```

Here `articleSearch` is a `Column[types.TSVector]` and `articleBody` a
`Column[string]`. Without a stored column, search a text column directly with
`fts.TSVector` over `articleBody` as the first operand of `fts.TSMatch` (an
expression index on the same expression keeps it fast).

Every use of a call binds its own parameters (the configuration and the query
text), so the same expression written in the select list and again in GROUP BY
gets different placeholders, and PostgreSQL rejects the statement as an
ungrouped column. The same applies to SELECT DISTINCT with an ORDER BY on the
call. Group by a stored tsvector instead, or compute the call once in a
subquery or CTE and refer to its alias. A golden test pins the placeholders.

Arguments bound by these calls are masked in hook events (shown as
`RedactedArg`) when an operand reads a column marked sensitive; the driver
still receives the real values.

## Hybrid ranking: vector plus text

For reciprocal rank fusion of the two signals, use `momo.HybridSearch` (see
[hybrid search](vectors.md#hybrid-search)). To weigh the signals yourself,
combine them in one score with the typed arithmetic helpers. The cosine
distance is lower for better matches and the text rank is higher, so subtract a
weighted rank from the distance and order ascending:

<!-- docscheck:fragment -->

```go
dist := vecop.Cosine(articleEmb, queryEmbedding) // vector.Vector
text := fts.TSRankCD(articleSearch, fts.TSWebsearchQuery("english", userInput))
score := sqlfn.SubExpr[float64](dist, sqlfn.Mul[float64](text, 0.5))

hits, err := momo.Select[Hit](db).
	From(momo.TableRef{Name: "kb_articles"}).
	Columns(articleID, score.As("score"), articleTitle).
	OrderBy(score.Asc()).
	Limit(20).
	All(ctx)
```

This scores every row. To restrict the candidate set to text matches, add
a Where on `fts.TSMatch` over `articleSearch` and the same query, which lets the GIN
index prune the rows before the vector distance is computed. The
`TestFullTextHybridRanking` integration test runs this pattern.

## Scanning

pgx returns tsvector and tsquery values as text, which `types.TSVector` and
`types.TSQuery` scan. `ts_rank` and `ts_rank_cd` return real; they scan into
`float64` fields.
