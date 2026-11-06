# quickstart

## What this shows

The compiled source of the snippets in the [root README](../../README.md): models,
code generation, insert, select, update, delete, relations, a vector similarity
query and a transaction. `readme_test.go` fails when a README snippet drifts from
this code.

## Run

```sh
cd examples/quickstart
export DATABASE_URL=postgres://user:pass@localhost:5432/scratch
go run .
```

This example has no migrations: the root README walks through creating them with
`momo migrate diff`. See [examples/README.md](../README.md) for the examples
that run end to end in CI.
