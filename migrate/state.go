// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"github.com/azrtydxb/momo/schema"
)

// A snapshot file records the schema its migration was generated to, and its
// parent is the migration it was generated from. Two branches that each add a
// migration therefore write snapshots that do not contain each other's work.
// The schema after a migration is derived instead of read: the migrations are
// folded in version order, and each one contributes only what it changed
// relative to its own parent (a three-way merge). A linear chain folds to
// exactly the stored snapshots.
//
// The unit of change is one object: a table with its triggers and policies, a
// view, an enum, an extension, a function or a namespace. Two migrations that
// change different objects commute (SQL on different tables does), so the fold
// accepts them in either order. Two that change the same object do not (a
// table rebuild or a DROP and ADD of a constraint restates the whole object),
// so the later one is an ErrBranchConflict naming the object and both
// migrations, unless both leave it identical.

// splitObjects breaks s into one single-object schema per key. A table's
// triggers and policies belong to its key, so a table and its triggers change
// together.
func splitObjects(s *schema.Schema) map[string]*schema.Schema {
	out := map[string]*schema.Schema{}
	one := func(key string) *schema.Schema {
		o, ok := out[key]
		if !ok {
			o = &schema.Schema{Dialect: s.Dialect}
			out[key] = o
		}
		return o
	}
	tables := map[string]bool{}
	for _, t := range s.Tables {
		tables[objectKey("table", t.Schema, t.Name)] = true
	}
	for _, x := range s.Schemas {
		o := one("schema:" + x.Name)
		o.Schemas = append(o.Schemas, x)
	}
	for _, x := range s.Extensions {
		o := one("extension:" + x.Name)
		o.Extensions = append(o.Extensions, x)
	}
	for _, x := range s.Enums {
		o := one("enum:" + x.Name)
		o.Enums = append(o.Enums, x)
	}
	for _, x := range s.Tables {
		o := one(objectKey("table", x.Schema, x.Name))
		o.Tables = append(o.Tables, x)
	}
	for _, x := range s.Views {
		o := one(objectKey("view", x.Schema, x.Name))
		o.Views = append(o.Views, x)
	}
	for _, x := range s.Functions {
		o := one(objectKey("function", x.Schema, x.Name+"("+x.Args+")"))
		o.Functions = append(o.Functions, x)
	}
	for _, x := range s.Triggers {
		key := objectKey("table", x.Schema, x.Table)
		if !tables[key] {
			key = objectKey("trigger", x.Schema, x.Table+"."+x.Name)
		}
		o := one(key)
		o.Triggers = append(o.Triggers, x)
	}
	for _, x := range s.Policies {
		key := objectKey("table", x.Schema, x.Table)
		if !tables[key] {
			key = objectKey("policy", x.Schema, x.Table+"."+x.Name)
		}
		o := one(key)
		o.Policies = append(o.Policies, x)
	}
	return out
}

// objectKey names one object: "<kind>:<schema>.<name>", or "<kind>:<name>" in
// the connection's current schema.
func objectKey(kind, ns, name string) string {
	if ns == "" {
		return kind + ":" + name
	}
	return kind + ":" + ns + "." + name
}

// describeKey renders an object key for an error message: table "notes".
func describeKey(key string) string {
	kind, name, _ := strings.Cut(key, ":")
	return fmt.Sprintf("%s %q", kind, name)
}

// objectsEqual reports whether two single-object schemas are the same object,
// by schema.Equal (spelling of types and expressions does not count). A missing
// object equals only a missing object.
func objectsEqual(a, b *schema.Schema) bool {
	if a == nil || b == nil {
		return a == b
	}
	return schema.Equal(a, b)
}

// changedKeys returns the keys whose object differs between a and b: added,
// removed or changed.
func changedKeys(a, b map[string]*schema.Schema) map[string]bool {
	out := map[string]bool{}
	for k, o := range a {
		if !objectsEqual(o, b[k]) {
			out[k] = true
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			out[k] = true
		}
	}
	return out
}

