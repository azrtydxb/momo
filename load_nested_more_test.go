// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
)

// cancelHook cancels the context handed to the relation named name, as the
// relation starts. The level above has fully run by then.
type cancelHook struct {
	*relfixture.Hook
	name   string
	cancel context.CancelFunc
}

func (c *cancelHook) BeforeRelation(ctx context.Context, e *momo.RelationEvent) context.Context {
	if e.Name == c.name {
		c.cancel()
	}
	return ctx
}
