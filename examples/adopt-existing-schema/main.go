// SPDX-License-Identifier: Apache-2.0

// Command adopt-existing-schema reads a database that began as plain SQL
// (legacy.sql) and was then adopted with `momo migrate baseline`.
//
//	DATABASE_URL=postgres://... momo migrate baseline --version <init version>
//	DATABASE_URL=postgres://... momo migrate apply
//	DATABASE_URL=postgres://... go run ./examples/adopt-existing-schema
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/examples/adopt-existing-schema/models"
	"github.com/azrtydxb/momo/examples/adopt-existing-schema/models/order"
	"github.com/azrtydxb/momo/examples/adopt-existing-schema/models/user"
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
	return run(ctx, momo.New(pgxdriver.New(pool)))
}

// run prints each user's orders. The rows were inserted by legacy.sql; the
// models read them like any other.
func run(ctx context.Context, q momo.Querier) error {
	users, err := momo.Select[models.User](q).OrderBy(user.ID.Asc()).All(ctx)
	if err != nil {
		return err
	}
	if err := momo.Load(ctx, q, users, user.Orders); err != nil {
		return err
	}
	for _, u := range users {
		for _, o := range u.Orders {
			fmt.Printf("%s order %d: %d cents (%s)\n", u.Email, o.ID, o.TotalCents, o.Status)
		}
	}
	// A query built from the generated columns, on the migrated column.
	n, err := momo.Select[models.Order](q).Where(order.Status.Eq("new")).Count(ctx)
	if err != nil {
		return err
	}
	fmt.Println("orders with status new:", n)
	return nil
}
