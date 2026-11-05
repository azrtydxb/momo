// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/schema"
)

// invalidIndexDetail is the Detail of an index PostgreSQL marks invalid.
const invalidIndexDetail = "invalid index (failed CONCURRENTLY build)"

// excludeMatcher validates the glob patterns of Config.Exclude and returns a
// function reporting whether any pattern matches name or schema.name.
func excludeMatcher(globs []string) (func(schemaName, name string) bool, error) {
	for _, g := range globs {
		if _, err := path.Match(g, ""); err != nil {
			return nil, fmt.Errorf("migrate: Config.Exclude pattern %q: %w", g, err)
		}
	}
	return func(ns, name string) bool {
		for _, g := range globs {
			if ok, _ := path.Match(g, name); ok {
				return true
			}
			if ns != "" {
				if ok, _ := path.Match(g, ns+"."+name); ok {
					return true
				}
			}
		}
		return false
	}, nil
}

// applyExclude returns a copy of s without the tables, views, functions, enums
// and extensions whose name matches a pattern of globs, and without the
// triggers and policies of an excluded table or whose own name matches. s is
// not modified. Patterns were validated by excludeMatcher.
func applyExclude(s *schema.Schema, globs []string) *schema.Schema {
	if len(globs) == 0 {
		return s
	}
	match, err := excludeMatcher(globs)
	if err != nil {
		return s
	}
	c := *s
	gone := map[string]bool{}
	c.Tables = slices.DeleteFunc(slices.Clone(s.Tables), func(t schema.Table) bool {
		if match(t.Schema, t.Name) {
			gone[t.Schema+"\x00"+t.Name] = true
			return true
		}
		return false
	})
	c.Views = slices.DeleteFunc(slices.Clone(s.Views), func(v schema.View) bool { return match(v.Schema, v.Name) })
	c.Functions = slices.DeleteFunc(slices.Clone(s.Functions), func(f schema.Function) bool { return match(f.Schema, f.Name) })
	c.Enums = slices.DeleteFunc(slices.Clone(s.Enums), func(e schema.Enum) bool { return match("", e.Name) })
	c.Extensions = slices.DeleteFunc(slices.Clone(s.Extensions), func(e schema.Extension) bool { return match("", e.Name) })
	c.Triggers = slices.DeleteFunc(slices.Clone(s.Triggers), func(t schema.Trigger) bool {
		return gone[t.Schema+"\x00"+t.Table] || match(t.Schema, t.Name)
	})
	c.Policies = slices.DeleteFunc(slices.Clone(s.Policies), func(p schema.Policy) bool {
		return gone[p.Schema+"\x00"+p.Table] || match(p.Schema, p.Name)
	})
	return &c
}

// object is one comparable database object: its identity, its normalized JSON
// and the table or view it belongs to.
type object struct {
	ref  ObjectRef
	body string
	// parent is the key of the table or view the object is part of, "" for a
	// top-level object. A child is skipped when its parent exists on one side
	// only, because the parent already reports the difference.
	parent string
	// child objects (columns, indexes, constraints) of a managed table that
	// exist on one side only are reported as changes of that table; triggers and
	// policies have a parent but are reported like top-level objects.
	child bool
}

func key(kind, ns, name string) string { return kind + "\x00" + ns + "\x00" + name }

func (o object) key() string { return key(o.ref.Kind, o.ref.Schema, o.ref.Name) }

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		// Every value passed here is built from the schema types, which always
		// encode; reaching this is a programming error.
		panic(fmt.Sprintf("migrate: encode %T: %v", v, err))
	}
	return string(b)
}

