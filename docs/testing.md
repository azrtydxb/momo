# Testing applications with momotest

`github.com/azrtydxb/momo/momotest` gives your tests a real PostgreSQL
database, isolated per test, with your project's migrations already applied. It
replaces the hand-written scratch-database helper that every application would
otherwise copy.

Do not mock `momo.DB`, `momo.Tx` or `momo.Querier`. The query builders are the
code under test; the constraints and the SQL are what you want to exercise.

## Setup

1. Point the tests at a server you may create schemas on:

   ```sh
   export MOMO_TEST_POSTGRES_URL=postgres://user:pass@localhost:5432/scratch
   ```

   The value must be a `postgres://` or `postgresql://` URL. Use a database you
   do not mind filling with temporary schemas; a disposable container is
   ideal.

2. Embed the migrations in the package that owns the schema, so the binary and
   its tests share them:

   <!-- docscheck:fragment -->

   ```go
   //go:embed migrations
   var files embed.FS

   func Migrations() fs.FS { sub, _ := fs.Sub(files, "migrations"); return sub }
   ```

3. Keep tests that use the kit behind the `integration` build tag (or one of
   your own), and run them with `-tags integration`. A plain `go test` then
   never touches a database.

When the variable is unset, the test is skipped. Set `MOMO_REQUIRE_INTEGRATION=1`
in CI and the test fails instead, so a misconfigured job cannot pass by skipping
everything. `momotest.WithEnv` reads a different variable.

## One database per test: New

<!-- docscheck:fragment -->

```go
func TestSignup(t *testing.T) {
	t.Parallel()
	db := momotest.New(t, momotest.WithMigrations(app.Migrations()))
	// db is a *momo.DB on a schema no other test can see.
}
```

`New` creates a schema, applies the migrations to it and returns a `*momo.DB`.
When the test ends the pool is closed and the schema is dropped. Code under test
may open and commit its own transactions. Use this mode when it does.

## A rollback per test: NewTx

<!-- docscheck:fragment -->

```go
func TestOverdraft(t *testing.T) {
	t.Parallel()
	tx := momotest.NewTx(t, momotest.WithMigrations(app.Migrations()))
	// tx is a *momo.Tx; everything written through it is gone after the test.
}
```

The migrations are applied once to a template schema, named after a hash of the
migration files, the extension list and the momo version, and kept on the
server between runs. The first `momotest.NewTx` in a test binary builds it (or finds it
ready) under an advisory lock, over a short-lived administrator connection;
after that success the binary remembers it, and each later test only opens its
own small pool and begins a transaction, which is much cheaper than `New`. A
changed migration gets a new template; old `momotest_tpl_*` schemas can be
dropped at any time between runs.

The `Tx` is adopted: `Commit` and `Rollback` on it return an error matching
`momo.ErrAdoptedTx`, so a test cannot make its writes permanent. `tx.Tx` opens
savepoints, so you can test a failing step and keep going. Two limits follow
from using one transaction per test:

- Code that must commit, or that needs two connections to see each other's work,
  belongs in a test that uses `New`.
- Parallel tests do not see each other's rows, but two that insert the same
  unique key wait for each other until one finishes. Give parallel tests
  distinct keys.
- DDL, `TRUNCATE` and `LOCK TABLE` inside a `momotest.NewTx` test take locks that block
  every other parallel test using the same template until this one ends. Run
  such tests with `New`.
- Identity and sequence values are consumed by a rolled-back transaction, so
  never assert a literal generated id; compare with the id that `Insert` read
  back.

## Migrations must stay in their schema

Migrations run in a schema of the test's own, with `public` after it on the
search path. Write them unqualified. `momotest` refuses early, with the file
name, a migration that contains `CREATE SCHEMA`, or `CREATE EXTENSION` for an
extension not listed in `momotest.WithExtensions`: an extension exists once per
database, so it would land in one test's schema and vanish under the others when
that schema is dropped. List extensions in `momotest.WithExtensions` (names are matched
in lower case); they are installed once in `public`.

The check reads the SQL with comments and string literals removed, so a comment
that mentions `create schema` is fine and `CREATE /* x */ EXTENSION foo` is still
caught. Quoted identifiers are kept. Dollar-quoted bodies are checked as code,
because a `DO` block can run `CREATE EXTENSION`: the cost is that text inside a
dollar-quoted string that looks like that statement is also refused, with a
message naming the file. Move such text into a normal string, or list the
extension.

`momotest.NewTx` remembers the result of the check and the template name per migration
`fs.FS` value and extension list. Build the FS once, in a package variable or an
`embed.FS`, so that the files are walked once; an FS rebuilt on every call (such
as `fs.Sub` inside a function) is re-read each time, which still works. Only
successes are remembered, so a failed read fails one test and the next one tries
again. Changing the files behind a remembered FS while the test binary runs, for
example rewriting the directory an `os.DirFS` points at, is not supported.

## Leftover schemas

Cleanup runs in `t.Cleanup`, so a killed test run (`kill -9`, a CI timeout)
can leave `momotest_<hex>` schemas behind. They are harmless but accumulate. Drop
them on a disposable server by recreating it, or with
`DROP SCHEMA momotest_<hex> CASCADE` for each name in `pg_namespace` that matches
`momotest\_%` and is not `momotest_tpl_%`. Template schemas `momotest_tpl_*` are
meant to stay. `momotest` has no sweep helper: it never drops a schema it did
not create in the current test.

