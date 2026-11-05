// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/azrtydxb/momo/schema"
)

// dependent is implemented by managed objects that report the names of the
// objects they depend on.
type dependent interface{ dependsOn() []string }

// phases lists the change kinds in execution order. Each inner slice is one
// phase; kinds within a phase keep their input order, except where a phase is
// dependency-sorted (see sortedPhase).
//
// Renames come first so that everything else sees the new names. Then the
// dependents of altered or dropped objects go (triggers, policies, views,
// foreign keys, indexes, constraints, columns, tables). Creates follow in
// dependency order. Functions and enums that are no longer used go last,
// after every statement that could still reference them.
var phases = [][]ChangeKind{
	{RenameTable, RenameColumn},
	{DropTrigger, DropPolicy},
	{DropView},
	{DropFK},
	{DropIndex, DropCheck, DropUnique, DropPrimaryKey, DropColumn},
	{DropTable},
	{CreateSchema},
	{CreateExtension},
	{CreateEnum, AlterEnumAddValue},
	{CreateTable},
	{AddColumn},
	{AlterColumn},
	{AddPrimaryKey, AddUnique, AddCheck},
	{CreateIndex},
	{AddFK},
	{CreateFunction, ReplaceFunction},
	{CreateView},
	{CreateTrigger},
	{CreatePolicy},
	{SetRLS},
	{DropFunction},
	{DropEnum},
}

// Order sorts changes into an executable sequence.
//
// Creates run in the order create_schema, create_extension, create_enum,
// create_table (parents before partitions, referenced tables before
// referencing ones), add_column, alter_column, constraints, create_index, add_fk,
// create_function, create_view (dependencies first), create_trigger,
// create_policy, set_rls. Drops run first: drop_trigger, drop_policy,
// drop_view (dependents first), drop_fk, drop_index and the other drops of
// constraints and columns, drop_table (partitions before parents), and, last
// of all, drop_function and drop_enum. Renames run before everything else.
//
// Renames run before every drop, so renaming A to B while dropping a
// different table B in the same plan would clash; Diff does not produce that.
//
// Order never panics and never modifies its argument. If the changes cannot
// be ordered it returns them unchanged, silently. Engine code must call
// OrderErr (or OrderWithViews) and surface the cycle error instead.
func Order(changes []Change) []Change {
	out, err := OrderErr(changes)
	if err != nil {
		return changes
	}
	return out
}

// OrderErr is Order with the error reported. The error is non-nil when a
// dependency cycle cannot be broken; it names the objects in the cycle. A
// foreign-key cycle among created tables is broken by creating the tables
// without those foreign keys and adding them afterwards with add_fk changes.
func OrderErr(changes []Change) ([]Change, error) {
	return OrderWithViews(changes, nil, nil)
}

// OrderWithViews is OrderErr plus view-dependent expansion. from and to are
// the views of the snapshot and of the desired schema. Every view in from that
// transitively depends on a view with a drop_view change, and that is not
// itself dropped, gets a drop_view (from) and a create_view (to) added, so a
// dependent that is not otherwise in the diff is rebuilt after its base.
func OrderWithViews(changes []Change, from, to []schema.View) ([]Change, error) {
	changes = expandViewDependents(changes, from, to)
	rank := map[ChangeKind]int{}
	for i, p := range phases {
		for _, k := range p {
			rank[k] = i
		}
	}
	buckets := make([][]Change, len(phases)+1)
	for _, c := range changes {
		r, ok := rank[c.Kind]
		if !ok {
			r = len(phases) // unknown kinds keep their input order, at the end
		}
		buckets[r] = append(buckets[r], c)
	}

	out := make([]Change, 0, len(changes))
	var deferred []Change
	for i, b := range buckets {
		if len(b) == 0 {
			continue
		}
		var (
			sorted []Change
			err    error
		)
		switch {
		case i == len(phases):
			sorted = b
		case slices.Contains(phases[i], CreateTable):
			sorted, deferred, err = sortTables(b, true)
		case slices.Contains(phases[i], DropTable):
			sorted, _, err = sortTables(b, false)
		case slices.Contains(phases[i], CreateView):
			sorted, err = sortByDeps(b, depsOf, true)
		case slices.Contains(phases[i], DropView):
			sorted, err = sortByDeps(b, depsOf, false)
		default:
			sorted = b
		}
		if err != nil {
			return nil, err
		}
		if slices.Contains(phases[i], AddFK) {
			sorted = append(slices.Clone(sorted), deferred...)
		}
		out = append(out, sorted...)
	}
	if len(deferred) > 0 && len(buckets[rank[AddFK]]) == 0 {
		// No add_fk phase ran, so the deferred foreign keys were never emitted.
		// They belong right after the add_fk position, which is after
		// create_index (a unique index may be what the foreign key references).
		pos := 0
		for i, c := range out {
			r, ok := rank[c.Kind]
			if !ok {
				continue
			}
			if r < rank[AddFK] {
				pos = i + 1
			}
		}
		out = slices.Insert(out, pos, deferred...)
	}
	return out, nil
}

