# upgrade-database

## What this shows

How to change a model after a database is in production: add a column with a
default, add an index, rename a column, and apply the result to a database that
already holds rows. The data survives.

The schema has three versions. Each is one migration in `migrations/`, written
by `momo migrate diff` and committed with the models.

| Version | Change to the model                               | Migration     |
| ------- | ------------------------------------------------- | ------------- |
| 1       | `Customer` with `ID`, `Email`, `Name`             | `init`        |
| 2       | add `Plan`, `NOT NULL`, default `'free'`, indexed | `add_plan`    |
| 3       | `Name` is stored in the column `full_name`        | `rename_name` |

`models/models.go` holds the latest version.

## The workflow

Edit the model, then regenerate the code and write the migration:

```sh
momo generate
momo migrate diff add_plan
```

`diff` compares the models with the last snapshot in `migrations/`. It works
offline and never touches a database. Run it again with nothing changed and it
prints `no changes`.

A new `NOT NULL` column is safe on a table with rows only when it has a
default. Without one `diff` refuses. Do not rely on `--stage` in v0.12.0: it <!-- x-release-please-version -->
splits the change into add-nullable, backfill and `SET NOT NULL` steps, but the
`NOT NULL` is never enforced afterwards (known issue s2, story
`20261106-staged-not-null-is-never-enforced-after-diff-stage`). Give the column
a default instead, as `Plan` does here, or write the `SET NOT NULL` migration
yourself as a custom migration (see `examples/custom-sql-migration`).

A column rename needs to be named. Without it `diff` sees a dropped column and
a new one, and refuses the drop as destructive:

```sh
momo migrate diff rename_name --rename-column customers.name=full_name
```

Apply the migrations to the database that is already running:

```sh
export DATABASE_URL=postgres://...
momo migrate plan
momo migrate apply
```

`plan` lists the pending migrations. `apply` runs them in order and records each
one in the history table, so a second `apply` does nothing.

## Checks

```sh
momo generate --check
momo migrate validate
momo migrate drift
```

- `generate --check` writes nothing. It exits 1 and prints `stale: <path>` when
  a model changed and `momo generate` was not run. Run it in CI.
- `migrate validate` checks the files in `migrations/` against `momo.sum`
  without a database. Run it in CI too. `diff` updates `momo.sum` itself; after
  editing a migration file by hand, run `momo migrate hash` to accept the edit.
- `migrate drift` compares the live database with the last applied snapshot.
  It exits 1 and names what differs, for example a column someone added with
  `ALTER TABLE` outside Momo.

## Run it

```sh
go test ./examples/upgrade-database
MOMO_TEST_POSTGRES_URL=postgres://... go test ./examples/upgrade-database
```

Without `MOMO_TEST_POSTGRES_URL` the test skips. With it, the test creates a
scratch database, applies only version 1, inserts a customer, applies the rest
with the real `momo` binary, and checks the row, the default, the index, and
drift.

To run the program against your own database:

```sh
DATABASE_URL=postgres://... momo migrate apply
DATABASE_URL=postgres://... go run ./examples/upgrade-database
```
