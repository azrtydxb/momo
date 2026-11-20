// SPDX-License-Identifier: Apache-2.0

// Package companiondomainbad declares domains and composite types the
// generator refuses to read.
package companiondomainbad

import "github.com/azrtydxb/momo"

type Bad struct {
	momo.Model `momo:"table:bad"`
	ID         int64 `momo:"pk"`
}

func (b *Bad) MomoConfig(c *momo.Config) {
	c.Composite("address").Attr("street", "text")
	// A name that is already a composite type.
	c.Composite("address").Attr("street", "text")
	// A domain without a base type, with a repeated option, with a name that
	// needs quoting and with a check that has no expression.
	c.Domain("no_base")
	c.Domain("twice").As("text").As("text")
	c.Domain("Upper").As("text")
	c.Domain("no_expr").As("text").Check("c", "")
	// A composite type without attributes, and with two of one name.
	c.Composite("empty")
	c.Composite("pair").Attr("a", "text").Attr("a", "text")
}
