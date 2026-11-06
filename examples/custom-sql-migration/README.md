# custom-sql-migration

## What this shows

Hand-written SQL migrations next to generated ones. Momo v0.1.0 supports them
as plain files: there is no `momo migrate new --custom` command yet (it reports
"not available yet"), so you create the files yourself.

The story: add an `email_norm` column, fill it in for the rows that already
exist, then put a unique index on it. A generator can write the column and the
index, but not the data fill between them.

| File                        | Written by          | Does                                 |
| --------------------------- | ------------------- | ------------------------------------ |
| `..._init`                  | `momo migrate diff` | creates `accounts`                   |
| `..._add_email_norm`        | `momo migrate diff` | adds the column, default `''`        |
| `..._backfill_email_norm`   | you                 | `UPDATE` plus a `DO $$ ... $$` check |
| `..._vacuum_after_backfill` | you                 | `VACUUM`, outside a transaction      |
| `..._unique_email_norm`     | `diff`, then edited | `CREATE UNIQUE INDEX CONCURRENTLY`   |

## Writing a custom migration

A migration is `<14-digit UTC version>_<name>.up.sql` plus
`<version>_<name>.snapshot.json`. The name is lowercase letters, digits and
underscores. The version must sort after the previous one.

1. Write the `.up.sql` file. Leave out the `-- momo:generated` line.
2. Copy the previous snapshot file to the new name and change two fields,
   `version` to the new version and `parent` to the previous version. A custom
   migration must not change the schema, and `momo migrate validate` fails if
   its snapshot differs from its parent's.
3. Run `momo migrate hash`. It lists the files it adds to `momo.sum`.
4. Run `momo migrate validate`.

Do this before the next `momo migrate diff`, which builds on the latest snapshot.

## What Momo does with the file

- **Checksums.** `momo.sum` holds a SHA-256 for every file. Editing a file
  without running `momo migrate hash` makes `validate` and `apply` fail.
  `hash` accepts every edit as the new baseline, so review the files first.
- **Statement splitting.** A statement ends at a semicolon outside strings,
  comments and dollar-quoted bodies. The `DO $$ ... $$` block above holds
  semicolons and stays one statement. `momo migrate plan` shows the count:
  `backfill_email_norm (2 statements)`. A `-- momo:split` directive in the
  leading comment block splits at every semicolon instead.
- **Transactions.** Each file runs in one transaction. A `-- momo:no-transaction`
  directive in the leading comment block runs its statements outside one. Use it
  for statements PostgreSQL refuses in a transaction, such as `VACUUM` and
  `CREATE INDEX CONCURRENTLY`, and put one statement in the file.
- **Directives.** An unknown `-- momo:` directive is an error. The others are
  `destructive` (allows a drop in a generated migration) and `generated`.

## CONCURRENTLY

In v0.1.0 `momo migrate diff` writes a plain `CREATE UNIQUE INDEX`, which blocks
writes while it builds, even if the model asks for `Concurrently()`. For a large
table, edit the generated file as in `..._unique_email_norm.up.sql`: add
`CONCURRENTLY` and the `-- momo:no-transaction` directive, keep the snapshot, then
run `momo migrate hash`. If a concurrent build fails, PostgreSQL leaves an
invalid index behind; `momo migrate drift` reports it.

## Run it

```sh
go test ./examples/custom-sql-migration
MOMO_TEST_POSTGRES_URL=postgres://... go test ./examples/custom-sql-migration
```

Without `MOMO_TEST_POSTGRES_URL` the test skips. With it, the test applies the
migrations up to the new column, inserts rows, applies the rest, and checks the
backfill, the index, `status` and `drift`. A second test edits a copy of a
migration and checks that `validate` rejects it.

To run the program against your own database:

```sh
DATABASE_URL=postgres://... momo migrate apply
DATABASE_URL=postgres://... go run ./examples/custom-sql-migration
```
