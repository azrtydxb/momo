// SPDX-License-Identifier: Apache-2.0

package projhook

import (
	"context"

	"github.com/azrtydxb/momo"
)

type Card struct {
	momo.Projection
	ID    int64
	Title string
}

func (c *Card) AfterLoad(ctx context.Context) error { return nil } // BAD-1
