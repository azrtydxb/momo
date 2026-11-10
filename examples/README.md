# Momo examples

Small programs that each show one task the way Momo intends it to be done.
Read one, copy the parts you need. Every example is compiled, and every example
that talks to a database is run end to end in CI, so the code here is code that
works.

| Example                                         | Shows                                                                                                                                              |
| ----------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| [quickstart](quickstart/)                       | The whole loop of the root README: models, generate, insert, select, relations, vector search, a Tx.                                               |
| [crud-zero-null](crud-zero-null/)               | Insert, select, update, delete; writing a zero value versus NULL versus the default; soft delete.                                                  |
| [transactions](transactions/)                   | `db.Tx`, rollback on error, savepoints, read-only and isolation options, retry on serialization.                                                   |
| [testing](testing/)                             | `momotest`: a database or a rolled-back transaction per test with the migrations applied, seeded fixtures, parallel tests.                         |
| [relations](relations/)                         | has-many, belongs-to, many-to-many; Load, Preload, Joined, Has/HasNo, Link/Replace, CreateGraph.                                                   |
| [projection-pagination](projection-pagination/) | Reading a subset of columns into a struct, keyset pagination with a tie-break, counting.                                                           |
| [vector-search](vector-search/)                 | A vector column, an HNSW index in a migration, nearest-neighbour queries, filters, search settings.                                                |
| [upgrade-database](upgrade-database/)           | change a model after production: add a column with a default, add an index, rename a column, apply with data kept, generate --check and drift.     |
| [custom-sql-migration](custom-sql-migration/)   | hand-written SQL migrations beside generated ones: backfill, DO $$ splitting, -- momo:no-transaction, CONCURRENTLY workaround, momo.sum checksums. |
| [adopt-existing-schema](adopt-existing-schema/) | put a plain-SQL database under Momo with migrate baseline (no model generation from a database in v0.4.0). <!-- x-release-please-version -->       |

## Layout of an example

Each `examples/<name>/` is a self-contained program inside the Momo module:

```text
examples/<name>/
  README.md        what it shows and how to run it
  momo.yaml        generator and migration settings
  main.go          package main: main, start (opens the database), run (the steps)
  models/          the schema; models.go is yours, momo_gen.go and <model>/ are generated
  migrations/      written by `momo migrate diff`, applied by `momo migrate apply`
  example_test.go  runs the example end to end (see below)
```

The generated code and the migrations are committed, like in a real project,
and are produced by the real tools: `momo generate` and `momo migrate diff init`
run in the example directory. Do not edit them by hand.

`main.go` keeps `main` and `start` trivial and puts the steps in
`run(ctx, db, out)`, which writes what it finds to `out`. A step is a function
named for what it shows, with a comment that says why it is done this way. The
printed lines are the contract: the test compares them.

## How an example is tested, and how CI runs it

`example_test.go` is built with `//go:build integration` and calls the harness
in [`internal/exampletest`](internal/exampletest/exampletest.go):

```go
func TestExample(t *testing.T) {
	exampletest.CheckGenerated(t)               // momo generate --check: generated code is current
	db := exampletest.Open(t)                         // scratch database + the example's migrations applied
	var out bytes.Buffer
	if err := run(context.Background(), db, &out); err != nil {
		t.Fatal(err)
	}
	// compare out.String() with the expected text
}
```

`Open` creates a scratch database (`testdb.ScratchDatabase`), so an example may
create the `vector` extension and any tables without touching the shared test
database, and drops it when the test ends. It skips the test when
`MOMO_TEST_POSTGRES_URL` is unset and fails it when `MOMO_REQUIRE_INTEGRATION=1`.

CI needs no per-example entry. The `integration` job runs
`go test -tags integration ./...`, which includes every `examples/*/` package,
and the `unit` and `lint` jobs compile and vet each one. An example whose test
needs no database (like `quickstart`) is an ordinary `_test.go` that the `unit`
job runs.

`TestExamplesAreWired` in `internal/repocheck` enforces the convention: every
example has a `main.go`, a `README.md` and a test, a test that uses the harness
carries the `integration` tag, this index and `docs/README.md` link the example,
and the integration job still runs every package.

## Adding an example

1. Create `examples/<name>/` with `momo.yaml` and `models/models.go`.
2. Run `momo generate` and `momo migrate diff init` in it. The repository
   formatter rewrites the `*.snapshot.json` file; if it does, run
   `momo migrate hash` afterwards so `momo.sum` matches the formatted file.
3. Write `main.go` and `example_test.go` as above. Run the test once, read the
   output, and paste it in as the expected text only when it is right.
4. Write `README.md`: what it shows, and the run command.
5. Add a row to the table above and a line to the Examples list in
   [`docs/README.md`](../docs/README.md).

To run an example by hand against your own database:

```sh
cd examples/<name>
export DATABASE_URL=postgres://user:pass@localhost:5432/scratch
momo migrate apply
go run .
```
