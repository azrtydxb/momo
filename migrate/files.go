// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
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

// walkErrPrefix starts the errors walkFiles itself produces; callers pass
// errors with this prefix through unwrapped.
const walkErrPrefix = "migrations: "

// walkFiles calls fn with the slash-separated path of every regular file in
// fsys, descending into nested directories, except the root momo.sum. Any
// entry whose name starts with a dot is skipped, and a dot directory is not
// entered: editors, VCS metadata (.git), OS files (.DS_Store) and momo's own
// in-flight temporary files (.momo-*.tmp) never belong to a migration
// directory's content. fs.FS paths always use forward slashes, so the paths
// are never OS-specific.
//
// Symlinks: a symlink to a file is followed and read as that file. A symlink
// to a directory is refused with an error ("migrations: <path> is a symlink to
// a directory; not supported"), because fs.WalkDir does not descend into it
// and silently skipping it would leave its files unhashed.
func walkFiles(fsys fs.FS, fn func(p string) error) error {
	return fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != "." && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || p == SumFileName {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			if fi, err := fs.Stat(fsys, p); err == nil && fi.IsDir() {
				return fmt.Errorf("%s%s is a symlink to a directory; not supported", walkErrPrefix, p)
			}
		}
		return fn(p)
	})
}

// Discover lists the migration files in fsys, walking nested directories
// (for example an archive/2026 folder), ordered by version and then by path,
// so an archived old migration still sorts before a newer top-level one. A
// File's Path is its slash-separated path relative to fsys. It ignores the
// root momo.sum, dotfiles and dot directories (see walkFiles), and unrelated files. A file that looks like a migration
// (digits, then an underscore) but is malformed, a version that is not a
// valid UTC timestamp, a repeated version and kind, one version under two
// names, and one version spread over two directories are errors that name the
// files.
func Discover(fsys fs.FS) ([]File, error) {
	var files []File
	err := walkFiles(fsys, func(p string) error {
		name := path.Base(p)
		if !looksLike.MatchString(name) {
			return nil
		}
		var m []string
		kind := ""
		if m = sqlName.FindStringSubmatch(name); m != nil {
			kind = m[3]
		} else if m = snapshotName.FindStringSubmatch(name); m != nil {
			kind = "snapshot"
		} else {
			return fmt.Errorf("migrate: %s: malformed migration file name; want <14-digit version>_<name>.up.sql, .down.sql or .snapshot.json with a lowercase name", p)
		}
		if _, err := time.Parse("20060102150405", m[1]); err != nil {
			return fmt.Errorf("migrate: %s: version %s is not a UTC timestamp (YYYYMMDDHHMMSS)", p, m[1])
		}
		body, err := fs.ReadFile(fsys, p)
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", p, err)
		}
		files = append(files, File{Version: m[1], Name: m[2], Kind: kind, Path: p, Body: body})
		return nil
	})
	if err != nil {
		if strings.HasPrefix(err.Error(), "migrate: ") || strings.HasPrefix(err.Error(), walkErrPrefix) {
			return nil, err
		}
		return nil, fmt.Errorf("migrate: read migration directory: %w", err)
	}
	slices.SortFunc(files, func(a, b File) int {
		if c := strings.Compare(a.Version, b.Version); c != 0 {
			return c
		}
		return strings.Compare(a.Path, b.Path)
	})
	seen := map[string]File{}
	names := map[string]File{}
	for _, f := range files {
		if p, ok := names[f.Version]; ok {
			if p.Name != f.Name {
				return nil, fmt.Errorf("migrate: %s and %s share version %s under different names", p.Path, f.Path, f.Version)
			}
			if path.Dir(p.Path) != path.Dir(f.Path) {
				return nil, fmt.Errorf("migrate: %s and %s hold files of the same version %s in different directories", p.Path, f.Path, f.Version)
			}
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
