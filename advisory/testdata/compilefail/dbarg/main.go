// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/advisory"
)

func main() {
	ctx := context.Background()
	var db *momo.DB
	// A lock outside a transaction is meaningless, so a DB is refused at
	// compile time.
	_ = advisory.XactLock(ctx, db, 1)
	_ = advisory.XactLockString(ctx, db, "k")
	_, _ = advisory.TryXactLock(ctx, db, 1)
	_, _ = advisory.TryXactLockString(ctx, db, "k")
}