// flatten turns s into its comparable objects, keyed by identity. s is
// normalized first (Equal's rules: spelling of types and expressions, order).
// With exact false the text PostgreSQL rewrites on its own (view queries,
// policy and trigger expressions) is left out, so only the identity of those
// objects counts.
func flatten(s *schema.Schema, exact bool) map[string]object {
	n := schema.Normalized(s)
	out := map[string]object{}
	add := func(o object) { out[o.key()] = o }
	for _, x := range n.Schemas {
		add(object{ref: ObjectRef{Kind: "schema", Name: x.Name}, body: mustJSON(x)})
	}
	for _, x := range n.Extensions {
		add(object{ref: ObjectRef{Kind: "extension", Name: x.Name}, body: mustJSON(x)})
	}
	for _, x := range n.Enums {
		add(object{ref: ObjectRef{Kind: "enum", Name: x.Name}, body: mustJSON(x)})
	}
	indexes := func(parent object, ns, owner string, ixs []schema.Index) {
		for _, ix := range ixs {
			add(object{ref: ObjectRef{Kind: "index", Schema: ns, Name: owner + "." + ix.Name}, body: mustJSON(ix), parent: parent.key(), child: true})
		}
	}
	for _, t := range n.Tables {
		cols, uqs, ixs, fks, chs := t.Columns, t.Uniques, t.Indexes, t.ForeignKeys, t.Checks
		t.Columns, t.Uniques, t.Indexes, t.ForeignKeys, t.Checks = nil, nil, nil, nil, nil
		tbl := object{ref: ObjectRef{Kind: "table", Schema: t.Schema, Name: t.Name}, body: mustJSON(t)}
		add(tbl)
		for _, c := range cols {
			add(object{ref: ObjectRef{Kind: "column", Schema: t.Schema, Name: t.Name + "." + c.Name}, body: mustJSON(c), parent: tbl.key(), child: true})
		}
		for _, u := range uqs {
			add(object{ref: ObjectRef{Kind: "unique", Schema: t.Schema, Name: t.Name + "." + u.Name}, body: mustJSON(u), parent: tbl.key(), child: true})
		}
		indexes(tbl, t.Schema, t.Name, ixs)
		for _, f := range fks {
			add(object{ref: ObjectRef{Kind: "fk", Schema: t.Schema, Name: t.Name + "." + f.Name}, body: mustJSON(f), parent: tbl.key(), child: true})
		}
		for _, c := range chs {
			add(object{ref: ObjectRef{Kind: "check", Schema: t.Schema, Name: t.Name + "." + c.Name}, body: mustJSON(c), parent: tbl.key(), child: true})
		}
	}
	for _, v := range n.Views {
		ixs := v.Indexes
		v.Indexes = nil
		if !exact {
			v.Query = ""
		}
		vw := object{ref: ObjectRef{Kind: "view", Schema: v.Schema, Name: v.Name}, body: mustJSON(v)}
		add(vw)
		indexes(vw, v.Schema, v.Name, ixs)
	}
	for _, f := range n.Functions {
		add(object{ref: ObjectRef{Kind: "function", Schema: f.Schema, Name: f.Name + "(" + f.Args + ")"}, body: mustJSON(f)})
	}
	parentOf := func(ns, table string) string {
		for _, kind := range []string{"table", "view"} {
			if _, ok := out[key(kind, ns, table)]; ok {
				return key(kind, ns, table)
			}
		}
		return ""
	}
	for _, t := range n.Triggers {
		if !exact {
			t.When = ""
		}
		add(object{ref: ObjectRef{Kind: "trigger", Schema: t.Schema, Name: t.Table + "." + t.Name}, body: mustJSON(t), parent: parentOf(t.Schema, t.Table)})
	}
	for _, p := range n.Policies {
		if !exact {
			p.Using, p.Check = "", ""
		}
		add(object{ref: ObjectRef{Kind: "policy", Schema: p.Schema, Name: p.Table + "." + p.Name}, body: mustJSON(p), parent: parentOf(p.Schema, p.Table)})
	}
	return out
}

// differingFields names the top-level JSON fields in which a and b differ.
func differingFields(a, b string) string {
	var x, y map[string]json.RawMessage
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return "definition differs"
	}
	var names []string
	for k, v := range x {
		if w, ok := y[k]; !ok || string(v) != string(w) {
			names = append(names, k)
		}
	}
	for k := range y {
		if _, ok := x[k]; !ok {
			names = append(names, k)
		}
	}
	slices.Sort(names)
	return strings.Join(names, ", ") + " differ"
}

// reconcileExtensions blanks the version of an extension on both sides when
// either side leaves it empty: an empty version means whatever the server
// installs (the rule schema.Equal applies). The schemas are modified, so pass
// copies.
func reconcileExtensions(a, b *schema.Schema) {
	for i := range a.Extensions {
		for j := range b.Extensions {
			x, y := &a.Extensions[i], &b.Extensions[j]
			if x.Name == y.Name && (x.Version == "" || y.Version == "") {
				x.Version, y.Version = "", ""
			}
		}
	}
}

