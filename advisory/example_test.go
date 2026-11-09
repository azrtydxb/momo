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

// XactLock serializes writers that share a key. The lock belongs to the
// transaction and is released at commit or rollback, so there is no unlock.
func ExampleXactLock() {
	ctx := context.Background()
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)

	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		return advisory.XactLockString(ctx, tx, "catalog:7")
	})
	fmt.Println(err)
	fmt.Println(drv.Calls()[0].SQL)
	// Output:
	// <nil>
	// SELECT pg_advisory_xact_lock(hashtext($1))
}

// Only PostgreSQL has transaction-scoped advisory locks; other dialects fail
// with ErrUnsupported. The functions take a *momo.Tx, so a *momo.DB does not
// compile.
func ExampleTryXactLock() {
	ctx := context.Background()
	db := momo.New(fakedriver.New(dialect.SQLite))

	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		_, err := advisory.TryXactLock(ctx, tx, 9001)
		return err
	})
	fmt.Println(errors.Is(err, momo.ErrUnsupported))
	// Output:
	// true
}
