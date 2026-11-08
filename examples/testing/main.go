// SPDX-License-Identifier: Apache-2.0

// Command testing shows how to test application code against a real
// PostgreSQL database with momotest: a database or a transaction per test,
// the project's embedded migrations applied, seeded fixtures, and tests that
// run in parallel.
//
// The program itself is a few lines of bank-account logic; read
// example_test.go for the point. Run it against a database that has the
// migrations applied:
//
//	DATABASE_URL=postgres://... go run ./examples/testing
package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/examples/testing/models"
	"github.com/azrtydxb/momo/examples/testing/models/account"
)

// migrationFiles embeds the migrations directory, so a binary and its tests
// carry the same schema. A real project does this once, in the package that
// owns the schema, and hands Migrations to momotest.WithMigrations.
//
//go:embed migrations
var migrationFiles embed.FS

// Migrations returns the embedded migrations directory.
func Migrations() fs.FS {
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		panic(err) // the directory is embedded above; this cannot fail
	}
	return sub
}

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

// run opens two accounts, moves money between them and prints the balances.
func run(ctx context.Context, db *momo.DB, out io.Writer) error {
	accounts := []models.Account{{Owner: "ada", Balance: 100}, {Owner: "grace", Balance: 50}}
	if _, err := momo.Insert[models.Account](db).Models(accounts).Exec(ctx); err != nil {
		return err
	}
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		return Transfer(ctx, tx, "ada", "grace", 30)
	})
	if err != nil {
		return err
	}
	for _, owner := range []string{"ada", "grace"} {
		b, err := Balance(ctx, db, owner)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s=%d\n", owner, b)
	}
	return nil
}

// Transfer moves amount between two accounts inside the caller's transaction.
// The database refuses a balance below zero, and the error then matches
// momo.ErrCheckViolation.
func Transfer(ctx context.Context, tx *momo.Tx, from, to string, amount int64) error {
	if err := addBalance(ctx, tx, to, amount); err != nil {
		return err
	}
	return addBalance(ctx, tx, from, -amount)
}

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

// Balance returns the balance of owner's account.
func Balance(ctx context.Context, q momo.Querier, owner string) (int64, error) {
	a, err := momo.Select[models.Account](q).Where(account.Owner.Eq(owner)).One(ctx)
	if err != nil {
		return 0, err
	}
	return a.Balance, nil
}