// compare reports how live differs from snapshot. Top-level objects only in the
// snapshot are Missing, only in the live database Unmanaged. A column, index
// or constraint of a table present on both sides that exists on one side only
// is a change of that table's definition: a Changed entry when it exists only
// live ("not in snapshot"), a Missing entry when only the snapshot has it.
// Objects of a table that is itself missing or unmanaged are not listed again.
func compare(snapshot, live *schema.Schema, exact bool, invalid []string) *DriftReport {
	a, b := *snapshot, *live
	a.Extensions, b.Extensions = slices.Clone(snapshot.Extensions), slices.Clone(live.Extensions)
	reconcileExtensions(&a, &b)
	from, to := flatten(&a, exact), flatten(&b, exact)
	rep := &DriftReport{}
	changed := map[string]*ObjectDiff{}
	setChanged := func(ref ObjectRef, detail string) {
		k := key(ref.Kind, ref.Schema, ref.Name)
		if d, ok := changed[k]; ok {
			d.Detail += "; " + detail
			return
		}
		changed[k] = &ObjectDiff{Object: ref, Detail: detail}
	}
	both := func(parent string) bool {
		if parent == "" {
			return true
		}
		_, inFrom := from[parent]
		_, inTo := to[parent]
		return inFrom && inTo
	}
	for k, o := range from {
		l, ok := to[k]
		switch {
		case !ok:
			if both(o.parent) {
				rep.Missing = append(rep.Missing, o.ref)
			}
		case o.body != l.body:
			setChanged(o.ref, differingFields(o.body, l.body))
		}
	}
	for k, o := range to {
		if _, ok := from[k]; ok || !both(o.parent) {
			continue
		}
		if o.child {
			setChanged(o.ref, "not in snapshot")
		} else {
			rep.Unmanaged = append(rep.Unmanaged, o.ref)
		}
	}
	for _, name := range invalid {
		ref := invalidIndexRef(live, name)
		if o, ok := to[key(ref.Kind, ref.Schema, ref.Name)]; ok && !both(o.parent) {
			continue
		}
		setChanged(ref, invalidIndexDetail)
	}
	for _, d := range changed {
		rep.Changed = append(rep.Changed, *d)
	}
	less := func(x, y ObjectRef) int {
		return strings.Compare(key(x.Kind, x.Schema, x.Name), key(y.Kind, y.Schema, y.Name))
	}
	slices.SortFunc(rep.Changed, func(x, y ObjectDiff) int { return less(x.Object, y.Object) })
	slices.SortFunc(rep.Missing, less)
	slices.SortFunc(rep.Unmanaged, less)
	return rep
}

// invalidIndexRef finds the index the dialect named in live (bare name in the
// current schema, "schema.index" elsewhere) and spells it as drift identifies
// indexes: table.index.
func invalidIndexRef(live *schema.Schema, name string) ObjectRef {
	ns, bare := "", name
	if i := strings.IndexByte(name, '.'); i >= 0 {
		ns, bare = name[:i], name[i+1:]
	}
	for _, t := range live.Tables {
		if t.Schema == ns {
			for _, ix := range t.Indexes {
				if ix.Name == bare {
					return ObjectRef{Kind: "index", Schema: ns, Name: t.Name + "." + bare}
				}
			}
		}
	}
	for _, v := range live.Views {
		if v.Schema == ns {
			for _, ix := range v.Indexes {
				if ix.Name == bare {
					return ObjectRef{Kind: "index", Schema: ns, Name: v.Name + "." + bare}
				}
			}
		}
	}
	return ObjectRef{Kind: "index", Schema: ns, Name: bare}
}

// kinds lists the distinct object kinds a report mentions, sorted.
func (d *DriftReport) kinds() []string {
	set := map[string]bool{}
	for _, c := range d.Changed {
		set[c.Object.Kind] = true
	}
	for _, r := range d.Missing {
		set[r.Kind] = true
	}
	for _, r := range d.Unmanaged {
		set[r.Kind] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// snapshotAt returns the snapshot recorded for exactly version, or
// ErrNoSnapshot.
func snapshotAt(dir fs.FS, version string) (*SnapshotFile, error) {
	files, err := Discover(dir)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.Kind != "snapshot" || f.Version != version {
			continue
		}
		s, err := ReadSnapshot(strings.NewReader(string(f.Body)))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		return s, nil
	}
	return nil, fmt.Errorf("%w: version %s", ErrNoSnapshot, version)
}

// snapshotUpTo returns the snapshot with the greatest version not after
// version, or nil when there is none. A migration without a snapshot of its own
// (custom SQL) leaves the state of the one before it.
func snapshotUpTo(dir fs.FS, version string) (*SnapshotFile, error) {
	files, err := Discover(dir)
	if err != nil {
		return nil, err
	}
	best := ""
	for _, f := range files {
		if f.Kind == "snapshot" && f.Version <= version && f.Version > best {
			best = f.Version
		}
	}
	if best == "" {
		return nil, nil
	}
	return snapshotAt(dir, best)
}

// Drift compares the live database with the snapshot of the latest applied
// migration (the empty schema when none is applied, so every live object is
// unmanaged) and reports what differs. See DriftReport for the three lists.
//
// It is read-only: it takes no lock, leaves nothing behind (a database without
// a history table has an empty history) and changes no application object. A
// dialect that implements Canonicalizer is asked to rewrite the snapshot's
// table text (check expressions, column defaults, index predicates) to the form
// the database reports, which on PostgreSQL creates a scratch schema in a
// transaction that is rolled back; a role that cannot create one gets the
// declared text compared as it is. Because PostgreSQL
// rewrites the text of views and of policy and trigger expressions, drift
// compares those objects by identity and definition apart from that text; a
// changed view query, trigger condition or policy expression (a USING or WITH
// CHECK of a row-level security policy) made outside migrations is NOT
// detected. DriftReport.Unverified names those objects so a clean report is not
// read as proof that their bodies match; Clean ignores it. Function bodies,
// which PostgreSQL stores verbatim, are compared. The history table is left out
// and Config.Exclude patterns hide matching objects on both sides. An index
// PostgreSQL marks invalid (a failed CREATE INDEX CONCURRENTLY) is reported as
// Changed with Detail "invalid index (failed CONCURRENTLY build)".
func (e *Engine) Drift(ctx context.Context) (*DriftReport, error) {
	c, err := e.pin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Release() }()
	hist, err := e.readHistory(ctx, c)
	if err != nil {
		return nil, err
	}
	latest := ""
	for _, r := range hist {
		if r.Status == StatusApplied && r.Version > latest {
			latest = r.Version
		}
	}
	live, invalid, err := e.inspectOn(ctx, c)
	if err != nil {
		return nil, err
	}
	from := &schema.Schema{Dialect: live.Dialect}
	if latest != "" {
		snap, err := snapshotUpTo(e.dir, latest)
		if err != nil {
			return nil, err
		}
		if snap == nil {
			return nil, fmt.Errorf("%w: applied version %s has no snapshot at or before it; run momo migrate diff", ErrNoSnapshot, latest)
		}
		from = applyExclude(snap.Schema, e.opts.config.Exclude)
		if from, err = e.canonicalSnapshot(ctx, c, from); err != nil {
			return nil, err
		}
	}
	rep := compare(from, live, false, invalid)
	rep.Unverified = unverifiedBodies(from, live)
	return rep, nil
}

