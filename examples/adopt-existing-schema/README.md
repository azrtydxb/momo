# adopt-existing-schema

## What this shows

How to bring a database that was built with plain SQL under Momo, without
recreating it and without touching its data. The database in `legacy.sql` has
two tables and some rows, and no migration history.

The mechanism is `momo migrate baseline`. It records migrations as applied
without running them, after checking that the live database matches the
snapshot of the version you name.

## What Momo v0.5.0 does not do <!-- x-release-please-version -->

There is no command that reads a database and writes the Go models for it. You
write the models by hand to match the tables, as `models/models.go` does for
`legacy.sql`. The baseline check is what tells you whether you got them right.

## The steps

1. Write the models so they describe the existing tables exactly.
2. Generate the code and the first migration from them. Both work offline:

   ```sh
   momo generate
   momo migrate diff init
   ```

3. Baseline the database at that migration. The database must match the
   snapshot, or the command exits 1 and lists the kinds of object that differ:

   ```sh
   export DATABASE_URL=postgres://...
   momo migrate baseline --version <version of init>
   ```

4. From here on, change the models and use the normal workflow:

   ```sh
   momo migrate diff add_order_status
   momo migrate apply
   ```

Only migrations after the baseline version are pending. `init` is never run
against this database. A new environment, with no tables, runs `init` and gets
the same schema, so every database converges.

If your models declare views, functions, triggers or policies, the baseline also
needs the `CREATE` privilege on the database, because it builds a scratch
schema to compare their text. This example declares none.

## Making the models match

The legacy schema has to be spelled the way Momo reads it. In this example:

- Identity columns, because the models declare `identity`. A `serial` column
  is a different definition (an integer with a sequence default).
- `users.email` is a unique index (`CREATE UNIQUE INDEX`), not a `UNIQUE`
  constraint. The baseline check treats them as different objects.
- The primary key and foreign key constraints keep the names PostgreSQL gives
  them, which are the names the generator uses.

When the check fails, change the models to describe the database as it is. If
the difference is deliberate, such as a table Momo should ignore, add it to
`migrate.exclude` (a list of glob patterns) in `momo.yaml`, or pass `--force` to record the baseline anyway.

## CI

After baselining, the normal checks apply:

```sh
momo generate --check
momo migrate validate
```

`generate --check` fails when the models changed without `momo generate`.
`validate` checks the files in `migrations/` against `momo.sum`; if you edit one,
run `momo migrate hash` to accept it.

## Run it

```sh
go test ./examples/adopt-existing-schema
MOMO_TEST_POSTGRES_URL=postgres://... go test ./examples/adopt-existing-schema
```

Without `MOMO_TEST_POSTGRES_URL` the test skips. With it, the test loads
`legacy.sql` into a scratch database, shows the baseline refusing a database
with an extra column, baselines the clean one, applies the one later migration,
and checks that the legacy rows are intact and `drift` is clean. A second test
builds a database from the migrations alone.

To run the program against your own database:

```sh
DATABASE_URL=postgres://... momo migrate baseline --version <version of init>
DATABASE_URL=postgres://... momo migrate apply
DATABASE_URL=postgres://... go run ./examples/adopt-existing-schema
```
