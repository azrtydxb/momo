// SPDX-License-Identifier: Apache-2.0

package projection

import (
	"time"

	"github.com/azrtydxb/momo"
)

type Widget struct {
	momo.Model
	ID        int64      `momo:"pk,identity"`
	Title     string     `momo:"notnull"`
	Score     int64      `momo:"notnull"`
	DeletedAt *time.Time `momo:"soft_delete:timestamp"`
}

type UnixWidget struct {
	momo.Model
	ID   int64  `momo:"pk,identity"`
	Gone *int64 `momo:"column:gone_at,soft_delete:unix"`
}

type BoolWidget struct {
	momo.Model
	ID      int64 `momo:"pk,identity"`
	Removed bool  `momo:"soft_delete:bool"`
}

// Card has columns set by tag and columns named by the naming rule.
type Card struct {
	momo.Projection
	ID        int64
	Title     string
	Score     int64      `momo:"column:points"`
	CreatedAt *time.Time `momo:"column:made_at"`
	hidden    int
	Extra     string `momo:"-"`
}

// Tally is an aggregate.
type Tally struct {
	momo.Projection
	Score    int64
	Articles int64
	HTTPCode int32
}
