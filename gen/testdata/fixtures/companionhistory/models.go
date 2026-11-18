// SPDX-License-Identifier: Apache-2.0

// Package companionhistory declares an audit history in MomoConfig.
package companionhistory

import "github.com/azrtydxb/momo"

type Note struct {
	momo.Model `momo:"table:notes"`
	ID         int64 `momo:"pk"`
	Title      string
	Body       *string
}

func (n *Note) MomoConfig(c *momo.Config) {
	c.History()
}

// Tag keeps no history.
type Tag struct {
	momo.Model `momo:"table:tags"`
	ID         int64 `momo:"pk"`
	Name       string
}
