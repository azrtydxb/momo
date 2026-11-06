// SPDX-License-Identifier: Apache-2.0

package companionstruct

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/schema"
)

type Account struct {
	momo.Model `momo:"table:accounts"`
	ID         int64
	Name       string
}

func (a *Account) MomoConfig(c *momo.Config) {
	c.View(schema.View{Name: "active_accounts", Materialized: true, Query: "select * from accounts", Depends: []string{"accounts"}})
	c.Function(schema.Function{Name: "touch", Language: "plpgsql", Returns: "trigger", Body: "begin return new; end"})
	c.Trigger(schema.Trigger{Table: "accounts", Name: "accounts_touch", Timing: "BEFORE", Events: []string{"UPDATE"}, ForEachRow: true, Function: "touch"})
	c.Policy(schema.Policy{Table: "accounts", Name: "own", Command: "ALL", Permissive: true, Using: "true"})
	c.RLS(schema.RLS{Enabled: true})
	c.Partition(schema.Partition{Strategy: "range", Key: "RANGE (id)"})
}
