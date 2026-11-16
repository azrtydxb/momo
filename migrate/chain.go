// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"time"

	"github.com/azrtydxb/momo/schema"
)

// versionLayout is the UTC timestamp layout of a migration version.
const versionLayout = "20060102150405"

// chainEntry is one snapshot together with the file it came from. state is the
// schema after this migration, derived by folding the chain (see foldChain);
// it is nil until loadChain has folded without a problem.
type chainEntry struct {
	path  string
	snap  SnapshotFile
	state *schema.Schema
}

// derived returns the schema after this migration: the folded state when the
// chain folded, else the snapshot file. After a merge the file lacks the other
// branch's work and the state has it.
func (e chainEntry) derived() *schema.Schema {
	if e.state != nil {
		return e.state
	}
	return e.snap.Schema
}

// leavesSchemaOf reports whether child changes nothing relative to this
// migration, its parent. A snapshot marked as generated from a derived state
// must equal the derived schema; one written before a merge may equal the
// parent's own stored snapshot as well.
func (e chainEntry) leavesSchemaOf(child *SnapshotFile) bool {
	if schemaEqual(child.Schema, e.derived()) {
		return true
	}
	return !baseIsDerived(child.Base) && schemaEqual(child.Schema, e.snap.Schema)
}

// loadChain reads every snapshot in fsys and checks that they form a chain:
// one root, every parent present and older than its child, no cycle, and every
// fork mergeable (the branches change different tables, see foldChain). It
// returns the snapshots ordered by version, and one error per problem; the
// order is only meaningful when there are no problems. Each error names the
// offending files.
func loadChain(files []File) ([]chainEntry, []error) {
	var problems []error
	byVersion := map[string]chainEntry{}
	var versions []string
	for _, f := range files {
		if f.Kind != "snapshot" {
			continue
		}
		s, err := ReadSnapshot(strings.NewReader(string(f.Body)))
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", f.Path, err))
			continue
		}
		if s.Version != f.Version {
			problems = append(problems, fmt.Errorf("migrate: %s records version %q but its file name says %s", f.Path, s.Version, f.Version))
			continue
		}
		byVersion[f.Version] = chainEntry{path: f.Path, snap: *s}
		versions = append(versions, f.Version)
	}
	slices.Sort(versions)

	// Only one snapshot may be a root. Two branches that each start the history
	// have no common base to merge from.
	var roots []string
	for _, v := range versions {
		if byVersion[v].snap.ParentVersion == "" {
			roots = append(roots, v)
		}
	}
	for _, other := range roots[min(1, len(roots)):] {
		problems = append(problems, fmt.Errorf("%w: %s and %s both have no parent; rebase one branch by hand (see docs/migrations.md)", ErrBranchConflict, byVersion[roots[0]].path, byVersion[other].path))
	}

	// Parents must exist and be older than their child.
	for _, v := range versions {
		e := byVersion[v]
		if p := e.snap.ParentVersion; p != "" {
			if _, ok := byVersion[p]; !ok {
				problems = append(problems, fmt.Errorf("migrate: %s: parent snapshot %s is missing", e.path, p))
			} else if p >= v {
				problems = append(problems, fmt.Errorf("migrate: %s: parent %s is not older than version %s (a migration builds on an earlier one)", e.path, p, v))
			}
		}
	}

	// Cycles: following parents from any snapshot must reach a root.
	inCycle := map[string]bool{}
	for _, v := range versions {
		if inCycle[v] {
			continue
		}
		var path []string
		seen := map[string]int{}
		cur := v
		for cur != "" {
			if i, ok := seen[cur]; ok {
				names := make([]string, 0, len(path)-i)
				for _, c := range path[i:] {
					inCycle[c] = true
					names = append(names, byVersion[c].path)
				}
				problems = append(problems, fmt.Errorf("migrate: snapshot parent cycle among %s", strings.Join(names, ", ")))
				break
			}
			e, ok := byVersion[cur]
			if !ok {
				break
			}
			seen[cur] = len(path)
			path = append(path, cur)
			cur = e.snap.ParentVersion
		}
	}

	entries := make([]chainEntry, 0, len(versions))
	for _, v := range versions {
		entries = append(entries, byVersion[v])
	}
	// Forks: fold the chain in version order. A fork whose branches change
	// different tables merges; one that changes the same table is a conflict.
	if len(problems) == 0 {
		states, foldProblems := foldChain(entries)
		problems = append(problems, foldProblems...)
		if len(foldProblems) == 0 {
			for i := range entries {
				entries[i].state = states[i]
			}
		}
	}
	return entries, problems
}

// Chain returns the snapshots in fsys ordered by version. It fails when the
// snapshots do not form a valid chain: a fork whose branches change the same
// table or a second root (the error wraps ErrBranchConflict), a missing parent,
// a cycle, or a parent that is not older than its child. Every error names the
// offending files.
func Chain(fsys fs.FS) ([]SnapshotFile, error) {
	files, err := Discover(fsys)
	if err != nil {
		return nil, err
	}
	entries, problems := loadChain(files)
	if len(problems) > 0 {
		return nil, errors.Join(problems...)
	}
	out := make([]SnapshotFile, len(entries))
	for i, e := range entries {
		out[i] = e.snap
	}
	return out, nil
}

// NextVersion returns the version for a new migration: the UTC time from now
// formatted as YYYYMMDDHHMMSS. When fsys already holds a version at or after
// that time (a clock behind a teammate's, or two migrations in one second) it
// returns the second after the latest existing version, so versions only
// grow. The clock is injected; NextVersion never reads the wall clock.
func NextVersion(now func() time.Time, fsys fs.FS) (string, error) {
	files, err := Discover(fsys)
	if err != nil {
		return "", err
	}
	v := now().UTC().Format(versionLayout)
	for _, f := range files {
		if f.Version >= v {
			t, perr := time.Parse(versionLayout, f.Version)
			if perr != nil {
				return "", fmt.Errorf("migrate: %s: %w", f.Path, perr)
			}
			v = t.Add(time.Second).Format(versionLayout)
		}
	}
	return v, nil
}