// objKey returns the qualified name used to match dependencies.
func objKey(c Change) string { return qualified(c.Schema, "", c.Name) }

// depsOf returns the dependencies reported by the managed object in c, which is
// c.After for creates and c.Before for drops. Names are returned as written;
// resolution against other changes is done by resolveDep.
func depsOf(c Change) []string {
	obj := c.After
	if c.Before != nil && c.After == nil {
		obj = c.Before
	}
	switch v := obj.(type) {
	case *schema.View:
		if v != nil {
			return v.Depends
		}
	case schema.View:
		return v.Depends
	case dependent:
		return v.dependsOn()
	}
	return nil
}

// resolveDep matches a dependency name against the known keys: first as
// written, then qualified with the dependent's own schema.
func resolveDep(dep, ownSchema string, keys map[string]int) (int, bool) {
	if i, ok := keys[dep]; ok {
		return i, true
	}
	if ownSchema != "" && !strings.Contains(dep, ".") {
		if i, ok := keys[ownSchema+"."+dep]; ok {
			return i, true
		}
	}
	return 0, false
}

// sortByDeps is a stable Kahn sort: among ready nodes the one with the lowest
// input index goes first. With dependenciesFirst a node follows the nodes it
// depends on (creates); otherwise it precedes them (drops).
func sortByDeps(items []Change, deps func(Change) []string, dependenciesFirst bool) ([]Change, error) {
	keys := make(map[string]int, len(items))
	for i, c := range items {
		keys[objKey(c)] = i
	}
	edges := make([][]int, len(items)) // edges[i]: nodes that must come before i
	for i, c := range items {
		for _, d := range deps(c) {
			j, ok := resolveDep(d, c.Schema, keys)
			if !ok || j == i {
				continue
			}
			if dependenciesFirst {
				edges[i] = append(edges[i], j)
			} else {
				edges[j] = append(edges[j], i)
			}
		}
	}
	order, cyc := kahn(len(items), edges)
	if cyc != nil {
		return nil, cycleError(items, cyc)
	}
	out := make([]Change, len(items))
	for n, i := range order {
		out[n] = items[i]
	}
	return out, nil
}

