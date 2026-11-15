// SPDX-License-Identifier: Apache-2.0

package fulltexttag

import "github.com/azrtydxb/momo"

type Article struct {
	momo.Model `momo:"table:articles"`
	ID         int64  `momo:"pk,identity"`
	Title      string `momo:"search"`
	Body       string `momo:"search"`
}
