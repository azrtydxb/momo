// SPDX-License-Identifier: Apache-2.0

package tableoptionsbad

import "github.com/azrtydxb/momo"

// Misplaced names the table options on fields, where they mean nothing.
type Misplaced struct {
	momo.Model `momo:"table:misplaced"`
	ID         int64  `momo:"pk,autoincrement"`
	Name       string `momo:"strict,nullable"`
}
