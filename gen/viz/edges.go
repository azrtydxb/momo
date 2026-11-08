// SPDX-License-Identifier: Apache-2.0

// Package viz draws entity-relationship diagrams of momo models or of a
// migration snapshot, as Mermaid erDiagram text or Graphviz DOT. It is a
// gen.Plugin and the engine behind the momo viz command.
package viz

import (
	"sort"
	"strings"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/schema"
)

// Cardinality values of an Edge.
const (
	OneToOne   = "one-to-one"
	OneToMany  = "one-to-many"
	ManyToMany = "many-to-many"
)

// Edge is one relation between two tables. For OneToOne and OneToMany, From
// is the referencing (child) table, whose FromColumns hold the foreign key,
// and To is the referenced (parent) table with ToColumns its key. For
// ManyToMany the two ends are in declaration order and Label names the join
// table.
type Edge struct {
	From, To    string
	FromColumns []string
	ToColumns   []string
	Cardinality string
	Label       string
}

// EdgesFromGraph returns one edge per relation of the models. A relation
// declared on both sides (for example has_many and the matching belongs_to)
// yields one edge. The result is sorted by From, To and Label.
func EdgesFromGraph(g *gen.Graph) []Edge {
	var out []Edge
	index := map[string]int{}
	add := func(key string, e Edge) {
		if i, ok := index[key]; ok {
			if e.Cardinality == OneToOne {
				out[i].Cardinality = OneToOne // a has_one is the stronger statement
			}
			return
		}
		index[key] = len(out)
		out = append(out, e)
	}
	for _, m := range g.Models {
		for _, r := range m.Relations {
			t := r.Target
			if t == nil {
				continue
			}
			switch r.Kind {
			case gen.BelongsTo:
				e := Edge{From: m.Table, To: t.Table, FromColumns: columnOf(m, r.FK), ToColumns: keyOf(g, t), Cardinality: OneToMany}
				e.Label = strings.Join(e.FromColumns, ", ")
				add("fk|"+e.From+"|"+e.To+"|"+e.Label, e)
			case gen.HasOne, gen.HasMany:
				e := Edge{From: t.Table, To: m.Table, FromColumns: columnOf(t, r.FK), ToColumns: keyOf(g, m), Cardinality: OneToMany}
				if r.Kind == gen.HasOne {
					e.Cardinality = OneToOne
				}
				e.Label = strings.Join(e.FromColumns, ", ")
				add("fk|"+e.From+"|"+e.To+"|"+e.Label, e)
			case gen.ManyToMany:
				a, b := m.Table, t.Table
				if b < a {
					a, b = b, a
				}
				e := Edge{From: m.Table, To: t.Table, Cardinality: ManyToMany, Label: r.JoinTable}
				add("m2m|"+a+"|"+b+"|"+r.JoinTable, e)
			}
		}
	}
	sortEdges(out)
	return out
}

// EdgesFromSchema returns one edge per foreign key of the snapshot: From the
// referencing table, To the referenced table, always OneToMany. The result is
// sorted by From, To and Label.
func EdgesFromSchema(s *schema.Schema) []Edge {
	var out []Edge
	for _, t := range s.Tables {
		for _, fk := range t.ForeignKeys {
			out = append(out, Edge{
				From:        t.Name,
				To:          fk.RefTable,
				FromColumns: append([]string(nil), fk.Columns...),
				ToColumns:   append([]string(nil), fk.RefColumns...),
				Cardinality: OneToMany,
				Label:       strings.Join(fk.Columns, ", "),
			})
		}
	}
	sortEdges(out)
	return out
}

// Tables returns the tables of the graph's schema, join tables included,
// sorted by name.
func Tables(g *gen.Graph) []schema.Table { return TablesFromSchema(g.Schema) }

// TablesFromSchema returns the tables of s sorted by name.
func TablesFromSchema(s *schema.Schema) []schema.Table {
	if s == nil {
		return nil
	}
	out := append([]schema.Table(nil), s.Tables...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Schema < out[j].Schema
	})
	return out
}

func sortEdges(es []Edge) {
	sort.SliceStable(es, func(i, j int) bool {
		a, b := es[i], es[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		return a.Label < b.Label
	})
}

// columnOf returns the column of the field named goName on m.
func columnOf(m *gen.Model, goName string) []string {
	for _, f := range m.Fields {
		if f.GoName == goName {
			return []string{f.Column}
		}
	}
	return nil
}

// keyOf returns the primary key columns of m's table.
func keyOf(g *gen.Graph, m *gen.Model) []string {
	if g.Schema == nil {
		return nil
	}
	if t, ok := g.Schema.Table(m.Table); ok {
		return append([]string(nil), t.PrimaryKey...)
	}
	return nil
}