## Fixtures: Seed

<!-- docscheck:fragment -->

```go
org := &models.Org{Name: "acme"}
momotest.Seed(t, tx,
	momotest.Insert(org),
	momotest.Insert(&models.User{OrgID: org.ID, Email: "ada@acme.test"}),
)
```

`momotest.Seed` runs its steps in the order given, so list a table before the tables that
reference it. All steps run in one transaction (a savepoint when the querier is
a `*momo.Tx`); if one fails, nothing is kept and the test fails. `Insert` reads
generated columns, such as identity keys, back into the rows you pass, so later
steps can use them. `momotest.Graph` inserts a root with the related rows it includes, as
`momo.CreateGraph` does. A step is any `func(ctx, momo.Querier) error`, so
custom fixtures fit too.

## Fixtures from a file: LoadFixtures

`momotest.LoadFixtures(t, q, path)` inserts the rows of a YAML file into `q`, a
`*momo.DB` or a `*momo.Tx`, for data that is easier to read as a file than as Go
structs. The file maps a table name to a list of rows, and each row maps a column
name to a value:

```yaml
authors:
  - id: 1
    name: Ada
documents:
  - id: 1
    author_id: 1
    title: First
  - id: 2
    author_id: null
    title: Orphan
```

Tables are inserted in file order and the rows of a table in list order, so
list a table before the tables that reference it. Columns are written sorted by
name and a YAML `null` becomes SQL `NULL`. Table and column names are quoted
with `momo.Ident` and every value is a bound argument, so a value is never
spliced into the SQL. A table name is one identifier, with no schema prefix.
A value is a string, integer, float, bool, null, timestamp or `!!binary` (read as
bytes); a nested map or list is refused. `momotest.LoadFixtures` writes plain `INSERT`
statements and is tested on PostgreSQL and SQLite; it has not been run against
MySQL or MariaDB. A file that is not a mapping of
tables to lists of rows, a missing file, and a row the database refuses (a
foreign key to a row that is not there, say) fail the test with a message that
names the file, the table and the row. Each row is its own statement, so rows
before a failing one stay unless you pass a transaction.

## SQLite without a server: sqlitetest

`github.com/azrtydxb/momo/momotest/sqlitetest` gives a test a SQLite database
with sqlite-vec loaded, without a server and without an environment variable:
`sqlitetest.New(t, opts...)` returns a `*momo.DB` on a new file in the test's
own temporary directory. Every call opens a file of its own, so two databases in
one test never see each other's tables and parallel tests do not collide. The
database is closed and the file removed when the test ends. The options go to
`momo.New`, for example `momo.WithHook`, and `momotest.LoadFixtures` fills it like any
other database. The database starts empty: create the tables with your own DDL,
for example `momo.SQLiteVectorType` for a vector column (see
[Vector search](vectors.md)); `momotest.WithMigrations` and `momotest.NewTx` are
PostgreSQL features and do not apply.

SQLite lives in its own package, and not in `momotest`, because the SQLite
driver is compiled from C sources: a test binary that imports only `momotest`
for PostgreSQL does not build them, and a repository test
(`TestMomotestAvoidsSQLite`) keeps it that way. Only a test that imports
`sqlitetest` pays for them. The driver needs cgo and a non-Windows target; on
any other build `sqlitetest.New` skips the test, the way `momotest.New` skips
when the server URL is unset.

## Options

| Option                           | Effect                                                                                          |
| -------------------------------- | ----------------------------------------------------------------------------------------------- |
| `momotest.WithMigrations(fs.FS)` | Apply these migrations. The directory needs no `momo.sum`. Without it the database is empty.    |
| `momotest.WithEnv(name)`         | Read the server URL from another variable.                                                      |
| `momotest.WithExtensions(names)` | `CREATE EXTENSION IF NOT EXISTS` in `public` first; with `vector` the pool registers the codec. |
| `momotest.WithMomoOptions(opts)` | Passed to `momo.New`, for example a hook.                                                       |
| `momotest.WithMaxConns(n)`       | Pool size per test: 4 for `New`, 2 for `momotest.NewTx` by default.                             |

## Concurrency

Every call opens its own connection pool and, in `New`, its own schema, so
`t.Parallel` and `-race` are safe. The one shared step, creating extensions and
building a template, runs under a PostgreSQL advisory lock (keyed by
`hashtext` of the names `momotest:extensions` and `momotest:template`), so tests
in one binary and several test binaries running at once (`go test ./...`)
cooperate. `New` also opens a short administrator connection to create and, at
cleanup, drop its schema. A `momotest.NewTx` test holds one connection for its
transaction, plus the pool's spare. Each parallel test holds a few connections; keep the server's
`max_connections` above the number of parallel tests times the pool size, or
lower the pool with `momotest.WithMaxConns` or the parallelism with `-parallel`.

## Example

[examples/testing](../examples/testing/) runs a small bank-account service
through both modes, in parallel, with seeded fixtures. Its test is the
reference for how to write yours.
