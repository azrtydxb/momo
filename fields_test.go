// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"

	"github.com/azrtydxb/momo"
)

type countHook struct{ n int }

func (c *countHook) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context {
	c.n++
	return ctx
}
func (c *countHook) AfterQuery(context.Context, *momo.QueryEvent) {}
