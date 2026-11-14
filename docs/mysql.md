# MySQL and MariaDB

What works on MySQL and MariaDB today, and what does not. The driver is
`driver/mysqldriver` (database/sql on `github.com/go-sql-driver/mysql`).
The migration dialect `migrate/mysql` runs from the `momo migrate` command
with `--dialect mysql` (see [Migrations](#migrations) and the generated status
in [capabilities](capabilities.md)).

## Supported servers

| Server           | Version | Image used in CI |
| ---------------- | ------- | ---------------- |
| MySQL 8.4 LTS    | 8.4.13  | `mysql:8.4.13`   |
| MySQL 9.7 LTS    | 9.7.4   | `mysql:9.7.4`    |
| MariaDB 11.8 LTS | 11.8.9  | `mariadb:11.8.9` |

No innovation releases are supported. CI runs the shared suites against all
three servers (the `integration-mysql` job), on every push.

## What runs on a real server

The `integration-mysql` CI job starts one server per row of the table above and
runs the whole integration suite against it with `MOMO_REQUIRE_INTEGRATION=1`,
so a server that is missing fails the job instead of skipping the tests.
Tests that use `testopen.EachWithMySQL` or `testopen.EachMySQL` run on all
three servers; the plain `testopen.Each` opens PostgreSQL and SQLite only.

Run on MySQL 8.4, MySQL 9.7 and MariaDB 11.8:

- the shared suites: transactions, errors, joins, aggregates, subqueries and
  CTEs, RETURNING, raw SQL, locking, hints, keyset pagination, writes, soft
  delete, optimistic lock, upserts and timestamps;
- `jsonop.Text`, `jsonop.Path`, `jsonop.Root`, `jsonop.Contains`,
  `jsonop.HasKey` and `jsonop.DeleteKey`, with the build error of
  `jsonop.Concat` and `jsonop.Set`;
- window functions (ranking, `Lag`, `Lead`, `FirstValue`, `LastValue`, `Rows`
  and `Range` frames), the build error for a `Groups` frame, the aggregate
  `FILTER` clause (rewritten to `CASE WHEN`, with and without `Over` and in
  `HAVING`), and `LoadPerParentLimit`;
- keyset pagination over string, descending and mixed keys, `Batches`, and
  `SelectQuery.Each` and `SelectQuery.Iter`, including an early stop, a query
  while the cursor is open, a canceled context and a stream inside a `Tx`;
- typed predicates, row-value comparison, projections, select-list aliases,
  the `sqlfn` string, CASE, cast and arithmetic helpers, `sqlfn.CurrentDate`,
  the `agg` aggregates and bulk-insert chunking with `WithMaxParams`;
- every `timeop` extract field, truncation unit and interval, `timeop.Now`,
  `timeop.EpochSeconds` and the typed date arithmetic.

`LATERAL` runs on MySQL 8.4 and 9.7, and on MariaDB it is `ErrUnsupported`.
`CreateGraph` and `FindOrCreate` work on all three (see below), and a test
checks the stored keys and the found and created cases on each server.
`SelectQuery.Explain`, the aggregate `FILTER` clause on `agg.JSONB`,
`agg.Array`, an ordered or distinct `agg.JSONB`, `unnest` and `generate_series` sources and `DISTINCT ON` are `ErrUnsupported`;
the tests check the error on each server.

Differences the tests pin down:

- a JSON null member extracts as the text `null`, not as SQL NULL, because
  `JSON_UNQUOTE` turns it into text;
- `sqlfn.Cast` writes the type name as given, and MySQL and MariaDB cast to
  `SIGNED`, not `bigint`;
- `momo.MapRows` returns text columns as `[]byte`, the driver's own type;
- an aggregate over `JSON_ARRAYAGG` has no defined member order.

Not run on MySQL or MariaDB: the one-million-row heap bound of streaming
(its seed SQL is PostgreSQL and SQLite only), and the CreateGraph behaviour
tests of the shared suite whose SQL is written for PostgreSQL alone. The graph writes
have their own tests in `driver/mysqldriver` and `rel_mysql_integration_test.go`.

## Connecting

Open the pool with `mysqldriver.Open(dsn, flavor)`, or build the DSN with
`mysqldriver.ConfigureDSN` and pass the `*sql.DB` to `mysqldriver.New`. The
configured DSN always has:

- `parseTime=true` and `loc=UTC`, and the session `time_zone` set to `+00:00`;
- `clientFoundRows=true`, so an UPDATE reports the rows it matched;
- `interpolateParams=false` and `multiStatements=false`: values always travel
  as bound parameters of a prepared statement and never become SQL text. A DSN
  that sets either is overridden. A `*sql.DB` handed to `New` directly must
  already be configured this way.

`Open` pings the server, as the SQLite drivers do, so a bad DSN, a refused
connection or bad credentials fail at `Open` and not on the first statement.
An unknown flavor makes `Open` return `ErrUnsupported`; on any error the pool
is closed and nothing is returned. (`New`, which takes an already opened
`*sql.DB`, still returns a driver whose every call is `ErrUnsupported` for an
unknown flavor, because it has no error to return.)

`ConfigureDSN` (and so `Open`) refuses two DSN settings with `ErrValidation`:

- `allowAllFiles=true`, which lets the server read any client file through
  `LOAD DATA LOCAL INFILE`;
- `allowCleartextPasswords=true`, which sends the password unhashed.

Remove them from the DSN. The error message never quotes the DSN.

Transport security is the DSN's business and defaults to none: add `tls=true`
(or a registered TLS config) for any server that is not on the same host.

## Capabilities

The rows below are checked against `dialect.MySQLFlavor` by
`TestCapabilityMatrixMatchesDialect`.

| Capability       | mysql | mariadb    |
| ---------------- | ----- | ---------- |
| Returning        | no    | no         |
| InsertReturning  | no    | yes        |
| OnConflict       | no    | no         |
| OnDuplicateKey   | yes   | yes        |
| NullsOrdering    | no    | no         |
| TransactionalDDL | no    | no         |
| Merge            | no    | no         |
| ILike            | no    | no         |
| Locking          | yes   | yes        |
| RowValues        | yes   | yes        |
| Savepoints       | yes   | yes        |
| Vector.Storage   | yes   | yes        |
| Vector.Metrics   | none  | l2, cosine |
| Vector.Indexes   | none  | vector     |

`NullsOrdering` and `ILike` mean native syntax, not availability. Both
features work on MySQL and MariaDB through emulation (see
[Emulated features](#emulated-features)), so the table says `no` while
`momo.ILike`, `NullsFirst` and `NullsLast` run.

`Returning` is the UPDATE and DELETE form. MariaDB 11.8 can return rows from an
INSERT only (`INSERT ... RETURNING`), which is why it has `InsertReturning`
set while `Returning` stays false: `Returning` on an insert works on MariaDB,
and on an update or delete it is `ErrUnsupported` on both servers. MySQL has
no `RETURNING` at all.

## Emulated features

Three PostgreSQL features have no MySQL or MariaDB spelling. Momo renders an
equivalent instead of returning `ErrUnsupported`.

- **ILIKE.** `momo.ILike(col, pattern)` renders
  `LOWER(col) LIKE LOWER(?) ESCAPE '!'`, and `momo.Not(momo.ILike(...))` negates
  it. `LOWER` folds Unicode letters by the character set, but the `LIKE`
  comparison follows the column collation. The default collations of MySQL 8.4
  and 9.7 (`utf8mb4_0900_ai_ci`) and MariaDB 11.8 are accent-insensitive, so `a`
  matches `ä`, which PostgreSQL `ILIKE` does not. Momo does not force a
  collation, because the accent-insensitive `_ai_` family is the thing it must
  not add. Because `LOWER` wraps the column, an index on the column is not
  used. The column must be a character type: on `BINARY`, `VARBINARY` and `BLOB`
  columns `LOWER` does nothing and `LIKE` is case-sensitive, so the match is
  case-sensitive there (momo adds no `CONVERT`).
- **NULLS FIRST and NULLS LAST.** `NullsFirst` renders
  `(e IS NULL) DESC, e ASC|DESC` and `NullsLast` renders
  `(e IS NULL) ASC, e ASC|DESC`, which orders exactly as PostgreSQL. `e` is
  rendered twice, so values it binds appear twice among the arguments, in
  order. `After` over a nullable key stays `ErrUnsupported`, and the outer
  `OrderBy` of a union with a NULLS term is `ErrUnsupported`, and so is a window
  with a `Range` frame that has an offset and a single `OrderBy` item with a
  NULLS term (the emulation adds a second sort key, and the server rejects that
  with error 3587).
- **Aggregate FILTER.** `agg(x) FILTER (WHERE p)` renders
  `agg(CASE WHEN p THEN x END)`, and `COUNT(*) FILTER (WHERE p)` renders
  `COUNT(CASE WHEN p THEN 1 END)`. It is exact for `Sum`, `Avg`, `Min`, `Max`,
  `CountOf`, `CountDistinct` and `agg.String`, and works with `Over`. A call
  that keeps NULLs (`agg.JSONB`, any `agg.Func` outside `count`, `sum`, `avg`,
  `min`, `max` and the `std` and `var` family, or one over a raw `*`) stays
  `ErrUnsupported`; use `momo.CountAll().Filter(p)` for `COUNT(*)`.

## Writes

- A single `Model` insert reads `LAST_INSERT_ID` and writes it into the
  identity field of the model. A key generated on the Go side (an id
  generator) always wins; `LAST_INSERT_ID` is read only for a key the database
  generates. A plain multi-row `Models` insert through `Exec` returns and
  writes back no keys, because the ids of a multi-row statement depend on
  `innodb_autoinc_lock_mode`. On MariaDB, ask for them with `Returning`; on
  MySQL that is `ErrUnsupported`. The `LAST_INSERT_ID` write-back does not
  apply to an upsert or to any insert with a conflict clause
  (`wantsGeneratedKey` in the core is false for them), because the statement
  may update an existing row and `LAST_INSERT_ID` then does not name the row's
  key; read the row back with a query.
- `momo.CreateGraph` reads every generated key back on both servers. MariaDB
  uses `INSERT ... RETURNING` (`InsertReturning`), also for the children of a
  has-many or many-to-many relation, in one multi-row statement. MySQL has no
  `RETURNING`, so it reads `LAST_INSERT_ID` after each statement and inserts
  the children one row per statement inside the graph's transaction (a
  savepoint on a `Tx`). The keys of one multi-row `INSERT` are consecutive only
  when `innodb_autoinc_lock_mode` is 0 or 1; MySQL 8 and 9 default to 2, where
  concurrent inserts can interleave, so momo never infers keys from a
  multi-row statement and an `ON DUPLICATE KEY UPDATE` statement is never used
  to read keys. The rows share one statement clock, so they get the same
  automatic timestamps as a bulk insert. Children that mix zero and explicit
  keys in one bulk insert fail with `ErrValidation` on MySQL, and a generated
  key on a non-integer field is `ErrUnsupported`. CI runs this on MySQL 8.4 and
  9.7 (mode 2) and MariaDB 11.8.
- `momo.FindOrCreate` works on both servers inside a transaction (a savepoint
  on a `Tx`): a plain `INSERT` (with `RETURNING` on MariaDB, `LAST_INSERT_ID`
  on MySQL), and when it fails with a unique violation, a shared-lock select
  by key (`FOR SHARE`, `LOCK IN SHARE MODE` on MariaDB) that returns the stored
  row, so the Go-side key still wins for a new row. A no-op
  `ON DUPLICATE KEY UPDATE` cannot be used: with `clientFoundRows` an inserted
  row and an untouched existing row both report one affected row, and MariaDB's
  `RETURNING` returns the row either way, so `created` could not be told. The
  price is one failed `INSERT` plus one `SELECT` when the row exists, and the
  failed insert uses up an auto-increment value. Shared locks do not conflict
  with each other, but if the first inserter rolls back instead of committing,
  the waiting callers can deadlock (error 1213, `ErrSerialization`) and one
  retries. On a caller's `Tx`, InnoDB keeps the locks taken after a savepoint
  until the outer transaction ends. A unique violation on another
  key than the one given is still `ErrUniqueViolation`; a `DB` pinned with
  `OnConn` is `ErrUnsupported` (it cannot open the transaction).
- Upserts render as `INSERT ... VALUES ... AS momo_new ON DUPLICATE KEY UPDATE`
  on MySQL and with `VALUES(col)` on MariaDB. A `FromSelect` upsert has no row
  alias on either: MySQL and MariaDB both render `VALUES(col)` there. The
  `updated_at` bump of a `DoUpdate` is a bound clock value, so it needs neither
  form. `DoNothing` renders a no-op
  assignment of a key column. A `Where` on a conflict clause is
  `ErrUnsupported`.
- `ON DUPLICATE KEY UPDATE` fires for a conflict on any unique key and cannot
  name one. Momo therefore accepts a conflict target only when it is a unique
  key of the table and no other unique key can conflict. A second key is safe
  when it is made only of auto-increment columns the insert leaves to the
  database: the server gives such a column a fresh value, so the key cannot
  match an existing row. The case this allows is an auto-increment primary key
  next to `UNIQUE(email)`, upserting on `email` with the primary key left out
  (judged from the columns the statement writes: the identity is not in the
  model columns, the `InsertQuery.Values` assignments or the `FromSelect`
  target list, and no Go-side id generator fills it). In every other case the call is `ErrUnsupported` and the message
  names the reason: a second unique key the insert can reach (an ordinary
  column, an expression, a composite key with an ordinary column, or the
  primary key written explicitly), or a target that is not a unique key. The
  unique keys come from the model's schema (its `pk` and `unique` tags and
  unique indexes); without a schema (a hand-written model) only a primary-key
  target is accepted.
- With `clientFoundRows`, an upsert reports 1 for an inserted row, 2 for an
  existing row that changed, and 1 for an existing row set to its current
  values. `DoNothing` also reports 1 for a skipped row. A count of 1 does not
  tell an insert from a skip.
- A bare `OFFSET` gets `LIMIT 18446744073709551615` in front of it, because
  MySQL and MariaDB reject an offset without a limit.

## Locking and transactions

`ForUpdate` and `ForShare` render `FOR UPDATE` and `FOR SHARE`; `NOWAIT` and
`SKIP LOCKED` are passed through. MariaDB 11.8 has no `FOR SHARE`, so it gets
`LOCK IN SHARE MODE`. `LockOf` (`FOR UPDATE OF`) is MySQL only; on MariaDB it is
`ErrUnsupported`. Savepoints and the four standard isolation levels work;
read-only transactions use `START TRANSACTION READ ONLY`. Any other isolation
level is `ErrUnsupported`. `ForNoKeyUpdate` and `ForKeyShare` are PostgreSQL
only and return `ErrUnsupported` here. `timeop.ClockTimestamp` renders
`SYSDATE(6)`; `timeop.StatementTimestamp` is `ErrUnsupported`.

## Errors

Server errors map by number: 1062 unique, 1451 and 1452 (and 1216, 1217)
foreign key, 1048 and 1364 not null, 3819 (MySQL) and 4025 (MariaDB) check,
1213 and MariaDB 1020 serialization (deadlock, and a row that changed under
`innodb_snapshot_isolation`), 1205 and 3572 busy. `Error()` never shows the
server text, which can quote a row value; `Detail()` returns it, and
`errors.As` reaches the `*mysql.MySQLError`. The constraint name differs by
server: MySQL 8 reports a unique key as `table.index`, MariaDB reports only the
index name.

## Vectors

MySQL 8.4 has no vector columns. MySQL 9.7 stores `VECTOR` and reads it back,
but offers no distance function in the Community edition (`DISTANCE()` is
HeatWave and MySQL AI only), so every distance builder is `ErrUnsupported` on
MySQL. MariaDB 11.8 renders `vecop.L2` as `VEC_DISTANCE_EUCLIDEAN` and
`vecop.Cosine` as `VEC_DISTANCE_COSINE` on `vector.Vector` columns, and its
driver reports `Vectors`. Inner product, L1, Hamming, Jaccard and the half,
sparse and bit types return `ErrUnsupported` on MariaDB. Details and the
`VECTOR INDEX` rules are in [vectors.md](vectors.md#mariadb-and-mysql).

## Migrations

The library dialect `migrate/mysql` inspects `information_schema`, renders DDL,
locks with `GET_LOCK` and runs every migration, up and down, statement by
statement, because DDL commits implicitly on these servers. Build the engine
with `migrate.WithDialect` and the `mysqldriver` driver; the inspector, drift
check and `-- momo:ddl` online DDL options work through it. The `momo migrate`
command does the same for you: `--dialect mysql`, with a `mysql://` or
`mariadb://` URL or a go-sql-driver DSN (one dialect for both servers; the
server tells them apart). A string without `size:N` is `text`, and `text`,
`blob` and `json` columns cannot be keyed: the generator refuses a model with
one in a primary key, unique key, index or foreign key, naming the column, so a
migration never fails half-way on error 1170. `types.Numeric` with a `precision`
tag is `decimal(p,s)` (precision up to 65, scale up to 30, scale not above
precision; no `NaN` or infinity), and a `vector.Vector` column is `vector(N)`,
which needs MySQL 9.0 or MariaDB 11.7. What the dialect reports and renders is in
[migrations.md](migrations.md#9-mysql-and-mariadb).

## Models from a database

`momo generate --from-db` reads a MySQL 8.4, MySQL 9.7 or MariaDB 11.8
database and writes the models and the init snapshot, as it does for
PostgreSQL. Set `dialects: [mysql]` in `momo.yaml` (MariaDB is the same
dialect; the flavor is read from the server). The command reuses the
`migrate/mysql` inspector, so what `momo migrate drift` sees is what the models
declare.

The value of `--from-db` is `-` for standard input, the name of an environment
variable that holds the connection, or the connection itself, in one of these
forms:

```text
user:pass@tcp(host:3306)/app          a go-sql-driver DSN
tcp(host:3306)/app                    the same without credentials
user:pass@unix(/run/mysqld.sock)/app  a unix socket (DSN only)
mysql://user:pass@host:3306/app       a URL, also mariadb://
```

The DSN must name the database. A URL may carry only `tls`, `timeout`,
`readTimeout`, `writeTimeout`, `charset`, `collation`, `maxAllowedPacket` and
`allowNativePasswords`; any other parameter is refused, because the server
would take it as a session variable. Use a DSN for sockets and every other
driver option. A DSN or URL for a remote host with no `tls=` setting prints a
warning, and the password, in either form, is redacted from every message.

## What a rebuilt database matches

A database rebuilt from the generated models (diff the models against an empty
database and apply) has the same schema as far as `schema.Equal` compares it:
columns, types, defaults, generated columns, keys, indexes, checks and foreign
keys. The `integration-mysql` jobs check that on all three servers with a
schema built from raw DDL. It is not byte for byte. These are known losses; the
ones the command can see are reported, the others are not:

```text
ON UPDATE CURRENT_TIMESTAMP   reported as a warning; the rebuilt column does not change on UPDATE
INVISIBLE columns             reported as a warning (MySQL); the rebuilt column is visible
prefix indexes, name(10)      reported as a warning; left out of the models and the snapshot
prefix PRIMARY KEY            an error: nothing is written
table charset and collation   not read; a column collation is read only where it differs from the table's
ENGINE, ROW_FORMAT, table comment, AUTO_INCREMENT counter, partitions   not read
triggers, views, stored routines                                         not read
```

Type mapping (the column type is always written as a `type:` tag, because the
generator has no MySQL type table, except `varchar(N)`, which becomes `size:N`):

```text
tinyint(1)                    bool (a pointer when nullable)
tinyint, smallint, mediumint  int8, int16, int32 (uint8, uint16, uint32 when unsigned)
int, bigint                   int32, int64 (uint32, uint64 when unsigned)
float, double                 float32, float64
decimal(p,s)                  types.Numeric
char, varchar, *text          string
binary, varbinary, *blob, bit []byte
date                          types.Date
datetime, timestamp           time.Time
time                          string (the driver returns TIME as text)
year                          int16
json                          types.JSON[json.RawMessage]
enum, set                     string
```

- **tinyint(1) is a Go bool.** MySQL stores any value from -128 to 127 in it,
  and a stored value other than 0 or 1 (a 2 written by another program) fails
  to scan, for the whole row. Every `tinyint(1)` column gets a note in the
  report. If the column holds other values, add an `introspect.types` rule that
  maps `tinyint(1)` to `int8`.
- **enum and set** are Go strings. The exact column type, values included, is
  written in the `type:` tag, so the database keeps the value list and a
  rebuilt database matches. No check constraint is added and the values are not
  checked in Go. The tag grammar reads a comma inside parentheses as part of
  the option and skips a quoted value there, so `type:decimal(10,2)` and
  `type:enum('a,b','it''s')` are one option each.
- **AUTO_INCREMENT** keys get the `identity` tag.
- **Defaults:** every default is written (`default:`). A NOT NULL `bool` or
  number column whose constant default is `true` or non-zero is handled by
  flavor, because only MariaDB has `INSERT ... RETURNING`. On MariaDB the field
  gets `dbdefault` and a pointer type, so nil takes the default and an explicit
  `false` or `0` is written. On MySQL 8.4 and 9.7 an insert that leaves a
  `dbdefault` field nil is refused (nothing reads the stored value back), so the
  field stays plain and the report has a note: a zero value is written instead
  of the column default. A computed default such as `CURRENT_TIMESTAMP` gets no
  `dbdefault` on either.
- **Generated columns** become `read_only` fields with `generated:` (and
  `virtual` for VIRTUAL).
- **Constraints:** unique keys, plain, composite and `DESC` indexes, check
  constraints and foreign keys are written as on PostgreSQL. A foreign key to a
  primary key is a `belongs_to` field with `fk_name`, because MySQL names an
  unnamed key `<table>_ibfk_<n>`; `introspect.inverse` adds `has_many` and
  `has_one`. A foreign key to a unique column that is not the primary key is a
  `c.ForeignKey(...)` statement in `MomoConfig`, with no relation field.
- **Left out with a warning**, so the models and the snapshot agree and the
  baseline reports them: prefix indexes, foreign keys to a table with no model,
  and every object that PostgreSQL alone has (schemas, extensions, enum types,
  functions, triggers, row-level security, partitions), which MySQL does not
  have.
- **No Go mapping** (geometry, `vector`): an error, and nothing is written; add
  an `introspect.types` or `introspect.columns` rule.
- MariaDB stores `json` as `longtext` with a `json_valid` check; the inspector
  folds that back into `json`. MariaDB reports a literal default quoted and
  MySQL does not; both are written as the SQL literal.
