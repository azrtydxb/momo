// SPDX-License-Identifier: Apache-2.0

package idgenbad

import "github.com/azrtydxb/momo"

// Number puts a generator on an integer field.
type Number struct {
	momo.Model
	ID int64 `momo:"pk,idgen:cuid"`
}

// Twice declares the generator in the tag and in MomoConfig.
type Twice struct {
	momo.Model
	ID string `momo:"pk,idgen:cuid"`
}

func (t *Twice) MomoConfig(c *momo.Config) {
	c.IDGen(&t.ID, "uuid")
}

// Shouty uses a name that is not lowercase.
type Shouty struct {
	momo.Model
	ID string `momo:"pk,idgen:CUID"`
}
