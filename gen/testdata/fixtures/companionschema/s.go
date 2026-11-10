// SPDX-License-Identifier: Apache-2.0

package companionschema

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/schema"
)

type Doc struct {
	momo.Model `momo:"table:docs"`
	ID         int64  `momo:"pk"`
	TenantID   string `momo:"notnull"`
}

func (d *Doc) MomoConfig(c *momo.Config) {
	c.RLS(schema.RLS{Enabled: true, Forced: true})
	c.Partition(schema.Partition{Strategy: "range", Key: "RANGE (id)"})
	c.Policy(schema.Policy{Table: "docs", Name: "docs_tenant", Command: "ALL", Permissive: true, Using: "tenant_id = current_setting('app.tenant', true)"})
	c.Function(schema.Function{Name: "set_tenant", Returns: "trigger", Language: "plpgsql", Volatility: "VOLATILE", Body: "BEGIN RETURN NEW; END"})
	c.Trigger(schema.Trigger{Table: "docs", Name: "docs_tenant_trg", Timing: "BEFORE", Events: []string{"INSERT"}, ForEachRow: true, Function: "set_tenant"})
}

// DocView maps a materialized view over docs.
type DocView struct {
	momo.Model `momo:"table:doc_view"`
	momo.View
	ID int64 `momo:"pk"`
}

func (d *DocView) MomoConfig(c *momo.Config) {
	c.View(schema.View{Name: "doc_view", Materialized: true, Query: "SELECT id FROM docs", Depends: []string{"docs"}})
}
