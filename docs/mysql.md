# MySQL and MariaDB

What works on MySQL and MariaDB today, and what does not. The driver is
`driver/mysqldriver` (database/sql on `github.com/go-sql-driver/mysql`).
Migrations and the inspector are **not available yet**; the
sections below say so where it matters.

## Supported servers

| Server           | Version | Image used in CI |
| ---------------- | ------- | ---------------- |
| MySQL 8.4 LTS    | 8.4.13  | `mysql:8.4.13`   |
| MySQL 9.7 LTS    | 9.7.4   | `mysql:9.7.4`    |
| MariaDB 11.8 LTS | 11.8.9  | `mariadb:11.8.9` |

No innovation releases are supported. CI runs the shared suites against all
three servers (the `integration-mysql` job), on every push.

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

Transport security is the DSN's business and defaults to none: add `tls=true`
(or a registered TLS config) for any server that is not on the same host.

## Capabilities

The rows below are generated from `dialect.MySQLFlavor` and checked by
`TestCapabilityMatrixMatchesDialect`.

| Capability       | mysql | mariadb |
| ---------------- | ----- | ------- |
| Returning        | no    | no      |
| InsertReturning  | no    | yes     |
| OnConflict       | no    | no      |
| OnDuplicateKey   | yes   | yes     |
| NullsOrdering    | no    | no      |
| TransactionalDDL | no    | no      |
| Merge            | no    | no      |
| ILike            | no    | no      |

`Returning` is the UPDATE and DELETE form. MariaDB 11.8 can return rows from an
INSERT only (`INSERT ... RETURNING`), which is why it has `InsertReturning`
set while `Returning` stays false: `Returning` on an insert works on MariaDB,
and on an update or delete it is `ErrUnsupported` on both servers. MySQL has
no `RETURNING` at all.

## Writes

- A single `Model` insert reads `LAST_INSERT_ID` and writes it into the
  identity field of the model. A key generated on the Go side (an id
  generator) always wins; `LAST_INSERT_ID` is read only for a key the database
  generates. A multi-row insert returns and writes back no keys, because the
  ids of a multi-row statement depend on `innodb_autoinc_lock_mode`. On MariaDB,
  ask for them with `Returning`; on MySQL that is `ErrUnsupported`.
- Upserts render as `INSERT ... VALUES ... AS momo_new ON DUPLICATE KEY UPDATE`
  on MySQL and with `VALUES(col)` on MariaDB. `DoNothing` renders a no-op
  assignment of a key column. A `Where` on a conflict clause is
  `ErrUnsupported`.
- `ON DUPLICATE KEY UPDATE` fires for a conflict on any unique key and cannot
  name one. Momo therefore accepts a conflict target only when it is the only
  unique key of the table, the primary key included; otherwise the call is
  `ErrUnsupported`. Without a schema (a hand-written model) only a primary-key
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
level is `ErrUnsupported`.

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
MySQL. MariaDB 11.8 renders `L2Distance` as `VEC_DISTANCE_EUCLIDEAN` and
`CosineDistance` as `VEC_DISTANCE_COSINE` on `vector.Vector` columns, and its
driver reports `Vectors`. Inner product, L1, Hamming, Jaccard and the half,
sparse and bit types return `ErrUnsupported` on MariaDB. Details and the
`VECTOR INDEX` rules are in [vectors.md](vectors.md#mariadb-and-mysql).

## Migrations

Not available yet. There is no `migrate/mysql` dialect, so `momo migrate`
rejects MySQL and MariaDB, and the inspector and drift checks do not run on
them. When they land, every migration will be non-transactional (DDL implies a
commit on these servers).
