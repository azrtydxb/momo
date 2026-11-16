// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/azrtydxb/momo/schema"
)

// A snapshot file records the schema its migration was generated to, and its
// parent is the migration it was generated from. Two branches that each add a
// migration therefore write snapshots that do not contain each other's work.
// The schema after a migration is derived instead of read: the migrations are
// folded in version order, and each one contributes only what it changed
// relative to its own base (a three-way merge). A chain without a fork folds to
// exactly the stored snapshots.
//
// The base of a migration is its parent's stored snapshot, because that is the
// schema it was generated from; a snapshot marked SnapshotFile.Base =
// "derived:<sha256>" (written by a diff after a merge) was generated from the
// parent's derived schema, so that is its base; the hash fingerprints that
// state, and a migration whose parent state has since changed (an older branch
// merged late) is a conflict, to be regenerated.
//
// The unit of change is one object: a table with its triggers and policies, a
// view, an enum, an extension, a function or a namespace. When independence of
// two migrations cannot be shown, the fold reports an ErrBranchConflict naming
// both. Independent means all of these hold, checked against the merged schema:
//
//   - they change different objects (two identical changes conflict too: both
//     SQL files would run, and the second would fail);
//   - a view that one changes does not read a table the other changes, and when
//     the sources of a changed view are unknown (no Depends), the other side
//     changes no table;
//   - when one changes a function, trigger or policy, whose body is opaque, the
//     other changes no table or view;
//   - every foreign key of the merged schema that was valid on either side is
//     still valid: its table exists, the referenced columns exist, have the
//     types of the referencing columns and are backed by a primary key, a unique
//     constraint or a unique index.

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

// objectCount returns the number of objects in s: the length of every list
// field, found by reflection so that a kind added to schema.Schema is counted
// without anyone remembering to.
func objectCount(s *schema.Schema) int {
	v := reflect.ValueOf(s).Elem()
	n := 0
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() == reflect.Slice {
			n += v.Field(i).Len()
		}
	}
	return n
}

// countObjs is objectCount summed over single-object schemas.
func countObjs(objs map[string]*schema.Schema) int {
	n := 0
	for _, o := range objs {
		n += objectCount(o)
	}
	return n
}

// fingerprintVersion names the canonical encoding stateFingerprint hashes. A
// marker records it, so a later encoding gets a new version instead of
// silently invalidating every marker written before.
const fingerprintVersion = "v1"

