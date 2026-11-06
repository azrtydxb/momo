// SPDX-License-Identifier: Apache-2.0

// Command custom-sql-migration looks accounts up by email, ignoring case, on
// a database whose migrations (generated and hand-written) are applied.
//
//	DATABASE_URL=postgres://... momo migrate apply
//	DATABASE_URL=postgres://... go run ./examples/custom-sql-migration
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/examples/custom-sql-migration/models"
	"github.com/azrtydxb/momo/examples/custom-sql-migration/models/account"
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

// run adds an account and finds it again by a differently-cased email. The
// unique index on email_norm, built by the last migration, answers the lookup.
func run(ctx context.Context, q momo.Querier) error {
	email := "Grace@Example.com"
	a := models.Account{Email: email, EmailNorm: strings.ToLower(email)}
	if _, err := momo.Insert[models.Account](q).Model(&a).Exec(ctx); err != nil {
		return err
	}
	found, err := momo.Select[models.Account](q).
		Where(account.EmailNorm.Eq(strings.ToLower("GRACE@EXAMPLE.COM"))).
		One(ctx)
	if err != nil {
		return err
	}
	fmt.Println("found:", found.Email)
	return nil
}
