// SPDX-License-Identifier: Apache-2.0

// Package companiondomainuse declares composite types and domains that are
// valid, and fields that use them in ways the generator refuses.
package companiondomainuse

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/types"
)

type Address struct {
	Street string
}

type Bad struct {
	momo.Model `momo:"table:bad"`
	ID         int64 `momo:"pk"`
	NoTag      types.Composite[Address]
	Wrong      string   `momo:"type:address"`
	Many       []string `momo:"type:address[]"`
}

func (b *Bad) MomoConfig(c *momo.Config) {
	c.Composite("address").Attr("street", "text")
	// An attribute that is a composite type, and one that is an array of one.
	c.Composite("outer").Attr("home", "address").Attr("homes", "address[]")
	// A domain over a composite type.
	c.Domain("over").As("address")
}
