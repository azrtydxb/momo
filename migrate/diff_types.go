// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/azrtydxb/momo/schema"
)

// ----------------------------------------------- domains and composite types

func domainKey(x *schema.Domain) string { return x.Schema + "\x00" + x.Name }

func compositeKey(x *schema.Composite) string { return x.Schema + "\x00" + x.Name }

// typeColumns lists the columns of the tables of the snapshot that use the SQL
// type ref (spelled as a column spells it, lower case), or an array of it, and
// still exist in the desired schema, under their new names when a rename moves
// them. Reattach is set for the ones that still use the type there. A column
// of a dropped table, or one the plan drops, is dropped before a type changes
// and is not listed.
func (d *differ) typeColumns(ref string) []DomainColumn {
	toTables := map[string]*schema.Table{}
	for i := range d.toNorm.Tables {
		toTables[tableKey(&d.toNorm.Tables[i])] = &d.toNorm.Tables[i]
	}
	uses := func(t string) bool { return strings.TrimSuffix(d.typ(t), "[]") == ref }
	isArray := func(t string) bool { return strings.HasSuffix(d.typ(t), "[]") }
	var out []DomainColumn
	for i := range d.fromNorm.Tables {
		ft := &d.fromNorm.Tables[i]
		fk := tableKey(ft)
		tk := fk
		if r, ok := d.tblRen[fk]; ok {
			tk = r
		}
		tt := toTables[tk]
		if tt == nil {
			continue
		}
		for _, fc := range ft.Columns {
			if !uses(fc.Type) {
				continue
			}
			name := fc.Name
			if r, ok := d.colRen[fk][fc.Name]; ok {
				name = r
			}
			tc, ok := tt.Column(name)
			if !ok {
				continue
			}
			// A column that changes between the domain and an array of it is
			// left off the domain: the plan changes its type itself.
			out = append(out, DomainColumn{
				Schema: tt.Schema, Table: tt.Name, Column: name,
				Reattach: uses(tc.Type) && isArray(tc.Type) == isArray(fc.Type), Array: isArray(fc.Type),
			})
		}
	}
	return out
}

// domains diffs the domains. A new one is created and a missing one dropped. A
// change of the default, NOT NULL or the constraints is one alter_domain, and a
// change of the base type, which PostgreSQL cannot alter in place, is one
// recreate_domain that takes the columns off the domain and puts them back; it
// is destructive when a column uses the domain, because it rewrites the column
// and can fail on a value the new type rejects.
func (d *differ) domains(from, to *schema.Schema) {
	index := func(xs []schema.Domain) map[string]*schema.Domain {
		m := make(map[string]*schema.Domain, len(xs))
		for i := range xs {
			m[domainKey(&xs[i])] = &xs[i]
		}
		return m
	}
	fromN, toN, fromO, toO := index(d.fromNorm.Domains), index(d.toNorm.Domains), index(from.Domains), index(to.Domains)
	for i := range d.toNorm.Domains {
		n := &d.toNorm.Domains[i]
		k := domainKey(n)
		o, ok := fromN[k]
		switch {
		case !ok:
			d.emit(Change{Kind: CreateDomain, Schema: n.Schema, Name: n.Name, After: cp(*toO[k])})
		case reflect.DeepEqual(*o, *n):
		case o.Type != n.Type:
			uses := d.typeColumns(strings.ToLower(o.Ref()))
			if why := d.recreateBlocker(o, uses); why != "" {
				d.errs = append(d.errs, fmt.Errorf("migrate: domain %s changes its base type, but %s; PostgreSQL cannot alter the base type and momo recreates a domain only under columns: write a custom migration", qualified(o.Schema, "", o.Name), why))
				continue
			}
			d.emit(Change{Kind: RecreateDomain, Schema: n.Schema, Name: n.Name, Before: cp(*fromO[k]), After: cp(*toO[k]), DomainColumns: uses, Destructive: len(uses) > 0})
		default:
			c := Change{Kind: AlterDomain, Schema: n.Schema, Name: n.Name, Before: cp(*fromO[k]), After: cp(*toO[k])}
			if stricterDomain(o, n) {
				c.Risky = true
				d.hint(fmt.Sprintf("risky: %s %s: the new NOT NULL or CHECK is validated against every column that uses the domain, and fails if a stored value breaks it", c.Kind, qualified(c.Schema, "", c.Name)))
			}
			d.emit(c)
		}
	}
	for i := range d.fromNorm.Domains {
		o := &d.fromNorm.Domains[i]
		k := domainKey(o)
		if _, ok := toN[k]; !ok {
			d.emit(Change{Kind: DropDomain, Schema: o.Schema, Name: o.Name, Before: cp(*fromO[k]), Destructive: true})
		}
	}
}

