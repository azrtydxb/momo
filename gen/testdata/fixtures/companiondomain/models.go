// SPDX-License-Identifier: Apache-2.0

// Package companiondomain declares a domain and a composite type, and fields
// that use them.
package companiondomain

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/types"
)

// Address is the Go struct of the composite type address.
type Address struct {
	Street string
	Zip    *string
}

type Customer struct {
	momo.Model `momo:"table:customers"`
	ID         int64                     `momo:"pk"`
	Email      string                    `momo:"type:email"`
	Backup     *string                   `momo:"type:email"`
	Home       types.Composite[Address]  `momo:"type:address"`
	Work       *types.Composite[Address] `momo:"type:address"`
}

func (c *Customer) MomoConfig(cfg *momo.Config) {
	cfg.Domain("email").As("text").NotNull().Default("''").Check("email_shape", "VALUE <> ''").Check("email_at", "VALUE ~ '@'")
	cfg.Composite("address").Attr("street", "text").Attr("zip", "text")
}
