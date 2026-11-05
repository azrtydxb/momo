// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"go/token"
	"go/types"

	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/gen/tags"
)

// graphNamer returns the naming preset a graph was loaded with, defaulting to
// naming.Momo when the graph is nil or carries none.
func graphNamer(g *Graph) naming.Namer {
	if g != nil && g.Namer != nil {
		return g.Namer
	}
	return naming.Momo{}
}

// joinColumnName is the join table column that references model m's primary
// key column pkColumn: the model's name as a column, an underscore and the
// key column. The join table's schema and the relation descriptor both use
// it, so they always agree.
func joinColumnName(namer naming.Namer, m *Model, pkColumn string) string {
	return namer.ColumnName(m.GoName) + "_" + pkColumn
}

// isRelationTag reports whether a field's tags declare a relation.
func isRelationTag(t tags.Tags) bool {
	return t.BelongsTo || t.HasOne || t.HasMany || t.ManyToMany || t.Rel != ""
}

// newRelation records a relation field. Kind, target and the other checks
// wait for resolveRelations, when every model of the load is known.
func newRelation(v *types.Var, t tags.Tags, fset *token.FileSet) *Relation {
	return &Relation{
		GoName:       v.Name(),
		FK:           t.Fk,
		JoinTable:    t.Join,
		PolyType:     t.PolyType,
		PolyValue:    t.PolyValue,
		OnDelete:     t.OnDelete,
		OnUpdate:     t.OnUpdate,
		NoConstraint: t.NoConstraint,
		Pos:          fset.Position(v.Pos()),
		tags:         t,
		ftype:        v.Type(),
	}
}

// relationKinds returns the distinct kinds a tag set names, in a fixed order.
func relationKinds(t tags.Tags) []RelKind {
	var ks []RelKind
	add := func(k RelKind) {
		for _, e := range ks {
			if e == k {
				return
			}
		}
		ks = append(ks, k)
	}
	if t.BelongsTo || t.Rel == "belongs_to" {
		add(BelongsTo)
	}
	if t.HasOne || t.Rel == "has_one" {
		add(HasOne)
	}
	if t.HasMany || t.Rel == "has_many" {
		add(HasMany)
	}
	if t.ManyToMany || t.Rel == "many2many" {
		add(ManyToMany)
	}
	return ks
}

func (k RelKind) tagName() string {
	switch k {
	case BelongsTo:
		return "belongs_to"
	case HasOne:
		return "has_one"
	case HasMany:
		return "has_many"
	case ManyToMany:
		return "many2many"
	}
	return "relation"
}

// relationTarget unwraps the declared type of a relation field: *T for
// to-one kinds, []T for to-many kinds.
func relationTarget(k RelKind, t types.Type) *types.Named {
	t = types.Unalias(t)
	switch k {
	case BelongsTo, HasOne:
		if p, ok := t.(*types.Pointer); ok {
			n, _ := types.Unalias(p.Elem()).(*types.Named)
			return n
		}
	case HasMany, ManyToMany:
		if s, ok := t.(*types.Slice); ok {
			n, _ := types.Unalias(s.Elem()).(*types.Named)
			return n
		}
	}
	return nil
}

func relationShape(k RelKind) string {
	if k == BelongsTo || k == HasOne {
		return "*T"
	}
	return "[]T"
}

func fieldByGoName(m *Model, name string) *Field {
	for _, f := range m.Fields {
		if f.GoName == name {
			return f
		}
	}
	return nil
}

// resolveRelations sets Kind and Target on every relation of models and
// returns the diagnostics plus the packages that have an invalid relation;
// such a package yields no output.
func resolveRelations(models []*Model) ([]Diagnostic, map[string]bool) {
	byNamed := map[*types.Named]*Model{}
	for _, m := range models {
		if m.named != nil {
			byNamed[m.named] = m
		}
	}
	var diags []Diagnostic
	bad := map[string]bool{}
	for _, m := range models {
		for _, r := range m.Relations {
			before := len(diags)
			diags = resolveRelation(diags, m, r, byNamed)
			if len(diags) > before {
				bad[m.PkgPath] = true
			}
		}
	}
	return diags, bad
}

func resolveRelation(diags []Diagnostic, m *Model, r *Relation, byNamed map[*types.Named]*Model) []Diagnostic {
	fail := func(msg string) []Diagnostic { return append(diags, Diagnostic{Pos: r.Pos, Msg: msg}) }
	ks := relationKinds(r.tags)
	if len(ks) != 1 {
		return fail("relation " + r.GoName + " must have exactly one kind")
	}
	r.Kind = ks[0]
	if r.PolyType != "" || r.PolyValue != "" {
		if r.PolyType == "" || r.PolyValue == "" {
			return fail("polymorphic relation " + r.GoName + " needs both polytype and polyvalue")
		}
		if r.Kind != HasOne && r.Kind != HasMany {
			return fail("polymorphic is only valid for has_one and has_many")
		}
	}
	named := relationTarget(r.Kind, r.ftype)
	if named == nil {
		return fail("relation " + r.GoName + " must be declared as " + relationShape(r.Kind) + " for " + r.Kind.tagName())
	}
	target := byNamed[named]
	if target == nil {
		return fail("relation target " + named.Obj().Name() + " is not a momo model")
	}
	r.Target = target
	switch r.Kind {
	case ManyToMany:
		if r.JoinTable == "" {
			return fail("many2many relation " + r.GoName + " needs join:<table>")
		}
	case BelongsTo:
		if r.FK == "" {
			return fail(r.Kind.tagName() + " relation " + r.GoName + " needs fk:<LocalField>")
		}
		if fieldByGoName(m, r.FK) == nil {
			return fail("relation " + r.GoName + ": foreign key field " + r.FK + " is not a field of " + m.GoName)
		}
	default:
		if r.FK == "" {
			return fail(r.Kind.tagName() + " relation " + r.GoName + " needs fk:<ChildField>")
		}
		if fieldByGoName(target, r.FK) == nil {
			return fail("relation " + r.GoName + ": foreign key field " + r.FK + " is not a field of " + target.GoName)
		}
	}
	if r.PolyType != "" {
		found := false
		for _, f := range target.Fields {
			if f.Column == r.PolyType {
				found = true
			}
		}
		if !found {
			return fail("polymorphic column " + r.PolyType + " is not a field of " + target.GoName)
		}
	}
	return diags
}
