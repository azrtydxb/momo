// SPDX-License-Identifier: Apache-2.0

package idgen

import "github.com/azrtydxb/momo"

// Contact declares its id generator with the idgen tag.
type Contact struct {
	momo.Model
	ID   string `momo:"pk,idgen:cuid"`
	Name string
}

// Note declares its id generator in MomoConfig.
type Note struct {
	momo.Model
	ID   string
	Body string
}

func (n *Note) MomoConfig(c *momo.Config) {
	c.IDGen(&n.ID, "ulid")
}

// Token names a generator registered at runtime.
type Token struct {
	momo.Model
	ID    string `momo:"pk,idgen:short_token"`
	Label string
}
