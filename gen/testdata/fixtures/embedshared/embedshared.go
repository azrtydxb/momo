// SPDX-License-Identifier: Apache-2.0

package embedshared

import "github.com/azrtydxb/momo"

type Bad struct {
	Note string `momo:"bogus_option"`
}

type One struct {
	momo.Model `momo:"table:one"`
	ID         int64 `momo:"pk"`
	Bad
}

type Two struct {
	momo.Model `momo:"table:two"`
	ID         int64 `momo:"pk"`
	Bad
}