// canonicalSnapshot returns a copy of the snapshot schema s with the declared
// table text (check expressions, defaults, index predicates) rewritten to the
// form the database reports, for a dialect that implements Canonicalizer. The
// snapshot holds the text as the model declared it, while inspection reports
// what PostgreSQL stored, so comparing them raw reports a phantom difference
// for a migration that was just applied. A role without the privilege the
// rewrite needs (ErrCanonicalizePrivilege) gets s unchanged: drift detection
// still works for it, but may then report such text as changed.
func (e *Engine) canonicalSnapshot(ctx context.Context, c momo.PinnedConn, s *schema.Schema) (*schema.Schema, error) {
	cz, ok := e.opts.dialect.(Canonicalizer)
	if !ok {
		return s, nil
	}
	want, err := cloneSchema(s)
	if err != nil {
		return nil, err
	}
	// Only table text is rewritten: views, functions, triggers and policies
	// stay as declared, so a view that selects from an excluded table cannot
	// make the scratch round trip fail and drift keeps its documented rules for
	// them.
	views, funcs, triggers, policies := want.Views, want.Functions, want.Triggers, want.Policies
	want.Views, want.Functions, want.Triggers, want.Policies = nil, nil, nil, nil
	err = cz.Canonicalize(ctx, c, want)
	want.Views, want.Functions, want.Triggers, want.Policies = views, funcs, triggers, policies
	if err != nil {
		if errors.Is(err, ErrCanonicalizePrivilege) {
			return s, nil
		}
		return nil, fmt.Errorf("migrate: canonicalize snapshot: %w", err)
	}
	return want, nil
}

// unverifiedBodies lists the views, triggers and policies that exist in both
// schemas and carry text that a non-exact comparison leaves out.
func unverifiedBodies(from, live *schema.Schema) []ObjectRef {
	var out []ObjectRef
	type id struct{ kind, ns, name string }
	in := map[id]bool{}
	for _, v := range live.Views {
		in[id{"view", v.Schema, v.Name}] = true
	}
	for _, t := range live.Triggers {
		in[id{"trigger", t.Schema, t.Table + "." + t.Name}] = true
	}
	for _, p := range live.Policies {
		in[id{"policy", p.Schema, p.Table + "." + p.Name}] = true
	}
	add := func(kind, ns, name string, hasText bool) {
		if hasText && in[id{kind, ns, name}] {
			out = append(out, ObjectRef{Kind: kind, Schema: ns, Name: name})
		}
	}
	for _, v := range from.Views {
		add("view", v.Schema, v.Name, v.Query != "")
	}
	for _, t := range from.Triggers {
		add("trigger", t.Schema, t.Table+"."+t.Name, t.When != "")
	}
	for _, p := range from.Policies {
		add("policy", p.Schema, p.Table+"."+p.Name, p.Using != "" || p.Check != "")
	}
	slices.SortFunc(out, func(x, y ObjectRef) int {
		return strings.Compare(key(x.Kind, x.Schema, x.Name), key(y.Kind, y.Schema, y.Name))
	})
	return out
}
