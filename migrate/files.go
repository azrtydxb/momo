// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"bytes"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"time"
)

// File is one migration file found in a dialect directory.
type File struct {
	Version string // from file name
	Name    string
	Kind    string // "up", "down", "snapshot"
	Path    string // relative to the dialect directory
	Body    []byte
}

// Directives are the `-- momo:` directives in a migration's leading comment block.
type Directives struct {
	NoTransaction        bool // -- momo:no-transaction
	Destructive          bool // -- momo:destructive
	Split                bool // -- momo:split
	SQLiteForeignKeysOff bool // -- momo:sqlite-foreign-keys-off
	Generated            bool // -- momo:generated (written by diff)
}

// ParseDirectives reads the directives in the leading comment block of body,
// that is, the comment and blank lines before the first statement. An
// unknown directive is an error naming its line.
func ParseDirectives(body []byte) (Directives, error) {
	d, err := parseDirectives(bytes.TrimPrefix(body, utf8BOM))
	return d, err
}

// parseDirectives returns the directives found before the first error.
func parseDirectives(body []byte) (Directives, error) {
	var d Directives
	inBlock := false
	for n, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(raw)
		if inBlock {
			if strings.Contains(line, "*/") {
				inBlock = false
			}
			continue
		}
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "/*"):
			inBlock = !strings.Contains(line[2:], "*/")
			continue
		case !strings.HasPrefix(line, "--"):
			return d, nil
		}
		text := strings.TrimSpace(strings.TrimLeft(line, "-"))
		rest, ok := strings.CutPrefix(text, "momo:")
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimSpace(rest), " ")
		switch name {
		case "no-transaction":
			d.NoTransaction = true
		case "destructive":
			d.Destructive = true
		case "split":
			d.Split = true
		case "sqlite-foreign-keys-off":
			d.SQLiteForeignKeysOff = true
		case "generated":
			d.Generated = true
		default:
			return d, fmt.Errorf("migrate: line %d: unknown directive %q", n+1, "-- momo:"+name)
		}
	}
	return d, nil
}

var (
	sqlName      = regexp.MustCompile(`^(\d{14})_([a-z][a-z0-9_]*)\.(up|down)\.sql$`)
	snapshotName = regexp.MustCompile(`^(\d{14})_([a-z][a-z0-9_]*)\.snapshot\.json$`)
	looksLike    = regexp.MustCompile(`^\d+_`)
)

// Discover lists the migration files in the top level of fsys, sorted by
// path. It ignores momo.sum, directories and unrelated files. A file that
// looks like a migration (digits, then an underscore) but is malformed, a
// version that is not a valid UTC timestamp, a repeated version and kind, and
// one version under two names are errors that name the files.
func Discover(fsys fs.FS) ([]File, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: read migration directory: %w", err)
	}
	var files []File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == "momo.sum" || !looksLike.MatchString(name) {
			continue
		}
		var m []string
		kind := ""
		if m = sqlName.FindStringSubmatch(name); m != nil {
			kind = m[3]
		} else if m = snapshotName.FindStringSubmatch(name); m != nil {
			kind = "snapshot"
		} else {
			return nil, fmt.Errorf("migrate: %s: malformed migration file name; want <14-digit version>_<name>.up.sql, .down.sql or .snapshot.json with a lowercase name", name)
		}
		if _, err := time.Parse("20060102150405", m[1]); err != nil {
			return nil, fmt.Errorf("migrate: %s: version %s is not a UTC timestamp (YYYYMMDDHHMMSS)", name, m[1])
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("migrate: read %s: %w", name, err)
		}
		files = append(files, File{Version: m[1], Name: m[2], Kind: kind, Path: name, Body: body})
	}
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	seen := map[string]File{}
	names := map[string]File{}
	for _, f := range files {
		if p, ok := names[f.Version]; ok && p.Name != f.Name {
			return nil, fmt.Errorf("migrate: %s and %s share version %s under different names", p.Path, f.Path, f.Version)
		}
		names[f.Version] = f
		key := f.Version + "/" + f.Kind
		if p, ok := seen[key]; ok {
			return nil, fmt.Errorf("migrate: %s and %s are both the %s file of version %s", p.Path, f.Path, f.Kind, f.Version)
		}
		seen[key] = f
	}
	return files, nil
}
