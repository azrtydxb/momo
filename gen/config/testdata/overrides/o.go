// SPDX-License-Identifier: Apache-2.0

package overrides

import "github.com/azrtydxb/momo"

type Item struct {
	momo.Model `momo:"table:items"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"column:custom_name"`
	ItemCode   string
}
