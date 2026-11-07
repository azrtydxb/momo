// SPDX-License-Identifier: Apache-2.0

package pgxdriver_test

import (
	"context"

	"github.com/azrtydxb/momo"
)

type countHook struct{ n int }

func (c *countHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	c.n++
	return ctx
}

func (c *countHook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {}
