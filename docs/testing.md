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
everything. `WithEnv` reads a different variable.

## One database per test: New

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

```go
func TestOverdraft(t *testing.T) {
	t.Parallel()
	tx := momotest.NewTx(t, momotest.WithMigrations(app.Migrations()))
	// tx is a *momo.Tx; everything written through it is gone after the test.
}
```

The migrations are applied once to a template schema, named after a hash of the
migration files, and kept on the server between runs. Each test only begins a
transaction on it, which is much cheaper than `New`. A changed migration gets a
new template; old `momotest_tpl_*` schemas can be dropped at any time.

The `Tx` is adopted: `Commit` and `Rollback` on it return an error matching
`momo.ErrAdoptedTx`, so a test cannot make its writes permanent. `tx.Tx` opens
savepoints, so you can test a failing step and keep going. Two limits follow
from using one transaction per test:

- Code that must commit, or that needs two connections to see each other's work,
  belongs in a test that uses `New`.
- Parallel tests do not see each other's rows, but two that insert the same
  unique key wait for each other until one finishes. Give parallel tests
  distinct keys.

## Fixtures: Seed

```go
org := &models.Org{Name: "acme"}
momotest.Seed(t, tx,
	momotest.Insert(org),
	momotest.Insert(&models.User{OrgID: org.ID, Email: "ada@acme.test"}),
)
```

`Seed` runs its steps in the order given, so list a table before the tables that
reference it. All steps run in one transaction (a savepoint when the querier is
a `*momo.Tx`); if one fails, nothing is kept and the test fails. `Insert` reads
generated columns, such as identity keys, back into the rows you pass, so later
steps can use them. `Graph` inserts a root with the related rows it includes, as
`momo.CreateGraph` does. A step is any `func(ctx, momo.Querier) error`, so
custom fixtures fit too.

## Options

| Option                  | Effect                                                                                          |
| ----------------------- | ----------------------------------------------------------------------------------------------- |
| `WithMigrations(fs.FS)` | Apply these migrations. The directory needs no `momo.sum`. Without it the database is empty.    |
| `WithEnv(name)`         | Read the server URL from another variable.                                                      |
| `WithExtensions(names)` | `CREATE EXTENSION IF NOT EXISTS` in `public` first; with `vector` the pool registers the codec. |
| `WithMomoOptions(opts)` | Passed to `momo.New`, for example a hook.                                                       |
| `WithMaxConns(n)`       | Pool size per test: 4 for `New`, 2 for `NewTx` by default.                                      |

## Concurrency

Every call opens its own connection pool and, in `New`, its own schema, so
`t.Parallel` and `-race` are safe. The one shared step, creating extensions and
building a template, runs under a PostgreSQL advisory lock, so tests in one
binary and several test binaries running at once (`go test ./...`) cooperate.
Each parallel test holds a few connections; keep the server's
`max_connections` above the number of parallel tests times the pool size, or
lower the pool with `WithMaxConns` or the parallelism with `-parallel`.

## Example

[examples/testing](../examples/testing/) runs a small bank-account service
through both modes, in parallel, with seeded fixtures. Its test is the
reference for how to write yours.
