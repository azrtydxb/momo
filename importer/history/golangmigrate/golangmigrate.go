// SPDX-License-Identifier: Apache-2.0

// Package golangmigrate reads a golang-migrate directory and its
// schema_migrations table into a history.Source. The table layout (version
// bigint primary key, dirty boolean) follows the PostgreSQL database driver of
// github.com/golang-migrate/migrate/v4; golang-migrate itself is not a
// dependency.
package golangmigrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/importer/history"
	"github.com/azrtydxb/momo/importer/history/internal/histutil"
)

var fileRe = regexp.MustCompile(`^(\d+)_(.*)\.(up|down)\.sql$`)

// Read returns a history.Source. Files are <version>_<title>.up.sql and
// <version>_<title>.down.sql; the version is the integer prefix as written.
//
// golang-migrate records one row, the current version, and treats every
// migration up to it as applied. Applied therefore lists every version up to
// and including the row's version when the row is clean. A dirty row means
// that version failed half way: Dirty is true and Applied lists the versions
// before it. An empty table means nothing is applied.
func Read(dir fs.FS, c momo.PinnedConn, table string) (history.Source, error) {
	src := history.Source{Tool: "golangmigrate", Applied: []string{}}
	ents, err := fs.ReadDir(dir, ".")
	if err != nil {
		return src, fmt.Errorf("golangmigrate: %w", err)
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
		m := fileRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		b, err := fs.ReadFile(dir, e.Name())
		if err != nil {
			return src, fmt.Errorf("golangmigrate: %w", err)
		}
		p := byVersion[m[1]]
		if p == nil {
			p = &pair{name: m[2]}
			byVersion[m[1]] = p
			versions = append(versions, m[1])
		}
		if m[3] == "up" {
			p.up, p.upFile, p.hasUp = b, e.Name(), true
		} else {
			p.down = b
		}
	}
	sort.Slice(versions, func(i, j int) bool { return histutil.LessNumeric(versions[i], versions[j]) })
	for _, v := range versions {
		p := byVersion[v]
		if !p.hasUp {
			src.Unsupported = append(src.Unsupported, v+"_"+p.name+".down.sql (no matching .up.sql)")
			continue
		}
		src.Migrations = append(src.Migrations, history.Raw{Version: v, Name: p.name, Up: p.up, Down: p.down, Origin: p.upFile})
	}
	tbl, err := histutil.QuoteTable(table)
	if err != nil {
		return src, err
	}
	rows, err := c.Query(context.Background(), "SELECT version, dirty FROM "+tbl, nil)
	if err != nil {
		return src, fmt.Errorf("golangmigrate: read %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	var cur int64
	var dirty, seen bool
	for rows.Next() {
		if seen {
			return src, fmt.Errorf("golangmigrate: %s holds more than one row; golang-migrate keeps exactly one", table)
		}
		if err := rows.Scan(&cur, &dirty); err != nil {
			return src, fmt.Errorf("golangmigrate: read %s: %w", table, err)
		}
		seen = true
	}
	if err := rows.Err(); err != nil {
		return src, fmt.Errorf("golangmigrate: read %s: %w", table, err)
	}
	if !seen {
		return src, nil
	}
	src.Dirty = dirty
	for _, m := range src.Migrations {
		v, err := strconv.ParseInt(m.Version, 10, 64)
		if err != nil {
			return src, fmt.Errorf("golangmigrate: %s: version %q: %w", m.Origin, m.Version, err)
		}
		if v < cur || (v == cur && !dirty) {
			src.Applied = append(src.Applied, m.Version)
		}
	}
	if cur != 0 && len(src.Migrations) > 0 && !hasVersion(src.Migrations, cur) {
		return src, errors.New("golangmigrate: " + table + " is at version " + strconv.FormatInt(cur, 10) + ", which has no file in the directory")
	}
	return src, nil
}

func hasVersion(ms []history.Raw, v int64) bool {
	for _, m := range ms {
		if n, err := strconv.ParseInt(m.Version, 10, 64); err == nil && n == v {
			return true
		}
	}
	return false
}
