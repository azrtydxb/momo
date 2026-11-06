// SPDX-License-Identifier: Apache-2.0

package embedshadow

import "github.com/azrtydxb/momo"

type Label struct {
	Name string `momo:"column:label_name"`
}

type Doc struct {
	momo.Model `momo:"table:docs"`
	ID         int64 `momo:"pk"`
	Name       string
	Label
}
