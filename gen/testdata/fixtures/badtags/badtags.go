// SPDX-License-Identifier: Apache-2.0

package badtags

import "github.com/azrtydxb/momo"

type NotAModel struct {
	ID int64 `momo:"pk"`
}

type Priv struct {
	momo.Model
	ID     int64  `momo:"pk"`
	hidden string `momo:"column:h"`
	quiet  string
}

type Ptr struct {
	*momo.Model
	ID int64
}

type Generic[T any] struct {
	momo.Model
	ID int64
	V  T
}

type Twice struct {
	momo.Model
	momo.Model
	ID int64
}

type Defaulted struct {
	momo.Model
	ID  int64
	Own string `momo:"bogus_option"`
}

type AsAlias = Priv

type Marker momo.Model

type NamedDefault struct {
	momo.Model
	ID   int64
	Rels []Defaulted `momo:"has_many"`
}
