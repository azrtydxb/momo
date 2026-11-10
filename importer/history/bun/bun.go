// SPDX-License-Identifier: Apache-2.0

// Package bun reads the SQL migrations of a Bun project and its
// bun_migrations table into a history.Source. The file naming
// (<timestamp>_<name>.up.sql, .down.sql and the .tx.up.sql variants) and the
// table layout (id, name, group_id, migrated_at; name holds the version
// prefix) follow github.com/uptrace/bun/migrate at v1.3.0; Bun itself is not a
// dependency. Go-function migrations (.go files) are listed as unsupported.
package bun

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/importer/history"
	"github.com/azrtydxb/momo/importer/history/internal/histutil"
)

// sqlSuffixes are the SQL migration file endings in the order they are
// tried; the .tx. forms must come before the plain ones.
var sqlSuffixes = []struct {
	suffix string
	up     bool
}{
	{".tx.up.sql", true}, {".up.sql", true}, {".tx.down.sql", false}, {".down.sql", false},
}

// Read returns a history.Source for Bun's SQL migrations: files
// <version>_<name>.up.sql (or .tx.up.sql) and the optional .down.sql. Bun's
// Go-function migrations (.go files) are not imported: each one is listed in
// Source.Unsupported, and history.Convert refuses the source. Applied state
// comes from the bun_migrations table; its name column holds the version
// prefix (the text before the first underscore is used, so a full file stem
// matches too). table is the migrations table, "" for bun_migrations.
// bun_migration_locks is ignored. A row with no file in the
// directory is an error.
func Read(dir fs.FS, c momo.PinnedConn, table string) (history.Source, error) {
	if table == "" {
		table = "bun_migrations"
	}
	src := history.Source{Tool: "bun", Applied: []string{}}
	ents, err := fs.ReadDir(dir, ".")
	if err != nil {
		return src, fmt.Errorf("bun: %w", err)
	}
	type pair struct {
		name     string
		up, down []byte
		upFile   string
		hasUp    bool
	}
	byVersion := map[string]*pair{}
	var versions []string
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasSuffix(n, ".go") {
			src.Unsupported = append(src.Unsupported, n)
			continue
		}
		var stem string
		var isUp, matched bool
		for _, s := range sqlSuffixes {
			if t, ok := strings.CutSuffix(n, s.suffix); ok {
				stem, isUp, matched = t, s.up, true
				break
			}
		}
		if !matched {
			continue
		}
		version, name, ok := strings.Cut(stem, "_")
		if !ok || version == "" || name == "" {
			src.Unsupported = append(src.Unsupported, n)
			continue
		}
		b, err := fs.ReadFile(dir, n)
		if err != nil {
			return src, fmt.Errorf("bun: %w", err)
		}
		p := byVersion[version]
		if p == nil {
			p = &pair{name: name}
			byVersion[version] = p
			versions = append(versions, version)
		}
		if isUp {
			if p.hasUp {
				return src, fmt.Errorf("bun: %s and %s both define the up migration of version %s", p.upFile, n, version)
			}
			p.up, p.upFile, p.hasUp = b, n, true
		} else {
			if p.down != nil {
				return src, fmt.Errorf("bun: two down files for version %s", version)
			}
			p.down = b
		}
	}
	sort.Slice(versions, func(i, j int) bool { return histutil.LessNumeric(versions[i], versions[j]) })
	for _, v := range versions {
		p := byVersion[v]
		if !p.hasUp {
			src.Unsupported = append(src.Unsupported, v+"_"+p.name+" (a down file without an up file)")
			continue
		}
		src.Migrations = append(src.Migrations, history.Raw{Version: v, Name: p.name, Up: p.up, Down: p.down, Origin: p.upFile})
	}
	if err := histutil.CheckDuplicates(versions); err != nil {
		return src, err
	}
	tbl, err := histutil.QuoteTable(table)
	if err != nil {
		return src, err
	}
	rows, err := c.Query(context.Background(), "SELECT name FROM "+tbl+" ORDER BY id", nil)
	if err != nil {
		return src, fmt.Errorf("bun: read %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	recorded := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return src, fmt.Errorf("bun: read %s: %w", table, err)
		}
		v, _, _ := strings.Cut(name, "_")
		recorded[v] = true
	}
	if err := rows.Err(); err != nil {
		return src, fmt.Errorf("bun: read %s: %w", table, err)
	}
	for _, m := range src.Migrations {
		if recorded[m.Version] {
			src.Applied = append(src.Applied, m.Version)
			delete(recorded, m.Version)
		}
	}
	// With unsupported files the source is refused by Convert anyway, and a
	// recorded row may belong to one of those files.
	if len(src.Unsupported) == 0 && len(recorded) > 0 {
		left := make([]string, 0, len(recorded))
		for v := range recorded {
			left = append(left, v)
		}
		sort.Strings(left)
		return src, fmt.Errorf("bun: %s lists %s as applied but the directory has no file for it", table, strings.Join(left, ", "))
	}
	return src, nil
}
