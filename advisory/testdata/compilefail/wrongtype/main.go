// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/advisory"
)

func main() {
	var tx *momo.Tx
	// The int64 form takes no string; the String form takes no int.
	_ = advisory.XactLock(context.Background(), tx, "catalog:7")
	_ = advisory.XactLockString(context.Background(), tx, 7)
}
