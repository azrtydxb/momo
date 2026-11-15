// SPDX-License-Identifier: Apache-2.0

package fulltextcall

import "github.com/azrtydxb/momo"

type Article struct {
	momo.Model `momo:"table:articles"`
	ID         int64 `momo:"pk,identity"`
	Title      string
	Body       string
}

func (a *Article) MomoConfig(c *momo.Config) {
	c.FullText("articles_search", &a.Title, &a.Body)
}