// recreateBlocker says why a domain whose base type changes cannot be recreated
// under its columns alone, or "" when it can: a composite type has an attribute
// of the domain, another domain is based on it, or a view that the plan leaves as
// it is reads a table with a column of the domain (PostgreSQL refuses to change
// the type of a column that a view reads, and a view the plan drops or changes
// is dropped before the columns change and created again after).
func (d *differ) recreateBlocker(o *schema.Domain, uses []DomainColumn) string {
	ref := strings.ToLower(o.Ref())
	of := func(t string) bool { return strings.TrimSuffix(d.typ(t), "[]") == ref }
	for _, c := range d.fromNorm.Composites {
		for _, a := range c.Attributes {
			if of(a.Type) {
				return fmt.Sprintf("the attribute %q of the composite type %s is of the domain", a.Name, c.Ref())
			}
		}
	}
	for _, x := range d.fromNorm.Domains {
		if x.Ref() != o.Ref() && of(x.Type) {
			return fmt.Sprintf("the domain %s is based on it", x.Ref())
		}
	}
	toViews := map[string]*schema.View{}
	for i := range d.toNorm.Views {
		toViews[viewKey(&d.toNorm.Views[i])] = &d.toNorm.Views[i]
	}
	for _, u := range uses {
		for i := range d.fromNorm.Views {
			v := &d.fromNorm.Views[i]
			reads := false
			for _, dep := range v.Depends {
				reads = reads || dep == u.Table || (u.Schema != "" && dep == u.Schema+"."+u.Table)
			}
			if nv, ok := toViews[viewKey(v)]; reads && ok && reflect.DeepEqual(*v, *nv) {
				return fmt.Sprintf("the view %s reads the table %s, and PostgreSQL refuses to change the type of a column that a view reads", qualified(v.Schema, "", v.Name), u.Table)
			}
		}
	}
	return ""
}

// stricterDomain reports whether n refuses values o accepts: it adds NOT NULL,
// or adds or changes a CHECK constraint.
func stricterDomain(o, n *schema.Domain) bool {
	if n.NotNull && !o.NotNull {
		return true
	}
	old := map[string]string{}
	for _, c := range o.Checks {
		old[c.Name] = c.Expr
	}
	for _, c := range n.Checks {
		if e, ok := old[c.Name]; !ok || e != c.Expr {
			return true
		}
	}
	return false
}

// composites diffs the composite types. A new one is created and a missing one
// dropped. A change of the attributes is one alter_composite, which adds,
// drops and retypes attributes with ALTER TYPE; it is destructive when an
// attribute is dropped. PostgreSQL refuses ALTER TYPE on a type that a column
// uses, and cannot reorder attributes or insert one before another, so the
// differ refuses those plans with an error that says so, instead of leaving
// the failure to the migration.
func (d *differ) composites(from, to *schema.Schema) {
	index := func(xs []schema.Composite) map[string]*schema.Composite {
		m := make(map[string]*schema.Composite, len(xs))
		for i := range xs {
			m[compositeKey(&xs[i])] = &xs[i]
		}
		return m
	}
	fromN, toN, fromO, toO := index(d.fromNorm.Composites), index(d.toNorm.Composites), index(from.Composites), index(to.Composites)
	for i := range d.toNorm.Composites {
		n := &d.toNorm.Composites[i]
		k := compositeKey(n)
		o, ok := fromN[k]
		switch {
		case !ok:
			d.emit(Change{Kind: CreateComposite, Schema: n.Schema, Name: n.Name, After: cp(*toO[k])})
		case reflect.DeepEqual(*o, *n):
		default:
			d.alterComposite(o, n, fromO[k], toO[k])
		}
	}
	for i := range d.fromNorm.Composites {
		o := &d.fromNorm.Composites[i]
		k := compositeKey(o)
		if _, ok := toN[k]; !ok {
			d.emit(Change{Kind: DropComposite, Schema: o.Schema, Name: o.Name, Before: cp(*fromO[k]), Destructive: true})
		}
	}
}

// alterComposite emits the alter_composite change from o to n (normalized), or
// reports why PostgreSQL cannot make it. orig and want are the declarations as
// written, for the payload.
func (d *differ) alterComposite(o, n, orig, want *schema.Composite) {
	name := qualified(n.Schema, "", n.Name)
	old := map[string]string{}
	for _, a := range o.Attributes {
		old[a.Name] = a.Type
	}
	kept := map[string]bool{}
	for _, a := range n.Attributes {
		if _, ok := old[a.Name]; ok {
			kept[a.Name] = true
		}
	}
	dropped := false
	var survivors []string // the old order of the attributes that stay
	for _, a := range o.Attributes {
		if kept[a.Name] {
			survivors = append(survivors, a.Name)
		} else {
			dropped = true
		}
	}
	// The new order must be the survivors in their old order, then the new
	// attributes: ALTER TYPE appends an attribute and cannot move one.
	var want2 []string
	added := false
	for _, a := range n.Attributes {
		switch {
		case kept[a.Name] && added:
			d.errs = append(d.errs, fmt.Errorf("migrate: composite type %s: attribute %q comes after a new attribute, but PostgreSQL adds an attribute at the end and cannot reorder them; declare new attributes last, or write a custom migration", name, a.Name))
			return
		case kept[a.Name]:
			want2 = append(want2, a.Name)
		default:
			added = true
		}
	}
	if len(want2) != len(survivors) {
		return
	}
	for i := range survivors {
		if survivors[i] != want2[i] {
			d.errs = append(d.errs, fmt.Errorf("migrate: composite type %s: the attributes %q and %q change their order, but PostgreSQL cannot reorder the attributes of a type; write a custom migration", name, survivors[i], want2[i]))
			return
		}
	}
	if uses := d.typeColumns(strings.ToLower(o.Ref())); len(uses) > 0 {
		d.errs = append(d.errs, fmt.Errorf("migrate: composite type %s changes its attributes while the column %s.%s uses it, and PostgreSQL refuses ALTER TYPE on a type that a column uses; write a custom migration", name, uses[0].Table, uses[0].Column))
		return
	}
	d.emit(Change{Kind: AlterComposite, Schema: n.Schema, Name: n.Name, Before: cp(*orig), After: cp(*want), Destructive: dropped})
}
