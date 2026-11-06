# transactions

## What this shows

Grouping writes so they all happen or none do, on a table of accounts whose
balance may not go below zero.

- `db.Tx(ctx, opts, fn)` commits when `fn` returns nil and rolls back on an
  error or a panic. `fn` runs exactly once; Momo never retries it.
- Query builders take a `momo.Querier`, which both `*DB` and `*Tx` satisfy, so
  the same function runs inside or outside a transaction.
- A failed step rolls back the steps before it: the transfer credits one account,
  the debit breaks the `CHECK`, and the credit is undone. The error matches
  `momo.ErrCheckViolation`.
- `tx.Tx(ctx, fn)` runs `fn` in a savepoint. When it fails, only its own work is
  undone and the outer transaction carries on. PostgreSQL would otherwise abort
  the whole transaction at the first error.
- `TxOptions{ReadOnly: true}` makes the server refuse every write.
  `TxOptions{Isolation: sql.LevelSerializable}` sets the isolation level.
- At serializable isolation the database can abort a transaction that raced with
  another one. The error matches `momo.ErrSerialization`. Retry by running the
  whole `db.Tx` again, as `retrySerializable` does, and only when the function
  has no side effects outside the database.
- `balance = balance + n` through `SetExpr` is one atomic statement, so it does
  not lose a concurrent change the way read, add in Go, write back does.

## Run

```sh
cd examples/transactions
export DATABASE_URL=postgres://user:pass@localhost:5432/scratch
momo migrate apply
go run .
```

The test (`example_test.go`) runs the same steps in a scratch database and
checks the output.
