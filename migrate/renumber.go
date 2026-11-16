// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

// Renumbered is one migration Renumber moved to a later version.
type Renumbered struct{ Name, Old, New string }

// AppliedVersions returns the versions the history holds, sorted: every row but
// a rolled-back one (a failed or running row counts, so a dirty migration is
// never moved). It is read-only and takes no lock.
func (e *Engine) AppliedVersions(ctx context.Context) ([]string, error) {
	c, err := e.pin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Release() }()
	hist, err := e.readHistory(ctx, c)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range hist {
		if r.Status != StatusRolledBack {
			out = append(out, r.Version)
		}
	}
	slices.Sort(out)
	return out, nil
}

// renumberSnapshots returns every snapshot of all as it will be after the
// versions in newVersion are applied, keyed by its old version: the new
// version, the parents mapped to the new versions, and, for a moved snapshot
// generated from a derived state, the migration just before it as parent and a
// base recomputed against the renumbered chain. Such a snapshot holds the whole
// merged state of everything older than it, so after the move its parent is the
// head it now follows.
func renumberSnapshots(all []File, newVersion map[string]string) (map[string]SnapshotFile, error) {
	var entries []chainEntry
	oldOf := map[string]string{} // new version -> old version
	for _, f := range all {
		if f.Kind != "snapshot" {
			continue
		}
		s, err := ReadSnapshot(strings.NewReader(string(f.Body)))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		nv, moved := newVersion[f.Version]
		if !moved {
			nv = f.Version
		}
		s.Version = nv
		if p, ok := newVersion[s.ParentVersion]; ok {
			s.ParentVersion = p
		}
		entries = append(entries, chainEntry{path: f.Path, snap: *s})
		oldOf[nv] = f.Version
	}
	slices.SortFunc(entries, func(a, b chainEntry) int { return strings.Compare(a.snap.Version, b.snap.Version) })
	for k := range entries {
		if _, moved := newVersion[oldOf[entries[k].snap.Version]]; !moved || k == 0 || !baseIsDerived(entries[k].snap.Base) {
			continue
		}
		entries[k].snap.ParentVersion = entries[k-1].snap.Version
		states, _ := foldChain(entries[:k])
		entries[k].snap.Base = derivedBase(states[k-1])
	}
	out := make(map[string]SnapshotFile, len(entries))
	for _, e := range entries {
		out[oldOf[e.snap.Version]] = e.snap
	}
	return out, nil
}

