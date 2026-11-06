// SPDX-License-Identifier: Apache-2.0

// Command upgrade-database reads the customers of a database that was
// upgraded from schema v1 to the latest version with `momo migrate apply`.
// Its models are the latest version, so it needs the migrations applied first.
//
//	DATABASE_URL=postgres://... momo migrate apply
//	DATABASE_URL=postgres://... go run ./examples/upgrade-database
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/examples/upgrade-database/models"
	"github.com/azrtydxb/momo/examples/upgrade-database/models/customer"
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

// run prints every customer. Rows written before the plan column existed have
// the column default, "free", because the migration that added it declared one.
func run(ctx context.Context, q momo.Querier) error {
	customers, err := momo.Select[models.Customer](q).OrderBy(customer.ID.Asc()).All(ctx)
	if err != nil {
		return err
	}
	for _, c := range customers {
		fmt.Printf("%d %s (%s) plan=%s\n", c.ID, c.Name, c.Email, c.Plan)
	}
	return nil
}