// kahn returns a topological order of 0..n-1 where edges[i] lists the nodes
// that must precede i, preferring the lowest index among ready nodes. If a
// cycle exists it returns the nodes on cycles (or between cycles) instead.
func kahn(n int, edges [][]int) (order, cycle []int) {
	indeg := make([]int, n)
	next := make([][]int, n)
	for i, ps := range edges {
		seen := map[int]bool{}
		for _, p := range ps {
			if seen[p] {
				continue
			}
			seen[p] = true
			indeg[i]++
			next[p] = append(next[p], i)
		}
	}
	var ready []int
	for i := range n {
		if indeg[i] == 0 {
			ready = append(ready, i)
		}
	}
	for len(ready) > 0 {
		i := ready[0]
		ready = ready[1:]
		order = append(order, i)
		for _, m := range next[i] {
			indeg[m]--
			if indeg[m] == 0 {
				k := sort.SearchInts(ready, m)
				ready = slices.Insert(ready, k, m)
			}
		}
	}
	if len(order) == n {
		return order, nil
	}
	// Prune remaining nodes nobody else waits on, leaving the cycles.
	left := map[int]bool{}
	for i := range n {
		if indeg[i] > 0 {
			left[i] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for i := range left {
			waited := false
			for _, m := range next[i] {
				if left[m] {
					waited = true
					break
				}
			}
			if !waited {
				delete(left, i)
				changed = true
			}
		}
	}
	for i := range n {
		if left[i] {
			cycle = append(cycle, i)
		}
	}
	return order, cycle
}

func cycleError(items []Change, idxs []int) error {
	names := make([]string, 0, len(idxs))
	for _, i := range idxs {
		names = append(names, fmt.Sprintf("%s %s", items[i].Kind, objKey(items[i])))
	}
	return fmt.Errorf("migrate: dependency cycle among %s", strings.Join(names, ", "))
}

// tableOf returns the table carried by a create_table (After) or drop_table
// (Before) change.
func tableOf(c Change) *schema.Table {
	obj := c.After
	if c.Kind == DropTable {
		obj = c.Before
	}
	switch t := obj.(type) {
	case *schema.Table:
		return t
	case schema.Table:
		return &t
	}
	return nil
}

func tableKeyOf(c Change) string {
	if t := tableOf(c); t != nil {
		return qualified(t.Schema, "", t.Name)
	}
	return qualified(c.Schema, "", c.Name)
}

// sortTables orders create_table changes (referenced tables and partition
// parents first) or drop_table changes (partitions before their parents;
// foreign keys of dropped tables are separate drop_fk changes that run
// earlier). On create, a cycle of foreign keys is broken by moving the
// foreign keys inside the cycle out of the create_table (on a copy) into the
// returned add_fk changes.
func sortTables(items []Change, create bool) (sorted, addFKs []Change, err error) {
	items = slices.Clone(items)
	keys := make(map[string]int, len(items))
	for i, c := range items {
		keys[tableKeyOf(c)] = i
	}
	build := func() [][]int {
		edges := make([][]int, len(items))
		for i, c := range items {
			t := tableOf(c)
			if t == nil {
				continue
			}
			link := func(ref string, always bool) {
				j, ok := resolveDep(ref, t.Schema, keys)
				if !ok || j == i {
					return
				}
				switch {
				case create:
					edges[i] = append(edges[i], j) // referenced first
				case always:
					edges[j] = append(edges[j], i) // child before parent
				}
			}
			if t.PartitionOf != nil {
				link(t.PartitionOf.Parent, true)
			}
			if create {
				for _, fk := range t.ForeignKeys {
					link(fk.RefTable, false)
				}
			}
		}
		return edges
	}

	edges := build()
	order, cyc := kahn(len(items), edges)
	if cyc != nil && create {
		// Break cycles: strip the foreign keys that point into the cycle.
		inCycle := map[int]bool{}
		for _, i := range cyc {
			inCycle[i] = true
		}
		stripped := false
		for _, i := range cyc {
			t := tableOf(items[i])
			if t == nil {
				continue
			}
			cp := *t
			cp.ForeignKeys = nil
			for _, fk := range t.ForeignKeys {
				j, ok := resolveDep(fk.RefTable, t.Schema, keys)
				if ok && j != i && inCycle[j] {
					addFKs = append(addFKs, Change{Kind: AddFK, Schema: t.Schema, Table: t.Name, Name: fk.Name, After: &schema.ForeignKey{
						Name: fk.Name, Columns: fk.Columns, RefTable: fk.RefTable, RefColumns: fk.RefColumns, OnDelete: fk.OnDelete, OnUpdate: fk.OnUpdate,
					}})
					stripped = true
					continue
				}
				cp.ForeignKeys = append(cp.ForeignKeys, fk)
			}
			items[i].After = &cp
		}
		if stripped {
			order, cyc = kahn(len(items), build())
		}
	}
	if cyc != nil {
		return nil, nil, cycleError(items, cyc)
	}
	sorted = make([]Change, len(items))
	for n, i := range order {
		sorted[n] = items[i]
	}
	return sorted, addFKs, nil
}

// expandViewDependents adds drop_view and create_view changes for the
// transitive dependents of dropped or changed views. The input is not modified.
func expandViewDependents(changes []Change, from, to []schema.View) []Change {
	if len(from) == 0 {
		return changes
	}
	keys := make(map[string]int, len(from))
	for i := range from {
		keys[qualified(from[i].Schema, "", from[i].Name)] = i
	}
	toByKey := map[string]*schema.View{}
	for i := range to {
		toByKey[qualified(to[i].Schema, "", to[i].Name)] = &to[i]
	}
	dropped, created := map[string]bool{}, map[string]bool{}
	var queue []int
	for _, c := range changes {
		switch c.Kind {
		case DropView:
			dropped[objKey(c)] = true
			if i, ok := keys[objKey(c)]; ok {
				queue = append(queue, i)
			}
		case CreateView:
			created[objKey(c)] = true
		}
	}
	seen := map[int]bool{}
	for _, i := range queue {
		seen[i] = true
	}
	out := slices.Clone(changes)
	for len(queue) > 0 {
		b := queue[0]
		queue = queue[1:]
		for i := range from {
			if seen[i] {
				continue
			}
			dependsOnB := false
			for _, d := range from[i].Depends {
				if j, ok := resolveDep(d, from[i].Schema, keys); ok && j == b {
					dependsOnB = true
					break
				}
			}
			if !dependsOnB {
				continue
			}
			seen[i] = true
			queue = append(queue, i)
			v := from[i]
			k := qualified(v.Schema, "", v.Name)
			if !dropped[k] {
				out = append(out, Change{Kind: DropView, Schema: v.Schema, Name: v.Name, Before: &from[i]})
			}
			if nv, ok := toByKey[k]; ok && !created[k] {
				out = append(out, Change{Kind: CreateView, Schema: nv.Schema, Name: nv.Name, After: nv})
			}
		}
	}
	return out
}