// assemble builds one schema from single-object schemas, sorted canonically.
func assemble(d *schema.Schema, objs map[string]*schema.Schema) (*schema.Schema, error) {
	out := &schema.Schema{FormatVersion: schema.FormatVersion, Dialect: d.Dialect}
	for _, k := range slices.Sorted(maps.Keys(objs)) {
		o := objs[k]
		out.Schemas = append(out.Schemas, o.Schemas...)
		out.Extensions = append(out.Extensions, o.Extensions...)
		out.Enums = append(out.Enums, o.Enums...)
		out.Tables = append(out.Tables, o.Tables...)
		out.Views = append(out.Views, o.Views...)
		out.Functions = append(out.Functions, o.Functions...)
		out.Triggers = append(out.Triggers, o.Triggers...)
		out.Policies = append(out.Policies, o.Policies...)
	}
	if out.Tables == nil {
		out.Tables = []schema.Table{}
	}
	// A deep copy, so Sort cannot reorder the slices of the snapshots it came from.
	cp, err := cloneSchema(out)
	if err != nil {
		return nil, err
	}
	cp.Sort()
	return cp, nil
}

// danglingForeignKeys lists "table.fk" for every foreign key of s whose
// referenced table is not in s.
func danglingForeignKeys(s *schema.Schema) map[string]bool {
	have := map[string]bool{}
	for _, t := range s.Tables {
		have[t.Name] = true
		if t.Schema != "" {
			have[t.Schema+"."+t.Name] = true
		}
	}
	out := map[string]bool{}
	for _, t := range s.Tables {
		for _, fk := range t.ForeignKeys {
			if !have[fk.RefTable] {
				out[t.Name+"."+fk.Name] = true
			}
		}
	}
	return out
}

// foldChain derives the schema after every entry of entries, which must be
// ordered by version and structurally valid (one root first, every parent
// present and older). It returns the derived schemas, index for index, and one
// error per conflict, each wrapping ErrBranchConflict; the schemas are only
// meaningful when there are no errors.
func foldChain(entries []chainEntry) ([]*schema.Schema, []error) {
	idx := make(map[string]int, len(entries))
	for i, e := range entries {
		idx[e.snap.Version] = i
	}
	states := make([]*schema.Schema, len(entries))
	exploded := make([]map[string]*schema.Schema, len(entries))
	objsOf := func(i int) map[string]*schema.Schema {
		if exploded[i] == nil {
			exploded[i] = splitObjects(states[i])
		}
		return exploded[i]
	}
	touched := make([]map[string]bool, len(entries))
	touchedBy := func(i int) map[string]bool {
		if touched[i] != nil {
			return touched[i]
		}
		base := map[string]*schema.Schema{}
		if p := entries[i].snap.ParentVersion; p != "" {
			base = objsOf(idx[p])
		}
		touched[i] = changedKeys(base, splitObjects(entries[i].snap.Schema))
		return touched[i]
	}

	var problems []error
	for i, e := range entries {
		if i == 0 || idx[e.snap.ParentVersion] == i-1 {
			// The migration was generated from the one before it: its snapshot is the state.
			states[i] = e.snap.Schema
			continue
		}
		p := idx[e.snap.ParentVersion]
		mine := splitObjects(e.snap.Schema)
		theirs := objsOf(i - 1)
		var conflicts []error
		for _, k := range slices.Sorted(maps.Keys(touchedBy(i))) {
			if objectsEqual(objsOf(p)[k], theirs[k]) {
				continue // nothing changed it since this migration's base
			}
			if objectsEqual(mine[k], theirs[k]) {
				continue // both sides made the same change
			}
			other := i - 1
			for j := p + 1; j < i; j++ {
				if touchedBy(j)[k] {
					other = j
					break
				}
			}
			conflicts = append(conflicts, fmt.Errorf("%w: %s and %s both change %s (parents %s and %s); rebase one branch with momo migrate rebase (see docs/migrations.md)",
				ErrBranchConflict, entries[other].path, e.path, describeKey(k), entries[other].snap.Version, e.snap.ParentVersion))
		}
		if len(conflicts) > 0 {
			problems = append(problems, conflicts...)
			states[i] = states[i-1]
			continue
		}
		merged := maps.Clone(theirs)
		for k := range touchedBy(i) {
			if o, ok := mine[k]; ok {
				merged[k] = o
			} else {
				delete(merged, k)
			}
		}
		s, err := assemble(e.snap.Schema, merged)
		if err != nil {
			problems = append(problems, fmt.Errorf("migrate: %s: %w", e.path, err))
			states[i] = states[i-1]
			continue
		}
		own := danglingForeignKeys(e.snap.Schema)
		for _, ref := range slices.Sorted(maps.Keys(danglingForeignKeys(s))) {
			if !own[ref] {
				problems = append(problems, fmt.Errorf("%w: %s: merged with the migrations between it and its parent %s, foreign key %s refers to a table that no longer exists", ErrBranchConflict, e.path, e.snap.ParentVersion, ref))
			}
		}
		states[i] = s
	}
	return states, problems
}

