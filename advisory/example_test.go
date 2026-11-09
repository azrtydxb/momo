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

// A call with the DB instead of a transaction is refused, because the server
// would release the lock at the end of that one statement.
func ExampleTryXactLock() {
	db := momo.New(fakedriver.New(dialect.Postgres))

	_, err := advisory.TryXactLock(context.Background(), db, 9001)
	fmt.Println(errors.Is(err, momo.ErrValidation))
	// Output:
	// true
}