// stateFingerprint returns the hash of the objects of s under encoding v1: for
// each object in key order, the key, a newline, the bytes the object would have
// as a format-1 snapshot schema (indented JSON, format_version 1) and a
// newline. The encoding is pinned by a test against a literal hash: changing
// what goes into it (a schema field, its JSON tags) changes the hash, and the
// change needs a new fingerprintVersion, not an edit of this one.
func stateFingerprint(s *schema.Schema) string {
	h := sha256.New()
	objs := splitObjects(s)
	for _, k := range slices.Sorted(maps.Keys(objs)) {
		c := *objs[k]
		c.FormatVersion = 1
		b, _ := json.MarshalIndent(&c, "", "  ")
		h.Write([]byte(k + "\n"))
		h.Write(b)
		h.Write([]byte("\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
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

// keyKind returns the kind of an object key: "table", "view", "function"...
func keyKind(key string) string {
	kind, _, _ := strings.Cut(key, ":")
	return kind
}

// objectsEqual reports whether two single-object schemas are the same object,
// by schema.Equal (spelling of types and expressions does not count). A missing
// object equals only a missing object.
func objectsEqual(a, b *schema.Schema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if x, err := json.Marshal(a); err == nil {
		if y, err := json.Marshal(b); err == nil && bytes.Equal(x, y) {
			return true
		}
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

// hasKind reports whether keys holds a key of one of the kinds.
func hasKind(keys map[string]bool, kinds ...string) bool {
	for k := range keys {
		if slices.Contains(kinds, keyKind(k)) {
			return true
		}
	}
	return false
}

// impliedKeys returns keys plus the tables and views that each changed view
// reads (its Depends, from any of srcs), and whether a changed view has no
// known sources.
func impliedKeys(keys map[string]bool, srcs ...map[string]*schema.Schema) (out map[string]bool, unknown bool) {
	out = maps.Clone(keys)
	for k := range keys {
		if keyKind(k) != "view" {
			continue
		}
		known := false
		for _, src := range srcs {
			if o := src[k]; o != nil && len(o.Views) == 1 {
				for _, d := range o.Views[0].Depends {
					ns, name := "", d
					if a, b, ok := strings.Cut(d, "."); ok {
						ns, name = a, b
					}
					out[objectKey("table", ns, name)] = true
					out[objectKey("view", ns, name)] = true
					known = true
				}
			}
		}
		if !known {
			unknown = true
		}
	}
	return out, unknown
}

// removesShared reports whether keys remove or alter an enum, an extension or
// a schema namespace that already existed in base: tables, views and functions
// may use any of them. Adding one is safe.
func removesShared(keys map[string]bool, base map[string]*schema.Schema) bool {
	for k := range keys {
		if slices.Contains([]string{"enum", "extension", "schema"}, keyKind(k)) && base[k] != nil {
			return true
		}
	}
	return false
}

// opaqueChanged reports whether the change from before to after of the keys
// touches an object whose body can refer to any table: a function, a trigger
// or a policy.
func opaqueChanged(keys map[string]bool, before, after map[string]*schema.Schema) bool {
	body := func(o *schema.Schema) string {
		if o == nil || len(o.Triggers)+len(o.Policies) == 0 {
			return ""
		}
		b, _ := json.Marshal([]any{o.Triggers, o.Policies})
		return string(b)
	}
	for k := range keys {
		switch keyKind(k) {
		case "function", "trigger", "policy":
			return true
		case "table":
			if body(before[k]) != body(after[k]) {
				return true
			}
		}
	}
	return false
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

func sameSet(a, b []string) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(slices.Compact(x), slices.Compact(y))
}

// invalidForeignKeys lists "table.fk" for every foreign key of s that cannot
// hold: the referenced table or a column is missing, a column type differs
// from the referencing one, or the referenced columns are not a primary key, a
// unique constraint or a unique index.
func invalidForeignKeys(s *schema.Schema) map[string]bool {
	tables := map[string]*schema.Table{}
	for i := range s.Tables {
		t := &s.Tables[i]
		tables[t.Name] = t
		if t.Schema != "" {
			tables[t.Schema+"."+t.Name] = t
		}
	}
	typeOf := func(t *schema.Table, col string) (string, bool) {
		c, ok := t.Column(col)
		if !ok {
			return "", false
		}
		return schema.NormalizeType(s.Dialect, c.Type), true
	}
	out := map[string]bool{}
	for i := range s.Tables {
		t := &s.Tables[i]
		for _, fk := range t.ForeignKeys {
			r := tables[fk.RefTable]
			refCols := fk.RefColumns
			if r != nil && len(refCols) == 0 {
				refCols = r.PrimaryKey
			}
			ok := r != nil && len(refCols) == len(fk.Columns)
			if ok {
				for j, c := range fk.Columns {
					lt, lok := typeOf(t, c)
					rt, rok := typeOf(r, refCols[j])
					ok = ok && lok && rok && lt == rt
				}
			}
			if ok {
				backed := sameSet(refCols, r.PrimaryKey)
				for _, u := range r.Uniques {
					backed = backed || sameSet(refCols, u.Columns)
				}
				for _, ix := range r.Indexes {
					if !ix.Unique || ix.Where != "" {
						continue
					}
					cols := make([]string, 0, len(ix.Parts))
					for _, p := range ix.Parts {
						cols = append(cols, p.Column)
					}
					backed = backed || sameSet(refCols, cols)
				}
				ok = backed
			}
			if !ok {
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
	stored := make([]map[string]*schema.Schema, len(entries))
	derived := make([]map[string]*schema.Schema, len(entries))
	storedObjs := func(i int) map[string]*schema.Schema {
		if stored[i] == nil {
			stored[i] = splitObjects(entries[i].snap.Schema)
		}
		return stored[i]
	}
	stateObjs := func(i int) map[string]*schema.Schema {
		if derived[i] == nil {
			derived[i] = splitObjects(states[i])
		}
		return derived[i]
	}
	// baseObjs is what entry i was generated from.
	baseObjs := func(i int) map[string]*schema.Schema {
		p := entries[i].snap.ParentVersion
		switch {
		case p == "":
			return map[string]*schema.Schema{}
		case baseIsDerived(entries[i].snap.Base):
			return stateObjs(idx[p])
		default:
			return storedObjs(idx[p])
		}
	}
	touched := make([]map[string]bool, len(entries))
	touchedBy := func(i int) map[string]bool {
		if touched[i] == nil {
			touched[i] = changedKeys(baseObjs(i), storedObjs(i))
		}
		return touched[i]
	}
	// other finds the latest earlier migration that changed a key matching pred.
	other := func(i int, pred func(map[string]bool) bool) int {
		for j := i - 1; j >= 0; j-- {
			if pred(touchedBy(j)) {
				return j
			}
		}
		return i - 1
	}

	var problems []error
	for i, e := range entries {
		if i == 0 {
			states[i] = e.snap.Schema
			continue
		}
		p := idx[e.snap.ParentVersion]
		// A snapshot generated from a derived state is only as good as that
		// state: when a branch merged since, it is stale.
		if baseIsDerived(e.snap.Base) && e.snap.Base != derivedBase(states[p]) {
			// The state changed because a branch older than the parent merged after
			// this migration was written. The hash cannot say which, so name the
			// migrations on other branches older than the parent.
			onChain := map[int]bool{}
			for j := p; ; {
				onChain[j] = true
				pv := entries[j].snap.ParentVersion
				if pv == "" {
					break
				}
				j = idx[pv]
			}
			// Those newer than the parent's own parent come first: a branch merged
			// late most likely sits just before the parent.
			var late []string
			for pass := 0; pass < 2; pass++ {
				for j := 0; j < p; j++ {
					recent := entries[j].snap.Version > entries[p].snap.ParentVersion
					if !onChain[j] && recent == (pass == 0) && len(late) < 5 {
						late = append(late, entries[j].path)
					}
				}
			}
			problems = append(problems, fmt.Errorf("%w: %s was generated from the merged state after %s, which has changed since: a branch with an older version was merged after it was written (possible causes, not on the path to %s: %s); rebase or renumber that branch so it sorts after %s, and if %s is not applied yet, delete it and run momo migrate diff again (see docs/migrations.md)",
				ErrBranchConflict, e.path, e.snap.ParentVersion, e.snap.ParentVersion, strings.Join(late, ", "), e.snap.ParentVersion, e.path))
			states[i] = states[i-1]
			continue
		}
		// Built on the migration just before it, whose stored snapshot is its
		// derived schema: nothing happened in between.
		if p == i-1 && (baseIsDerived(e.snap.Base) || states[p] == entries[p].snap.Schema) {
			states[i] = e.snap.Schema
			continue
		}
		base, mine, running := baseObjs(i), storedObjs(i), stateObjs(i-1)
		mineKeys, runKeys := touchedBy(i), changedKeys(base, running)
		if len(runKeys) == 0 {
			states[i] = e.snap.Schema
			continue
		}
		// Every object must survive the split, or the merge would drop it silently.
		if objectCount(e.snap.Schema) != countObjs(mine) || objectCount(states[i-1]) != countObjs(running) {
			problems = append(problems, fmt.Errorf("%w: %s holds an object of a kind this momo cannot merge, so it cannot be shown independent of the migrations before it", ErrBranchConflict, e.path))
			states[i] = states[i-1]
			continue
		}
		name := func(j int) string { return entries[j].path }
		var conflicts []error
		conflict := func(format string, args ...any) {
			conflicts = append(conflicts, fmt.Errorf("%w: "+format+"; rebase one branch with momo migrate rebase (see docs/migrations.md)",
				append([]any{ErrBranchConflict}, args...)...))
		}
		mineImp, mineUnknown := impliedKeys(mineKeys, mine, base)
		runImp, runUnknown := impliedKeys(runKeys, running, base)
		for _, k := range slices.Sorted(maps.Keys(mineImp)) {
			if runImp[k] {
				j := other(i, func(t map[string]bool) bool { return t[k] })
				conflict("%s and %s both change %s (or one changes a view that reads it)", name(j), e.path, describeKey(k))
			}
		}
		if mineUnknown && hasKind(runKeys, "table") || runUnknown && hasKind(mineKeys, "table") {
			j := other(i, func(t map[string]bool) bool { return len(t) > 0 })
			conflict("%s and %s: one changes a view whose source tables are unknown while the other changes a table", name(j), e.path)
		}
		if opaqueChanged(mineKeys, base, mine) && hasKind(runKeys, "table", "view", "function") ||
			opaqueChanged(runKeys, base, running) && hasKind(mineKeys, "table", "view", "function") {
			j := other(i, func(t map[string]bool) bool { return len(t) > 0 })
			conflict("%s and %s: one changes a function, trigger or policy, whose body can refer to any table, view or function, while the other changes a table, view or function", name(j), e.path)
		}
		if removesShared(mineKeys, base) && hasKind(runKeys, "table", "view", "function") ||
			removesShared(runKeys, base) && hasKind(mineKeys, "table", "view", "function") {
			j := other(i, func(t map[string]bool) bool { return len(t) > 0 })
			conflict("%s and %s: one removes or alters an enum, extension or schema that the tables, views and functions of the other may use", name(j), e.path)
		}
		if len(conflicts) > 0 {
			problems = append(problems, conflicts...)
			states[i] = states[i-1]
			continue
		}
		merged := maps.Clone(running)
		for k := range mineKeys {
			if o, ok := mine[k]; ok {
				merged[k] = o
			} else {
				delete(merged, k)
			}
		}
		s, err := assemble(e.snap.Schema, merged)
		if err == nil && objectCount(s) != countObjs(merged) {
			err = errors.New("the merged schema lost an object")
		}
		if err != nil {
			problems = append(problems, fmt.Errorf("%w: %s: %w", ErrBranchConflict, e.path, err))
			states[i] = states[i-1]
			continue
		}
		badMine, badRun := invalidForeignKeys(e.snap.Schema), invalidForeignKeys(states[i-1])
		for _, ref := range slices.Sorted(maps.Keys(invalidForeignKeys(s))) {
			if !badMine[ref] && !badRun[ref] {
				problems = append(problems, fmt.Errorf("%w: %s: merged with the migrations since its base %s, foreign key %s no longer holds (its table or referenced columns are gone or changed, or no key backs them); rebase one branch with momo migrate rebase (see docs/migrations.md)",
					ErrBranchConflict, e.path, e.snap.ParentVersion, ref))
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

// deriveState returns the schema after the last of entries (nil for none) and
// whether it differs from the stored snapshot of the last entry (a merged
// fork). A chain that is not even structurally valid (a second root, a missing
// or newer parent) has no meaningful fold, so the stored last snapshot is
// returned as before and validate and apply report the problem. A conflict is
// an error.
func deriveState(entries []chainEntry) (*schema.Schema, bool, error) {
	if len(entries) == 0 {
		return nil, false, nil
	}
	last := entries[len(entries)-1].snap.Schema
	have := map[string]bool{}
	for i, e := range entries {
		if p := e.snap.ParentVersion; (p == "") != (i == 0) || (p != "" && (!have[p] || p >= e.snap.Version)) {
			return last, false, nil
		}
		have[e.snap.Version] = true
	}
	states, problems := foldChain(entries)
	if len(problems) > 0 {
		return nil, false, errors.Join(problems...)
	}
	st := states[len(states)-1]
	return st, st != last, nil
}

// LatestState returns the latest snapshot of fsys with its schema derived from
// the whole chain, which is what the next migration builds on. For a chain
// without a fork it is LatestSnapshot. After two branches merged it holds the
// work of both, which neither branch's own snapshot does, and its Base is
// BaseDerived, which a migration generated from it records in its snapshot. It
// returns ErrNoSnapshot when there is none and an error wrapping
// ErrBranchConflict when two branches cannot be shown independent.
func LatestState(fsys fs.FS) (*SnapshotFile, error) {
	latest, err := LatestSnapshot(fsys)
	if err != nil {
		return nil, err
	}
	entries, err := snapshotEntries(fsys, "")
	if err != nil {
		return nil, err
	}
	st, isDerived, err := deriveState(entries)
	if err != nil {
		return nil, err
	}
	out := *latest
	out.Schema, out.Base = st, ""
	if isDerived {
		out.Base = derivedBase(st)
	}
	return &out, nil
}

// stateUpTo is LatestState for the migrations the database has applied, at or
// before version: the schema after the greatest applied snapshot not after it,
// or nil when there is none. A pending migration older than the newest applied
// one (a branch merged late) is not part of the database yet and is left out,
// so drift does not report its objects as missing. A migration without a
// snapshot of its own leaves the state of the one before it. When none of the
// snapshots belongs to an applied version, every snapshot up to version counts.
func stateUpTo(fsys fs.FS, version string, applied map[string]bool) (*SnapshotFile, error) {
	entries, err := snapshotEntries(fsys, version)
	if err != nil {
		return nil, err
	}
	var kept []chainEntry
	for _, e := range entries {
		if applied[e.snap.Version] {
			kept = append(kept, e)
		}
	}
	if len(kept) == 0 {
		snap, err := snapshotUpTo(fsys, version)
		if err != nil || snap == nil {
			return snap, err
		}
		if entries, err = snapshotEntries(fsys, snap.Version); err != nil {
			return nil, err
		}
		kept = entries
	}
	snap := kept[len(kept)-1].snap
	st, _, err := deriveState(kept)
	if err != nil {
		return nil, err
	}
	out := snap
	out.Schema = st
	return &out, nil
}
