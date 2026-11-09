// SPDX-License-Identifier: Apache-2.0

package gendocs

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// DirectDeps returns the direct requirements of the go.mod text, in file
// order, as module path and version.
func DirectDeps(gomod string) [][2]string {
	var out [][2]string
	inBlock := false
	for _, line := range strings.Split(gomod, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "require (":
			inBlock = true
			continue
		case inBlock && line == ")":
			inBlock = false
			continue
		case !inBlock && strings.HasPrefix(line, "require "):
			line = strings.TrimSpace(strings.TrimPrefix(line, "require "))
		case !inBlock:
			continue
		}
		if line == "" || strings.HasPrefix(line, "//") || strings.Contains(line, "// indirect") {
			continue
		}
		f := strings.Fields(line)
		if len(f) >= 2 {
			out = append(out, [2]string{f[0], f[1]})
		}
	}
	return out
}

// depNotes says what each direct dependency is used for. A dependency without
// an entry shows an empty note.
var depNotes = map[string]string{
	"github.com/asg017/sqlite-vec-go-bindings": "sqlite-vec for `driver/sqlitedriver` (cgo).",
	"github.com/go-sql-driver/mysql":           "used by `driver/mysqldriver` and by test harness code (`internal/testdb`).",
	"github.com/jackc/pgx/v5":                  "PostgreSQL driver for `driver/pgxdriver`.",
	"github.com/mattn/go-sqlite3":              "used by `driver/sqlitedriver` (cgo).",
	"go.yaml.in/yaml/v3":                       "`momo.yaml`.",
	"golang.org/x/tools":                       "generator.",
	"modernc.org/sqlite":                       "pure-Go SQLite for `driver/sqlitepure` (no cgo, no vectors).",
}

// dependenciesBlock is the table of direct go.mod requirements.
func dependenciesBlock(root string) (string, error) {
	gomod, err := readFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	var rows [][]string
	for _, d := range DirectDeps(gomod) {
		rows = append(rows, []string{code(d[0]), d[1], depNotes[d[0]]})
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("go.mod has no direct requirements")
	}
	return table([]string{"Module", "Version", "Used by"}, rows), nil
}

// checkFeatureAPI fails when an identifier named by the features data does not
// exist as an exported declaration in its package directory, so a rename in
// the code cannot leave the documentation naming the old identifier.
func checkFeatureAPI(root string) error {
	fset := token.NewFileSet()
	for _, f := range features {
		for _, id := range f.API {
			pkg, name, ok := strings.Cut(id, ".")
			if !ok {
				return fmt.Errorf("feature %q: %q is not package.Name", f.Name, id)
			}
			paths, err := filepath.Glob(filepath.Join(root, pkg, "*.go"))
			if err != nil {
				return err
			}
			var files []*ast.File
			for _, path := range paths {
				if strings.HasSuffix(path, "_test.go") {
					continue
				}
				src, err := readFile(path)
				if err != nil {
					return err
				}
				af, err := parser.ParseFile(fset, path, src, 0)
				if err != nil {
					return fmt.Errorf("feature %q: %s: %w", f.Name, id, err)
				}
				files = append(files, af)
			}
			if !declares(files, name) {
				return fmt.Errorf("feature %q names %s, which package %s does not declare", f.Name, id, pkg)
			}
		}
	}
	return nil
}

// declares reports whether the files declare the top-level name.
func declares(files []*ast.File, name string) bool {
	for _, f := range files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.Name == name {
					return true
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						if s.Name.Name == name {
							return true
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							if n.Name == name {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}