// Renumber moves the pending migrations that sort before the newest applied
// one to versions after every existing version, so they stop being
// ErrOutOfOrder. applied lists the versions the database has applied; a
// migration is pending when it is not in it. Only pending migrations move: an
// applied one never changes, and a pending migration that builds on a moved one
// moves with it. The relative order of the moved migrations is kept.
//
// Each moved migration gets new file names (up, down and snapshot), its
// snapshot records the new version and the new parent links, and momo.sum is
// updated; the schema in the snapshot and the SQL are untouched. It does not
// touch the database or write files. removed lists, sorted, the paths the
// caller should delete; files holds the new files, including momo.sum, keyed by
// file name, like Migration.Files. When nothing is out of order, both are empty
// and the directory is left as it is.
//
// Renumber refuses, changing nothing, when a moved migration lies in a
// subdirectory, when an applied migration builds on one that would move, or
// when a pending Go migration (registered with WithGoMigrations) sorts before
// the newest applied version, naming it: a Go migration's version is set in
// code, so renumber cannot move it. The command line has no Go migrations, so
// it cannot see them; the engine that has them can. It also refuses when a
// moved snapshot generated from a merged state (a marked base) would sort
// behind a migration that was newer than it and stays, because its schema does
// not include that migration; it names both and says to regenerate it. A
// version moved past a migration of the other branch is checked by validate
// afterwards: two migrations that change the same table are then a conflict.
func (e *Engine) Renumber(applied []string) (removed []string, files map[string][]byte, moved []Renumbered, err error) {
	newest := ""
	done := map[string]bool{}
	for _, v := range applied {
		done[v] = true
		newest = max(newest, v)
	}
	// A Go migration's version lives in the code, not in a file, so it cannot be
	// moved here: name the ones that are out of order and refuse.
	goVersion := map[string]bool{}
	var stuck []string
	for _, g := range e.opts.goMigrations {
		goVersion[g.Version] = true
		if !done[g.Version] && g.Version < newest {
			stuck = append(stuck, g.Version+"_"+g.Name)
		}
	}
	if len(stuck) > 0 {
		slices.Sort(stuck)
		return nil, nil, nil, fmt.Errorf("migrate: pending Go migration(s) %s sort before the newest applied version %s and cannot be renumbered, because their version is set in code; change the Version of each in the code", strings.Join(stuck, ", "), newest)
	}
	all, err := Discover(e.dir)
	if err != nil {
		return nil, nil, nil, err
	}
	parentOf := map[string]string{}
	marked := map[string]bool{} // snapshots generated from a derived (merged) state
	for _, f := range all {
		if f.Kind != "snapshot" {
			continue
		}
		s, err := ReadSnapshot(strings.NewReader(string(f.Body)))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		parentOf[f.Version] = s.ParentVersion
		marked[f.Version] = baseIsDerived(s.Base)
	}
	move := map[string]bool{}
	for _, f := range all {
		if f.Kind == "up" && !done[f.Version] && f.Version < newest {
			move[f.Version] = true
		}
	}
	// A migration built on a moved one moves too, so its parent stays older.
	for grew := true; grew; {
		grew = false
		for v, p := range parentOf {
			if move[p] && !move[v] {
				if done[v] {
					return nil, nil, nil, fmt.Errorf("migrate: version %s is applied but is built on %s, which is pending and would move; renumber cannot change applied migrations", v, p)
				}
				if goVersion[v] {
					return nil, nil, nil, fmt.Errorf("migrate: Go migration %s is built on %s, which would move; renumber cannot change the version of a Go migration", v, p)
				}
				move[v], grew = true, true
			}
		}
		// A snapshot generated from a merged state includes every migration older
		// than itself, so one of those moving past it leaves its state stale: it
		// moves too, behind them, and its base is recomputed.
		for v := range marked {
			if !marked[v] || move[v] {
				continue
			}
			for m := range move {
				if m >= v {
					continue
				}
				if done[v] {
					return nil, nil, nil, fmt.Errorf("migrate: version %s is applied and was generated from a merged state that includes pending version %s, which would move behind it; renumber cannot change an applied migration, so renumbering that branch is not possible here", v, m)
				}
				if goVersion[v] {
					return nil, nil, nil, fmt.Errorf("migrate: Go migration %s was generated from a merged state that includes %s, which would move; renumber cannot change the version of a Go migration", v, m)
				}
				move[v], grew = true, true
				break
			}
		}
	}
	if len(move) == 0 {
		return nil, nil, nil, nil
	}
	// A moved snapshot generated from a merged state holds only what was older
	// than itself. Every migration that stays sorts before it afterwards, so one
	// that was newer would vanish from the derived state: refuse, before writing.
	for _, v := range slices.Sorted(maps.Keys(move)) {
		if !marked[v] {
			continue
		}
		var newer []string
		for u := range parentOf {
			if u > v && !move[u] {
				newer = append(newer, u)
			}
		}
		if len(newer) > 0 {
			slices.Sort(newer)
			return nil, nil, nil, fmt.Errorf("migrate: version %s was generated from a merged state and would have to move behind %s, which are newer than it and stay: its schema does not include them, so renumbering would drop their objects from the derived state; delete %s and run momo migrate diff again to regenerate it", v, strings.Join(newer, ", "), v)
		}
	}
	for _, f := range all {
		if move[f.Version] && strings.Contains(f.Path, "/") {
			return nil, nil, nil, fmt.Errorf("migrate: %s is in a subdirectory; renumber moves files in the top level of the migration directory only", f.Path)
		}
	}
	first, err := NextVersion(e.opts.clock, e.dir)
	if err != nil {
		return nil, nil, nil, err
	}
	t, err := time.Parse(versionLayout, first)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("migrate: renumber: %w", err)
	}
	newVersion := map[string]string{}
	for i, v := range slices.Sorted(maps.Keys(move)) {
		newVersion[v] = t.Add(time.Duration(i) * time.Second).Format(versionLayout)
	}

	snaps, err := renumberSnapshots(all, newVersion)
	if err != nil {
		return nil, nil, nil, err
	}
	added := memFS{}
	var addedNames []string
	out := map[string][]byte{}
	for _, f := range all {
		nv, ok := newVersion[f.Version]
		if !ok {
			continue
		}
		removed = append(removed, f.Path)
		name := nv + strings.TrimPrefix(f.Path, f.Version)
		body := f.Body
		if f.Kind == "snapshot" {
			var buf bytes.Buffer
			if err := WriteSnapshot(&buf, snaps[f.Version]); err != nil {
				return nil, nil, nil, err
			}
			body = buf.Bytes()
		}
		added[name], out[name] = body, body
		addedNames = append(addedNames, name)
		if f.Kind == "up" {
			moved = append(moved, Renumbered{Name: f.Name, Old: f.Version, New: nv})
		}
	}
	slices.Sort(removed)
	slices.SortFunc(moved, func(a, b Renumbered) int { return strings.Compare(a.Old, b.Old) })

	existing, err := e.existingSum()
	if err != nil {
		return nil, nil, nil, err
	}
	kept := &Sum{Format: existing.Format, Files: maps.Clone(existing.Files)}
	for _, p := range removed {
		delete(kept.Files, p)
	}
	sum, err := AppendSum(kept, added, addedNames)
	if err != nil {
		return nil, nil, nil, err
	}
	var buf bytes.Buffer
	if err := sum.Write(&buf); err != nil {
		return nil, nil, nil, err
	}
	out[SumFileName] = buf.Bytes()
	return removed, out, moved, nil
}
