// SPDX-License-Identifier: Apache-2.0

package mixins

import (
	"time"

	"github.com/azrtydxb/momo"
)

type Timestamps struct {
	CreatedAt time.Time `momo:"column:created_at,read_only"`
	UpdatedAt time.Time `momo:"column:updated_at,read_only,index"`
}

type Address struct {
	Street string `momo:"column:street"`
	City   string `momo:"column:city"`
}

type Article struct {
	momo.Model `momo:"table:articles"`
	ID         int64 `momo:"pk,identity"`
	Timestamps
	Billing  Address `momo:"embedded,prefix:billing_"`
	Shipping Address `momo:"embedded,prefix:shipping_"`
}
