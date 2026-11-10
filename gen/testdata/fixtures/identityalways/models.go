// SPDX-License-Identifier: Apache-2.0

// Package identityalways has GENERATED ALWAYS and BY DEFAULT identity columns.
package identityalways

import "github.com/azrtydxb/momo"

type Ledger struct {
	momo.Model `momo:"table:ledger"`
	Seq        int64 `momo:"pk,identity:always"`
	Body       string
}

type Counter struct {
	momo.Model `momo:"table:counter"`
	ID         int64 `momo:"pk,identity"`
	Hits       int64 `momo:"identity:by_default"`
}
