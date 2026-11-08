# testing

## What this shows

Testing application code against a real PostgreSQL database with
[`momotest`](../../docs/testing.md), on a small bank-account service.

- `momotest.NewTx(t, momotest.WithMigrations(Migrations()))` gives a test a
  transaction on a schema that already has the embedded migrations. Everything
  the test writes is rolled back when it ends, so parallel subtests start from
  the same two accounts and never see each other's transfers.
- A refused overdraft is tested against the real `CHECK` constraint, inside a
  savepoint, so the test can look at the balances afterwards.
- `momotest.New` gives a schema of its own, dropped when the test ends, for code
  that opens and commits its own transactions.
- `momotest.Seed` with `momotest.Insert` writes the fixtures in order and reads
  the generated ids back into the rows.
- The migrations are embedded with `go:embed`, so the binary and its tests carry
  the same schema.

The tests are skipped when `MOMO_TEST_POSTGRES_URL` is unset, and fail instead
when `MOMO_REQUIRE_INTEGRATION=1`.

## Run

```sh
cd examples/testing
export DATABASE_URL=postgres://user:pass@localhost:5432/scratch
momo migrate apply
go run .
```

To run the tests, point `MOMO_TEST_POSTGRES_URL` at a server and run
`go test -tags integration -race ./examples/testing`.
