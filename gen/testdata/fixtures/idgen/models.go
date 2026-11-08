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

// Account has its generated id after other columns, so a column index that is
// off by one is caught.
type Account struct {
	momo.Model
	ID       int64 `momo:"pk"`
	Email    string
	PublicID string `momo:"idgen:uuid"`
}

// Token names a generator registered at runtime.
type Token struct {
	momo.Model
	ID    string `momo:"pk,idgen:short_token"`
	Label string
}
