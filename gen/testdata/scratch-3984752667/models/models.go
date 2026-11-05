// SPDX-License-Identifier: Apache-2.0

package models

import "github.com/azrtydxb/momo"


type C struct {
	momo.Model
	ID int64 `momo:"pk"`
	OwnerType string
	OwnerID int64
}
type A struct {
	momo.Model
	ID int64 `momo:"pk"`
	Cs []C `momo:"has_many,fk:OwnerID,polytype:owner_type"`
}