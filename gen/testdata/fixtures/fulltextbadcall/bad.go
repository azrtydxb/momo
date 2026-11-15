// SPDX-License-Identifier: Apache-2.0

package fulltextbadcall

import "github.com/azrtydxb/momo"

type Counter struct {
	momo.Model `momo:"table:counters"`
	ID         int64 `momo:"pk,identity"`
	Views      int64
}

var other Counter

type DuplicateOption struct {
	momo.Model `momo:"table:duplicate_options"`
	ID         int64 `momo:"pk,identity"`
	Title      string
}

func (d *DuplicateOption) MomoConfig(c *momo.Config) {
	c.FullText("duplicate_options_search", &d.Title).Config("english").Config("simple")
}

type Unresolved struct {
	momo.Model `momo:"table:unresolveds"`
	ID         int64 `momo:"pk,identity"`
	Title      string
}

func (u *Unresolved) MomoConfig(c *momo.Config) {
	c.FullText("unresolveds_search", &other.Views)
}
