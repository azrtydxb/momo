// SPDX-License-Identifier: Apache-2.0

package gendocs

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// nonDriverDirs are directories under driver/ that hold no driver package.
// Every other directory there must declare itself.
var nonDriverDirs = map[string]bool{"internal": true, "testdata": true}

// driverInfo is a driver package's own declaration of its database status: the
// //momo:driver directive in its status.go.
type driverInfo struct {
	Dir      string       // "driver/pgxdriver"
	Dialect  dialect.Name // the dialect the package serves
	Usable   bool         // the package runs queries and writes for the dialect
	NeedsCGO bool         // the package needs cgo
	Windows  bool         // the package builds on Windows
	Vectors  string       // "yes", "no" or "server"
}

const directive = "//momo:driver"

// scanDrivers reads the declaration of every package under driver/ and fails
// when a package has none, has more than one, or declares something invalid.
// It reads comments only and imports no driver.
func scanDrivers(root string) ([]driverInfo, error) {
	dirs, err := os.ReadDir(filepath.Join(root, "driver"))
	if err != nil {
		return nil, err
	}
	var out []driverInfo
	for _, d := range dirs {
		if !d.IsDir() || nonDriverDirs[d.Name()] {
			continue
		}
		info, err := readDeclaration(root, d.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	return out, nil
}

func readDeclaration(root, name string) (driverInfo, error) {
	dir := "driver/" + name
	path := filepath.Join(root, "driver", name, "status.go")
	src, err := readFile(path)
	if err != nil {
		return driverInfo{}, fmt.Errorf("%s has no status.go with a %s declaration (add one, or list the directory in nonDriverDirs if it is not a driver): %w", dir, directive, err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), path, src, parser.ParseComments)
	if err != nil {
		return driverInfo{}, fmt.Errorf("%s/status.go: %w", dir, err)
	}
	var lines []string
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, directive+" ") {
				lines = append(lines, strings.TrimPrefix(c.Text, directive+" "))
			}
		}
	}
	if len(lines) != 1 {
		return driverInfo{}, fmt.Errorf("%s/status.go must hold exactly one %s declaration, found %d", dir, directive, len(lines))
	}
	kv := map[string]string{}
	for _, field := range strings.Fields(lines[0]) {
		k, v, ok := strings.Cut(field, "=")
		if !ok {
			return driverInfo{}, fmt.Errorf("%s/status.go: %q is not key=value", dir, field)
		}
		kv[k] = v
	}
	info := driverInfo{Dir: dir}
	for _, k := range []string{"dialect", "usable", "cgo", "windows", "vectors"} {
		if _, ok := kv[k]; !ok {
			return driverInfo{}, fmt.Errorf("%s/status.go: declaration lacks %s=", dir, k)
		}
	}
	if len(kv) != 5 {
		return driverInfo{}, fmt.Errorf("%s/status.go: declaration has an unknown key: %q", dir, lines[0])
	}
	switch dialect.Name(kv["dialect"]) {
	case dialect.Postgres, dialect.SQLite, dialect.MySQL:
		info.Dialect = dialect.Name(kv["dialect"])
	default:
		return driverInfo{}, fmt.Errorf("%s/status.go: unknown dialect %q", dir, kv["dialect"])
	}
	for k, dst := range map[string]*bool{"usable": &info.Usable, "cgo": &info.NeedsCGO, "windows": &info.Windows} {
		switch kv[k] {
		case "true":
			*dst = true
		case "false":
		default:
			return driverInfo{}, fmt.Errorf("%s/status.go: %s must be true or false, got %q", dir, k, kv[k])
		}
	}
	switch kv["vectors"] {
	case "yes", "no", "server":
		info.Vectors = kv["vectors"]
	default:
		return driverInfo{}, fmt.Errorf("%s/status.go: vectors must be yes, no or server, got %q", dir, kv["vectors"])
	}
	return info, nil
}

// driversFor returns the usable drivers that serve dialect n, in directory
// order.
func driversFor(all []driverInfo, n dialect.Name) []driverInfo {
	var out []driverInfo
	for _, d := range all {
		if d.Dialect == n && d.Usable {
			out = append(out, d)
		}
	}
	return out
}

// cliDialects returns the dialect names the momo command runs migrations for,
// read from the dialect switch in cmd/momo: the dialect.* identifiers in the
// body of dialectImpl. It fails when that function is missing, so a refactor of
// the switch cannot silently empty the answer.
func cliDialects(root string) (map[string]bool, error) {
	files, err := filepath.Glob(filepath.Join(root, "cmd", "momo", "*.go"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	fset := token.NewFileSet()
	found := false
	out := map[string]bool{}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := readFile(path)
		if err != nil {
			return nil, err
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return nil, err
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "dialectImpl" || fn.Body == nil {
				continue
			}
			found = true
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "dialect" {
					switch sel.Sel.Name {
					case "Postgres":
						out[string(dialect.Postgres)] = true
					case "SQLite":
						out[string(dialect.SQLite)] = true
					case "MySQL":
						out[string(dialect.MySQL)] = true
					}
				}
				return true
			})
		}
	}
	if !found {
		return nil, fmt.Errorf("cmd/momo has no dialectImpl function; gendocs reads the CLI's dialect switch there")
	}
	return out, nil
}

// hasMigrateDialect reports whether migrate/<name> exists as a package.
func hasMigrateDialect(root, name string) bool {
	files, _ := filepath.Glob(filepath.Join(root, "migrate", name, "*.go"))
	return len(files) > 0
}
