// SPDX-License-Identifier: Apache-2.0

package advisory_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/advisory"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// Acquire with Xact serializes writers that share a key. The lock belongs to
// the transaction and is released at commit or rollback, so there is no
// release.
func ExampleAcquire() {
	ctx := context.Background()
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)

	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		_, err := advisory.Acquire(ctx, tx, "catalog:7", advisory.Xact())
		return err
	})
	fmt.Println(err)
	fmt.Println(drv.Calls()[0].SQL)
	// Output:
	// <nil>
	// SELECT pg_advisory_xact_lock(hashtext($1))
}

// Only PostgreSQL has transaction-scoped advisory locks; other dialects fail
// with ErrUnsupported. With Xact the handle must be a *momo.Tx; a *momo.DB is
// ErrValidation.
func ExampleAcquire_try() {
	ctx := context.Background()
	db := momo.New(fakedriver.New(dialect.SQLite))

	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		_, err := advisory.Acquire(ctx, tx, 9001, advisory.Xact(), advisory.Try())
		return err
	})
	fmt.Println(errors.Is(err, momo.ErrUnsupported))
	// Output:
	// true
}