// snapshotEntries reads the snapshots of fsys in version order, only those at
// or before upTo when upTo is not empty.
func snapshotEntries(fsys fs.FS, upTo string) ([]chainEntry, error) {
	files, err := Discover(fsys)
	if err != nil {
		return nil, err
	}
	var out []chainEntry
	for _, f := range files {
		if f.Kind != "snapshot" || (upTo != "" && f.Version > upTo) {
			continue
		}
		s, err := ReadSnapshot(strings.NewReader(string(f.Body)))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		out = append(out, chainEntry{path: f.Path, snap: *s})
	}
	return out, nil
}

// deriveState returns the schema after the last of entries (nil for none). A
// chain in which every snapshot is generated from the one before it is the
// stored last snapshot, unchanged. A chain with forks is folded, and a
// conflict is an error. A chain that is not even structurally valid (a second
// root, a missing or newer parent) has no meaningful fold, so the stored last
// snapshot is returned as before and validate and apply report the problem.
func deriveState(entries []chainEntry) (*schema.Schema, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	last := entries[len(entries)-1].snap.Schema
	linear, shaped := true, true
	have := map[string]bool{}
	for i, e := range entries {
		want := ""
		if i > 0 {
			want = entries[i-1].snap.Version
		}
		if e.snap.ParentVersion != want {
			linear = false
		}
		if p := e.snap.ParentVersion; (p == "") != (i == 0) || (p != "" && (!have[p] || p >= e.snap.Version)) {
			shaped = false
		}
		have[e.snap.Version] = true
	}
	if linear || !shaped {
		return last, nil
	}
	states, problems := foldChain(entries)
	if len(problems) > 0 {
		return nil, errors.Join(problems...)
	}
	return states[len(states)-1], nil
}

// LatestState returns the latest snapshot of fsys with its schema derived from
// the whole chain, which is what the next migration builds on. For a linear
// chain it is LatestSnapshot. After two branches merged it holds the work of
// both, which neither branch's own snapshot does. It returns ErrNoSnapshot when
// there is none and an error wrapping ErrBranchConflict when two branches
// changed the same object.
func LatestState(fsys fs.FS) (*SnapshotFile, error) {
	latest, err := LatestSnapshot(fsys)
	if err != nil {
		return nil, err
	}
	entries, err := snapshotEntries(fsys, "")
	if err != nil {
		return nil, err
	}
	st, err := deriveState(entries)
	if err != nil {
		return nil, err
	}
	out := *latest
	out.Schema = st
	return &out, nil
}

// stateUpTo is LatestState for the migrations at or before version: the schema after
// the greatest snapshot not after it, or nil when there is none. A migration
// without a snapshot of its own leaves the state of the one before it.
func stateUpTo(fsys fs.FS, version string) (*SnapshotFile, error) {
	snap, err := snapshotUpTo(fsys, version)
	if err != nil || snap == nil {
		return snap, err
	}
	entries, err := snapshotEntries(fsys, snap.Version)
	if err != nil {
		return nil, err
	}
	st, err := deriveState(entries)
	if err != nil {
		return nil, err
	}
	out := *snap
	out.Schema = st
	return &out, nil
}
