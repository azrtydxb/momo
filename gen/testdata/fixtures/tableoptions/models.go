// SPDX-License-Identifier: Apache-2.0

package tableoptions

import "github.com/azrtydxb/momo"

// Counter is an AUTOINCREMENT STRICT table.
type Counter struct {
	momo.Model `momo:"table:counters,autoincrement,strict"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"nullable"`
}

// Setting is a WITHOUT ROWID table.
type Setting struct {
	momo.Model `momo:"table:settings,without_rowid"`
	K          string `momo:"pk"`
	V          string `momo:"nullable"`
}
