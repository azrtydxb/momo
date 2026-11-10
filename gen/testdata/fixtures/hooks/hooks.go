// SPDX-License-Identifier: Apache-2.0

package hooks

import (
	"context"

	"github.com/azrtydxb/momo"
)

type Doc struct {
	momo.Model `momo:"table:docs"`
	ID         int64 `momo:"pk,identity"`
	Title      string
}

func (d *Doc) BeforeInsert(ctx context.Context) error { return nil }

func (d *Doc) AfterLoad(ctx context.Context) error { return nil }

type Plain struct {
	momo.Model `momo:"table:plains"`
	ID         int64 `momo:"pk,identity"`
}
