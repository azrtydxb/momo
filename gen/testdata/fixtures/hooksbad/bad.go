// SPDX-License-Identifier: Apache-2.0

package hooksbad

import (
	"context"

	"github.com/azrtydxb/momo"
)

type Doc struct {
	momo.Model `momo:"table:docs"`
	ID         int64 `momo:"pk,identity"`
}

func (d *Doc) BeforeInsert() error { return nil } // BAD-1

var _ = context.Background
