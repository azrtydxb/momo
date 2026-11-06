// SPDX-License-Identifier: Apache-2.0

// Command transactions shows how to group writes so they all happen or none
// do: db.Tx, rollback on error, savepoints for a step that may fail,
// read-only and isolation options, and a retry loop for serialization
// failures.
//
// Run it against a database that has the migrations applied:
//
//	DATABASE_URL=postgres://... go run ./examples/transactions
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/examples/transactions/models"
	"github.com/azrtydxb/momo/examples/transactions/models/account"
)

func main() {
	if err := start(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func start(ctx context.Context) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is not set")
	}
	pool, err := pgxdriver.NewPool(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	return run(ctx, momo.New(pgxdriver.New(pool)), os.Stdout)
}

// run walks through every step and prints what it finds. The test compares
// the output, so the printed lines are the contract of this example.
func run(ctx context.Context, db *momo.DB, out io.Writer) error {
	accounts := []models.Account{{Owner: "ada", Balance: 100}, {Owner: "grace", Balance: 50}}
	if _, err := momo.Insert[models.Account](db).Models(accounts).Exec(ctx); err != nil {
		return err
	}
	steps := []func(context.Context, *momo.DB, io.Writer) error{
		commitTransfer,
		rollBackTransfer,
		savepointStep,
		readOnly,
		retryOnSerializationFailure,
	}
	for _, step := range steps {
		if err := step(ctx, db, out); err != nil {
			return err
		}
		if err := printBalances(ctx, db, out); err != nil {
			return err
		}
	}
	return nil
}

// transfer moves amount between two accounts. It takes a *momo.Tx, not a
// *momo.DB, so a caller cannot run half of it outside a transaction. Every
// query builder takes a momo.Querier, which both *DB and *Tx satisfy, so the
// same code runs inside or outside a transaction.
func transfer(ctx context.Context, tx *momo.Tx, from, to string, amount int64) error {
	// The credit runs first on purpose: when the debit then fails, the
	// rollback has something to undo.
	if err := addBalance(ctx, tx, to, amount); err != nil {
		return err
	}
	return addBalance(ctx, tx, from, -amount)
}

// addBalance changes a balance with one atomic UPDATE (balance = balance + n)
// instead of read, add in Go, write back, which would lose a concurrent
// change.
func addBalance(ctx context.Context, q momo.Querier, owner string, delta int64) error {
	res, err := momo.Update[models.Account](q).
		Set(account.Balance.SetExpr(momo.TypedSQL[int64]("balance + ?", delta))).
		Where(account.Owner.Eq(owner)).
		Exec(ctx)
	if err != nil {
		return err
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("no account %q: %w", owner, momo.ErrNotFound)
	}
	return nil
}

// commitTransfer: db.Tx commits when the function returns nil.
func commitTransfer(ctx context.Context, db *momo.DB, out io.Writer) error {
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		return transfer(ctx, tx, "ada", "grace", 30)
	})
	fmt.Fprintf(out, "transfer 30 ada->grace: err=%v\n", err)
	return nil
}

// rollBackTransfer: db.Tx rolls back when the function returns an error. The
// credit to grace succeeded inside the transaction, then the debit broke the
// balance >= 0 check, so the credit is undone too. Momo reports the check
// failure as momo.ErrCheckViolation.
func rollBackTransfer(ctx context.Context, db *momo.DB, out io.Writer) error {
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		return transfer(ctx, tx, "ada", "grace", 500)
	})
	fmt.Fprintf(out, "transfer 500 ada->grace: check violation=%t\n", errors.Is(err, momo.ErrCheckViolation))
	return nil
}

// savepointStep: tx.Tx runs a function inside a savepoint. If it fails, only
// its own work is undone and the outer transaction carries on. Without the
// savepoint, PostgreSQL would abort the whole transaction at the first error.
func savepointStep(ctx context.Context, db *momo.DB, out io.Writer) error {
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if err := addBalance(ctx, tx, "ada", 1); err != nil { // the bonus we want to keep
			return err
		}
		optional := tx.Tx(ctx, func(sp *momo.Tx) error {
			// "grace" exists, so this violates the unique owner constraint.
			_, err := momo.Insert[models.Account](sp).Model(&models.Account{Owner: "grace"}).Exec(ctx)
			return err
		})
		fmt.Fprintf(out, "savepoint: unique violation=%t\n", errors.Is(optional, momo.ErrUniqueViolation))
		return nil // the outer transaction still commits the bonus
	})
	return err
}

// readOnly: TxOptions can mark a transaction read-only. The server refuses
// every write in it, which turns "this code path must not write" into a
// guarantee.
func readOnly(ctx context.Context, db *momo.DB, out io.Writer) error {
	err := db.Tx(ctx, momo.TxOptions{ReadOnly: true}, func(tx *momo.Tx) error {
		return addBalance(ctx, tx, "ada", 1000)
	})
	fmt.Fprintf(out, "write in read-only transaction refused=%t\n", err != nil)
	return nil
}

// retryOnSerializationFailure: at the serializable isolation level the
// database may abort a transaction that raced with another one and report
// momo.ErrSerialization. Momo never retries by itself, because it cannot know
// the function is safe to run twice. You can retry, as long as the function
// has no side effects outside the database: wrap the whole db.Tx call in a
// loop, never just the failing statement.
func retryOnSerializationFailure(ctx context.Context, db *momo.DB, out io.Writer) error {
	attempts := 0
	err := retrySerializable(ctx, db, func(tx *momo.Tx) error {
		attempts++
		if _, err := momo.Select[models.Account](tx).Where(account.Owner.Eq("ada")).One(ctx); err != nil {
			return err // this read fixes the transaction's snapshot
		}
		if attempts == 1 {
			// A competing transaction commits a change to the same row after
			// our read. On the first attempt this makes our write fail.
			if err := db.Tx(ctx, momo.TxOptions{}, func(other *momo.Tx) error {
				return addBalance(ctx, other, "ada", 10)
			}); err != nil {
				return err
			}
		}
		return addBalance(ctx, tx, "ada", 5)
	})
	fmt.Fprintf(out, "serializable retry: attempts=%d err=%v\n", attempts, err)
	return nil
}

// retrySerializable runs fn in a serializable transaction and runs the whole
// transaction again, up to maxAttempts times, while it fails with
// momo.ErrSerialization. A real service adds a short randomized pause
// between attempts.
func retrySerializable(ctx context.Context, db *momo.DB, fn func(tx *momo.Tx) error) error {
	const maxAttempts = 3
	var err error
	for range maxAttempts {
		err = db.Tx(ctx, momo.TxOptions{Isolation: sql.LevelSerializable}, fn)
		if !errors.Is(err, momo.ErrSerialization) {
			return err
		}
	}
	return err
}

func printBalances(ctx context.Context, q momo.Querier, out io.Writer) error {
	accounts, err := momo.Select[models.Account](q).OrderBy(account.Owner.Asc()).All(ctx)
	if err != nil {
		return err
	}
	fmt.Fprint(out, "  balances:")
	for _, a := range accounts {
		fmt.Fprintf(out, " %s=%d", a.Owner, a.Balance)
	}
	fmt.Fprintln(out)
	return nil
}
