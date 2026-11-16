// SPDX-License-Identifier: Apache-2.0

package checkincall

import "github.com/azrtydxb/momo"

type Note struct {
	momo.Model `momo:"table:notes"`
	ID         int64 `momo:"pk,identity"`
	Kind       string
	Tag        *string
}

func (n *Note) MomoConfig(c *momo.Config) {
	c.CheckIn(&n.Kind,
		"published",
		"draft",
		"it's",
		"archived",
	)
	c.CheckIn(&n.Tag,
		"x",
		"y",
	).Name("notes_tag_in")
}
