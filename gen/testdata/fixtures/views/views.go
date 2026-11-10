// SPDX-License-Identifier: Apache-2.0

package views

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/schema"
)

type Account struct {
	momo.Model `momo:"table:accounts"`
	ID         int64 `momo:"pk,identity"`
	Name       string
}

// ActiveAccount maps a view; the generator records it in Meta.View.
type ActiveAccount struct {
	momo.Model `momo:"table:active_accounts"`
	momo.View
	ID   int64 `momo:"pk"`
	Name string
}

func (a *ActiveAccount) MomoConfig(c *momo.Config) {
	c.View(schema.View{Name: "active_accounts", Query: "select id, name from accounts", Depends: []string{"accounts"}})
}
