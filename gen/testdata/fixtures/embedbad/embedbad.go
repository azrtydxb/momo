// SPDX-License-Identifier: Apache-2.0

package embedbad

import "github.com/azrtydxb/momo"

type Address struct {
	Street string `momo:"column:street"`
}

type Clash struct {
	momo.Model    `momo:"table:clash"`
	ID            int64 `momo:"pk"`
	BillingStreet string
	Billing       Address `momo:"embedded,prefix:billing_"`
}

type Ptr struct {
	momo.Model `momo:"table:ptr"`
	ID         int64 `momo:"pk"`
	*Address
}

type NotStruct struct {
	momo.Model `momo:"table:notstruct"`
	ID         int64 `momo:"pk"`
	Count      int   `momo:"embedded"`
}

type PrefixOnly struct {
	momo.Model `momo:"table:prefixonly"`
	ID         int64   `momo:"pk"`
	Billing    Address `momo:"prefix:b_"`
}

type Dup struct {
	Code string `momo:"column:code,unique:code_u"`
}

type Twice struct {
	momo.Model `momo:"table:twice"`
	ID         int64 `momo:"pk"`
	A          Dup   `momo:"embedded"`
	B          Dup   `momo:"embedded"`
}
