// SPDX-License-Identifier: Apache-2.0

// Package companionhistorybad repeats the History call.
package companionhistorybad

import "github.com/azrtydxb/momo"

type A struct {
	momo.Model `momo:"table:a"`
	ID         int64 `momo:"pk"`
}

func (a *A) MomoConfig(c *momo.Config) {
	c.History()
	c.History()
}
