// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"slices"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// historyStmt is the declaration a recognised audit history is written as.
const historyStmt = "c.History()"

// recogniseHistory finds the audit histories that c.History() would declare:
// a table T with the table <T>_history, the function <T>_history_fn and the
// trigger <T>_history_trg on T, all exactly what schema.HistoryFor builds from
// T as inspected. They are declared by one c.History() on the model of T, so
// the history table gets no model of its own and the function and trigger are
// not written; the snapshot keeps all three, as the models build them. Anything
// that differs, a hand-written history for instance, is not recognised and is
// modeled as the ordinary table, function and trigger it is, so a regenerate
// never declares one twice. PostgreSQL only, like c.History().
func (g *generator) recogniseHistory() {
	if g.o.Dialect != dialect.Postgres {
		return
	}
	g.histTables, g.histBase = map[string]bool{}, map[string]bool{}
	for i := range g.clean.Tables {
		base := &g.clean.Tables[i]
		want, err := schema.HistoryFor(base)
		if err != nil {
			continue
		}
		hi := slices.IndexFunc(g.clean.Tables, func(t schema.Table) bool {
			return t.Schema == base.Schema && t.Name == want.Table.Name
		})
		fi := slices.IndexFunc(g.clean.Functions, func(f schema.Function) bool {
			return f.Schema == base.Schema && f.Name == want.Function.Name && f.Args == ""
		})
		ti := slices.IndexFunc(g.clean.Triggers, func(x schema.Trigger) bool {
			return x.Schema == base.Schema && x.Table == base.Name && x.Name == want.Trigger.Name
		})
		if hi < 0 || fi < 0 || ti < 0 || g.histTables[tableKey(base.Schema, base.Name)] {
			continue
		}
		live := &schema.Schema{Dialect: dialect.Postgres, Tables: []schema.Table{g.clean.Tables[hi]}, Functions: []schema.Function{g.clean.Functions[fi]}, Triggers: []schema.Trigger{g.clean.Triggers[ti]}}
		exp := &schema.Schema{Dialect: dialect.Postgres, Tables: []schema.Table{want.Table}, Functions: []schema.Function{want.Function}, Triggers: []schema.Trigger{want.Trigger}}
		if !schema.Equal(live, exp) {
			g.add(SevNote, want.Table.Name, "looks like the audit history of %s but differs from what c.History() declares: it is modeled as an ordinary table, function and trigger", base.Name)
			continue
		}
		g.histTables[tableKey(base.Schema, want.Table.Name)] = true
		g.histBase[tableKey(base.Schema, base.Name)] = true
		g.histFns = append(g.histFns, g.clean.Functions[fi])
		g.histTrgs = append(g.histTrgs, g.clean.Triggers[ti])
		g.clean.Functions = slices.Delete(g.clean.Functions, fi, fi+1)
		g.clean.Triggers = slices.Delete(g.clean.Triggers, ti, ti+1)
		g.add(SevNote, base.Name, "its audit history (%s, %s, %s) is declared by c.History()", want.Table.Name, want.Function.Name, want.Trigger.Name)
	}
}

// restoreHistory puts the history functions and triggers that c.History()
// declares back into the snapshot, which holds what the models build.
func (g *generator) restoreHistory() {
	if len(g.histFns) == 0 {
		return
	}
	g.clean.Functions = append(g.clean.Functions, g.histFns...)
	g.clean.Triggers = append(g.clean.Triggers, g.histTrgs...)
	g.clean.Sort()
}
