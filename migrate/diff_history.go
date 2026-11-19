// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"slices"

	"github.com/azrtydxb/momo/schema"
)

// historyBases returns the tables of s that carry an audit history built by
// schema.HistoryFor: a table T with no schema, a table <T>_history, a function
// <T>_history_fn and a trigger <T>_history_trg on T. The result maps T to the
// index of its history table in s.Tables.
func historyBases(s *schema.Schema) map[string]int {
	out := map[string]int{}
	for i := range s.Tables {
		base := &s.Tables[i]
		if base.Schema != "" {
			continue
		}
		hist := schema.HistoryTableName(base.Name)
		hi := slices.IndexFunc(s.Tables, func(t schema.Table) bool { return t.Schema == "" && t.Name == hist })
		fnName, trgName := schema.HistoryFunctionName(base.Name), schema.HistoryTriggerName(base.Name)
		hasFn := slices.ContainsFunc(s.Functions, func(f schema.Function) bool { return f.Schema == "" && f.Name == fnName && f.Args == "" })
		hasTrg := slices.ContainsFunc(s.Triggers, func(g schema.Trigger) bool { return g.Schema == "" && g.Table == base.Name && g.Name == trgName })
		if hi >= 0 && hasFn && hasTrg {
			out[base.Name] = hi
		}
	}
	return out
}

// adaptHistory makes Diff treat the audit history tables as what they are: the
// record of every state of a model, which a change to the model must not erase.
// It returns to and opts adjusted for from, and never modifies its arguments.
//
//   - A --rename-table or --rename-column of a model with a history is repeated
//     on its history table, so the recorded versions keep their data. The
//     history function, trigger and index are rebuilt under the new names.
//   - A column the history table has and the model no longer has stays on the
//     history table, relaxed to nullable (the trigger no longer writes it), so
//     the old audit data survives. It is part of the desired schema, so the
//     snapshot records it and drift sees no difference. Dropping it is a manual
//     migration.
//
// The adjustment is idempotent: the returned schema and options give themselves
// back.
func adaptHistory(from, to *schema.Schema, opts DiffOptions) (*schema.Schema, DiffOptions) {
	fromH, toH := historyBases(from), historyBases(to)
	if len(fromH) == 0 || len(toH) == 0 {
		return to, opts
	}
	opts.RenameTables = cloneRenames(opts.RenameTables)
	opts.RenameColumns = cloneRenames(opts.RenameColumns)

	// Table renames: the history table follows its model's table.
	newBase := map[string]string{} // old base -> base in to
	for oldBase := range fromH {
		newBase[oldBase] = oldBase
		if n, ok := opts.RenameTables[oldBase]; ok {
			if _, wanted := toH[n]; wanted {
				if _, existed := fromH[n]; !existed {
					newBase[oldBase] = n
				}
			}
		}
	}
	for oldBase, nb := range newBase {
		if nb == oldBase {
			continue
		}
		if _, set := opts.RenameTables[schema.HistoryTableName(oldBase)]; !set {
			opts.RenameTables[schema.HistoryTableName(oldBase)] = schema.HistoryTableName(nb)
		}
	}

	out := *to
	out.Tables = slices.Clone(to.Tables)
	for _, oldBase := range sortedMapKeys(fromH) {
		nb := newBase[oldBase]
		ti, ok := toH[nb]
		if !ok {
			continue
		}
		fromHist, toHist := &from.Tables[fromH[oldBase]], &out.Tables[ti]

		// Column renames: a flag that names the model's table names the history
		// table too.
		renamed := map[string]string{}
		for _, key := range sortedMapKeys(opts.RenameColumns) {
			for _, tbl := range []string{oldBase, nb} {
				col, ok := cutDotPrefix(key, tbl)
				if !ok {
					continue
				}
				newCol := opts.RenameColumns[key]
				hk := fromHist.Name + "." + col
				if _, set := opts.RenameColumns[hk]; !set && hasColumn(fromHist, col) && hasColumn(toHist, newCol) && !hasColumn(fromHist, newCol) && !hasColumn(toHist, col) {
					opts.RenameColumns[hk] = newCol
				}
			}
		}
		for _, key := range sortedMapKeys(opts.RenameColumns) {
			if col, ok := cutDotPrefix(key, fromHist.Name); ok && hasColumn(fromHist, col) {
				renamed[col] = opts.RenameColumns[key]
			}
		}

		// A column the model dropped stays, nullable.
		var kept []schema.Column
		for _, c := range fromHist.Columns {
			if hasColumn(toHist, c.Name) {
				continue
			}
			if _, isRename := renamed[c.Name]; isRename {
				continue
			}
			c.Nullable = true
			kept = append(kept, c)
		}
		if len(kept) > 0 {
			toHist.Columns = append(slices.Clone(toHist.Columns), kept...)
		}
	}
	return &out, opts
}

// historyTriggersAfterRename returns from with the triggers of renamed history
// bases moved to the new table name, which is the name the table has when the
// plan drops them (renames run first). It never modifies from.
func historyTriggersAfterRename(from, to *schema.Schema, opts DiffOptions) *schema.Schema {
	fromH, toH := historyBases(from), historyBases(to)
	var out *schema.Schema
	for _, oldBase := range sortedMapKeys(fromH) {
		nb, ok := opts.RenameTables[oldBase]
		if !ok || nb == oldBase {
			continue
		}
		if _, wanted := toH[nb]; !wanted {
			continue
		}
		if _, existed := fromH[nb]; existed {
			continue
		}
		if out == nil {
			c := *from
			c.Triggers = slices.Clone(from.Triggers)
			out = &c
		}
		for i := range out.Triggers {
			if out.Triggers[i].Schema == "" && out.Triggers[i].Table == oldBase && out.Triggers[i].Name == schema.HistoryTriggerName(oldBase) {
				out.Triggers[i].Table = nb
			}
		}
	}
	if out == nil {
		return from
	}
	return out
}

func cloneRenames(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func hasColumn(t *schema.Table, name string) bool {
	return slices.ContainsFunc(t.Columns, func(c schema.Column) bool { return c.Name == name })
}

// cutDotPrefix returns the rest of key after "<prefix>.".
func cutDotPrefix(key, prefix string) (string, bool) {
	if len(key) > len(prefix)+1 && key[:len(prefix)] == prefix && key[len(prefix)] == '.' {
		return key[len(prefix)+1:], true
	}
	return "", false
}
