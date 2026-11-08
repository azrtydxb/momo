// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/azrtydxb/momo/schema"
)

// ValidateReport lists what is wrong with a migration directory. Each problem
// names the offending file. An empty report means the directory is valid.
type ValidateReport struct{ Problems []string }

// ValidateOffline checks a migration directory without a database:
//
//  1. every migration file parses and its directives are known;
//  2. momo.sum exists and matches the files;
//  3. versions are unique and strictly increasing;
//  4. every snapshot names a parent that exists (or none for the first);
//  5. no two snapshots share a parent (the ErrBranchConflict text);
//  6. every migration has a snapshot, and a custom (not generated)
//     migration's snapshot equals its parent's.
//
// Go migrations have no file, so only the application knows them: pass the
// ones registered with WithGoMigrations and each is checked like a custom
// migration (a snapshot that equals its parent's, a version no SQL file
// shares). Without them a snapshot that has no up file is not checked.
//
// It returns an error only when the directory cannot be read.
func ValidateOffline(fsys fs.FS, goMigrations ...GoMigration) (*ValidateReport, error) {
	if _, err := fs.ReadDir(fsys, "."); err != nil {
		return nil, fmt.Errorf("migrate: read migration directory: %w", err)
	}
	rep := &ValidateReport{}
	add := func(format string, args ...any) {
		rep.Problems = append(rep.Problems, fmt.Sprintf(format, args...))
	}

	files, err := Discover(fsys)
	if err != nil {
		add("%v", err)
		return rep, nil
	}
	directives := map[string]Directives{}
	for _, f := range files {
		if f.Kind == "snapshot" {
			continue
		}
		d, err := ParseDirectives(f.Body)
		if err != nil {
			add("%s: %v", f.Path, err)
			continue
		}
		directives[f.Path] = d
	}

	if len(files) > 0 {
		checkSum(fsys, add)
	}

	// Checks 3 to 5 are the chain: unique, increasing, parent exists, no fork.
	entries, chainProblems := loadChain(files)
	for _, p := range chainProblems {
		add("%v", p)
	}

	// Check 6: every migration has a snapshot; custom ones do not change it.
	snapshots := map[string]chainEntry{}
	for _, e := range entries {
		snapshots[e.snap.Version] = e
	}
	for _, f := range files {
		if f.Kind != "up" {
			continue
		}
		e, ok := snapshots[f.Version]
		if !ok {
			add("%s has no snapshot file %s_%s.snapshot.json", f.Path, f.Version, f.Name)
			continue
		}
		d, known := directives[f.Path]
		if !known || d.Generated || e.snap.ParentVersion == "" {
			continue
		}
		parent, ok := snapshots[e.snap.ParentVersion]
		if !ok {
			continue // reported as a missing parent
		}
		if !schemaEqual(e.snap.Schema, parent.snap.Schema) {
			add("%s: custom migration %s must not change the schema, but its snapshot differs from its parent's %s", e.path, f.Path, parent.path)
		}
	}
	validateGo(goMigrations, files, snapshots, add)
	return rep, nil
}

func schemaEqual(a, b *schema.Schema) bool {
	if a == nil || b == nil {
		return a == b
	}
	return schema.Equal(a, b)
}

func checkSum(fsys fs.FS, add func(string, ...any)) {
	data, err := fs.ReadFile(fsys, SumFileName)
	if err != nil {
		add("%s is missing; run momo migrate hash to create it", SumFileName)
		return
	}
	want, err := ParseSum(strings.NewReader(string(data)))
	if err != nil {
		add("%s: %v", SumFileName, err)
		return
	}
	for _, name := range VerifySum(fsys, want) {
		add("%s mismatch: %s was added, removed or changed since the sum was written", SumFileName, name)
	}
}
