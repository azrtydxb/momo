// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"sort"
	"strings"

	"github.com/azrtydxb/momo/schema"
)

// inverses adds, to the model a belongs_to points at, the has_one (the foreign
// key column is unique) or has_many (it is not) that mirrors it. It runs after
// every foreign key is decided, so a foreign key left out of the models gets no
// inverse and neither does one from a table with no model.
//
// The default name is the plural of the child model (the child table in Go
// case) for has_many and the child model name for has_one. When one child has
// several foreign keys to the same parent, or the child is the parent, the name
// gains By and the foreign key field without its ID suffix (MessagesBySender),
// so the names are distinct and stable. introspect.relations renames or skips
// one by its "Model.Field" key.
func (g *generator) inverses() {
	type pair struct{ child, parent *model }
	count := map[pair]int{}
	for _, m := range g.models {
		for _, r := range m.rels {
			count[pair{r.child, r.parent}]++
		}
	}
	var added []*relation
	for _, m := range g.models {
		for _, r := range m.rels {
			inv := &relation{kind: "has_many", target: r.child.name, child: r.child, parent: r.parent, fkField: r.fkField}
			name := pascal(splitWords(g.o.Namer.TableName(r.child.name)), r.child.name)
			if g.fkUnique(r.child, r.fkField) {
				inv.kind = "has_one"
				name = r.child.name
			}
			if count[pair{r.child, r.parent}] > 1 || r.child == r.parent {
				base := strings.TrimSuffix(strings.TrimSuffix(r.fkField, "ID"), "Id")
				if base == "" {
					base = r.fkField
				}
				name += "By" + base
			}
			inv.opts = []string{inv.kind, "fk:" + r.fkField}
			if rule, ok := g.o.Config.Relations[r.parent.name+"."+name]; ok {
				if rule.Skip {
					continue
				}
				if rule.Name != "" {
					name = rule.Name
				}
			}
			inv.name = name
			added = append(added, inv)
		}
	}
	// Name them in a fixed order, so a collision always resolves the same way.
	sort.SliceStable(added, func(i, j int) bool {
		a, b := added[i], added[j]
		if a.child.t.Name != b.child.t.Name {
			return a.child.t.Name < b.child.t.Name
		}
		return a.fkField < b.fkField
	})
	for _, inv := range added {
		inv.name = uniqueName(inv.parent.used, inv.name)
		inv.parent.rels = append(inv.parent.rels, inv)
	}
}

// fkUnique reports whether the single column behind the Go field fkField of m
// is unique on its own: the whole primary key, a unique constraint, or a plain
// unique index with no predicate.
func (g *generator) fkUnique(m *model, fkField string) bool {
	col := ""
	for c, f := range m.byCol {
		if f.name == fkField {
			col = c
		}
	}
	if col == "" {
		return false
	}
	t := m.t
	return singleColumnUnique(t, col)
}

func singleColumnUnique(t *schema.Table, col string) bool {
	if len(t.PrimaryKey) == 1 && t.PrimaryKey[0] == col {
		return true
	}
	for _, u := range t.Uniques {
		if len(u.Columns) == 1 && u.Columns[0] == col {
			return true
		}
	}
	for _, ix := range t.Indexes {
		if ix.Unique && ix.Where == "" && len(ix.Parts) == 1 && ix.Parts[0].Expr == "" && ix.Parts[0].Column == col {
			return true
		}
	}
	return false
}
