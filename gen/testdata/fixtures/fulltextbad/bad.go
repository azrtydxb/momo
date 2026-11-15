// SPDX-License-Identifier: Apache-2.0

package fulltextbad

import "github.com/azrtydxb/momo"

type Counter struct {
	momo.Model `momo:"table:counters"`
	ID         int64 `momo:"pk,identity"`
	Views      int64 `momo:"search"`
}

type Both struct {
	momo.Model `momo:"table:boths"`
	ID         int64  `momo:"pk,identity"`
	Title      string `momo:"search"`
	Body       string
}

func (b *Both) MomoConfig(c *momo.Config) {
	c.FullText("boths_search", &b.Title, &b.Body)
}
