// SPDX-License-Identifier: Apache-2.0

package conflict

import "github.com/azrtydxb/momo"

type Clash struct {
	momo.Model `momo:"table:clashes"`
	ID         int64   `momo:"pk,identity"`
	Both       string  `momo:"read_only,immutable"`
	Ptr        *string `momo:"create_only"`
}
