// SPDX-License-Identifier: Apache-2.0

// Package companionuniqbad declares Unique twice on one index.
package companionuniqbad

import "github.com/azrtydxb/momo"

type Twice struct {
	momo.Model `momo:"table:twice"`
	ID         int64 `momo:"pk"`
	Email      string
}

func (t *Twice) MomoConfig(c *momo.Config) {
	c.IndexExpr("twice_email_key", "lower(email)").Unique().Unique()
}
