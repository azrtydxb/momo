// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"slices"

	"github.com/azrtydxb/momo/schema"
)

// deferKeyDependentFKs moves a foreign key out of its create_table and into an
// add_fk change when the columns it references only become a key later in the
// plan.
//
// A create_table carries its foreign keys inline, but the key a foreign key
// needs on the referenced columns may come from a create_index (a unique
// index), an add_unique or an add_primary_key, and those run after every
// create_table. PostgreSQL then rejects the table (SQLSTATE 42830, no unique
// constraint matching given keys), and MySQL and MariaDB need the index to
// exist as well. add_fk runs after all of them, so the foreign key is created
// there instead. A foreign key whose referenced columns are keyed by a
// constraint written in the create_table itself, or already in the database,
// stays inline: nothing in the plan changes its target. It returns changes
// unchanged when no foreign key needs the move, and never modifies the
// changes it was given: the create_table payload is copied.
func deferKeyDependentFKs(changes []Change) []Change {
	pending := map[string][][]string{}
	for _, c := range changes {
		cols := keyColumnsOf(c)
		if len(cols) == 0 {
			continue
		}
		k := qualified(c.Schema, "", c.Table)
		pending[k] = append(pending[k], sortedCols(cols))
	}
	if len(pending) == 0 {
		return changes
	}
	var out, added []Change
	for i, c := range changes {
		t := tableOf(c)
		if c.Kind != CreateTable || t == nil {
			continue
		}
		var keep []schema.ForeignKey
		for _, fk := range t.ForeignKeys {
			if !awaitsPendingKey(fk, t.Schema, pending) {
				keep = append(keep, fk)
				continue
			}
			added = append(added, Change{Kind: AddFK, Schema: t.Schema, Table: t.Name, Name: fk.Name, After: &fk})
		}
		if len(keep) == len(t.ForeignKeys) {
			continue
		}
		if out == nil {
			out = slices.Clone(changes)
		}
		cp := *t
		cp.ForeignKeys = keep
		out[i].After = &cp
	}
	if out == nil {
		return changes
	}
	return append(out, added...)
}

// awaitsPendingKey reports whether fk references columns that a change of the
// plan makes unique.
func awaitsPendingKey(fk schema.ForeignKey, ownSchema string, pending map[string][][]string) bool {
	if len(fk.RefColumns) == 0 {
		return false
	}
	want := sortedCols(fk.RefColumns)
	for _, key := range []string{fk.RefTable, qualified(ownSchema, "", fk.RefTable)} {
		for _, have := range pending[key] {
			if slices.Equal(have, want) {
				return true
			}
		}
	}
	return false
}

func sortedCols(cols []string) []string {
	out := slices.Clone(cols)
	slices.Sort(out)
	return out
}

// keyColumnsOf returns the columns that change c makes a unique key of its
// table, or nil. Only a plain unique index counts: a partial or expression
// index cannot be the target of a foreign key.
func keyColumnsOf(c Change) []string {
	switch c.Kind {
	case CreateIndex:
		var ix schema.Index
		switch v := c.After.(type) {
		case *schema.Index:
			if v == nil {
				return nil
			}
			ix = *v
		case schema.Index:
			ix = v
		default:
			return nil
		}
		if !ix.Unique || ix.Where != "" {
			return nil
		}
		cols := make([]string, 0, len(ix.Parts))
		for _, p := range ix.Parts {
			if p.Column == "" || p.Expr != "" {
				return nil
			}
			cols = append(cols, p.Column)
		}
		return cols
	case AddUnique:
		switch v := c.After.(type) {
		case *schema.Unique:
			if v != nil {
				return v.Columns
			}
		case schema.Unique:
			return v.Columns
		}
	case AddPrimaryKey:
		if cols, ok := c.After.([]string); ok {
			return cols
		}
	}
	return nil
}
