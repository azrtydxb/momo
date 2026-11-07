// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"time"
)

// versionLayout is the UTC timestamp layout of a migration version.
const versionLayout = "20060102150405"

// chainEntry is one snapshot together with the file it came from.
type chainEntry struct {
	path string
	snap SnapshotFile
}

// loadChain reads every snapshot in fsys and checks that they form one linear
// chain. It returns the snapshots ordered root to head, and one error per
// problem; the order is only meaningful when there are no problems. Each error
// names the offending files.
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
	structural := len(problems)

	// Forks: no two snapshots may share a parent, and only one may be a root.
	children := map[string][]string{}
	for _, v := range versions {
		p := byVersion[v].snap.ParentVersion
		children[p] = append(children[p], v)
	}
	for _, parent := range slices.Sorted(maps.Keys(children)) {
		kids := children[parent]
		for _, other := range kids[1:] {
			a, b := byVersion[kids[0]].path, byVersion[other].path
			if parent == "" {
				problems = append(problems, fmt.Errorf("%w: %s and %s both have no parent; rebase one branch by hand (see docs/migrations.md)", ErrBranchConflict, a, b))
			} else {
				problems = append(problems, fmt.Errorf("%w: %s and %s both have parent %s; rebase one branch by hand (see docs/migrations.md)", ErrBranchConflict, a, b, parent))
			}
		}
	}

	// Parents must exist.
	for _, v := range versions {
		e := byVersion[v]
		if p := e.snap.ParentVersion; p != "" {
			if _, ok := byVersion[p]; !ok {
				problems = append(problems, fmt.Errorf("migrate: %s: parent snapshot %s is missing", e.path, p))
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

	// Linearity: each snapshot's parent is the snapshot just before it.
	if len(problems) == structural {
		prev := ""
		for _, v := range versions {
			e := byVersion[v]
			if e.snap.ParentVersion != prev {
				want := "none"
				if prev != "" {
					want = prev
				}
				problems = append(problems, fmt.Errorf("migrate: %s: parent is %q but the previous snapshot is %s (gap or out-of-order version)", e.path, e.snap.ParentVersion, want))
			}
			prev = v
		}
	}

	entries := make([]chainEntry, 0, len(versions))
	for _, v := range versions {
		entries = append(entries, byVersion[v])
	}
	return entries, problems
}

// Chain returns the snapshots in fsys ordered root to head. It fails when the
// snapshots do not form one linear chain: a fork (the error wraps
// ErrBranchConflict), a missing parent, a cycle, or a parent that is not the
// previous version. Every error names the offending files.
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
