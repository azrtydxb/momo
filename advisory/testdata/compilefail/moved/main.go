// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"

	"github.com/azrtydxb/momo"
)

func main() {
	ctx := context.Background()
	var db *momo.DB
	var tx *momo.Tx
	_ = tx.AdvisoryXactLock(ctx, 1)
	_ = tx.AdvisoryXactLockString(ctx, "k")
	_, _ = tx.TryAdvisoryXactLock(ctx, 1)
	_, _ = tx.TryAdvisoryXactLockString(ctx, "k")
	_ = db.AdvisoryXactLock(ctx, 1)
	_ = db.AdvisoryXactLockString(ctx, "k")
	_, _ = db.TryAdvisoryXactLock(ctx, 1)
	_, _ = db.TryAdvisoryXactLockString(ctx, "k")
}
