// SPDX-License-Identifier: Apache-2.0

package gendocs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// driverInfo is what the source of one package under driver/ says about it.
type driverInfo struct {
	Dir        string                // "driver/pgxdriver"
	Dialects   map[dialect.Name]bool // dialects the package names through the dialect package
	NeedsCGO   bool                  // a file builds only with cgo
	NoWindows  bool                  // a file excludes Windows
	Vectors    string                // "yes", "no" or "varies" from momo.Capabilities literals
	vectorSeen map[string]bool
}

var cgoConstraint = regexp.MustCompile(`(^|[^!\w])cgo\b`)

// scanDrivers reads every package under driver/. A package counts for a
// dialect when its non-test source refers to dialect.Postgres, dialect.SQLite,
// dialect.MySQL or dialect.MySQLFlavor, so adding a driver package flips the
// dialect to usable without a change here.
func scanDrivers(root string) ([]driverInfo, error) {
	dirs, err := os.ReadDir(filepath.Join(root, "driver"))
	if err != nil {
		return nil, err
	}
	var out []driverInfo
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		info, err := scanDriver(root, d.Name())
		if err != nil {
			return nil, err
		}
		if len(info.Dialects) > 0 {
			out = append(out, info)
		}
	}
	return out, nil
}

func scanDriver(root, name string) (driverInfo, error) {
	info := driverInfo{
		Dir:        "driver/" + name,
		Dialects:   map[dialect.Name]bool{},
		vectorSeen: map[string]bool{},
	}
	files, err := filepath.Glob(filepath.Join(root, "driver", name, "*.go"))
	if err != nil {
		return info, err
	}
	sort.Strings(files)
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := readFile(path)
		if err != nil {
			return info, err
		}
		for _, line := range strings.Split(src, "\n") {
			if !strings.HasPrefix(line, "//go:build ") {
				continue
			}
			if cgoConstraint.MatchString(line) {
				info.NeedsCGO = true
			}
			if strings.Contains(line, "!windows") {
				info.NoWindows = true
			}
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return info, err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.SelectorExpr:
				if id, ok := n.X.(*ast.Ident); ok && id.Name == "dialect" {
					switch n.Sel.Name {
					case "Postgres":
						info.Dialects[dialect.Postgres] = true
					case "SQLite":
						info.Dialects[dialect.SQLite] = true
					case "MySQL", "MySQLFlavor":
						info.Dialects[dialect.MySQL] = true
					}
				}
			case *ast.CompositeLit:
				if sel, ok := n.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "Capabilities" {
					info.vectorSeen[vectorsOf(n)] = true
				}
			}
			return true
		})
	}
	switch {
	case len(info.vectorSeen) == 0:
		info.Vectors = "no"
	case len(info.vectorSeen) == 1:
		for v := range info.vectorSeen {
			info.Vectors = v
		}
	default:
		info.Vectors = "varies"
	}
	return info, nil
}

// vectorsOf reads the Vectors field of a Capabilities literal: a constant
// true or false, or "varies" when the driver computes it.
func vectorsOf(lit *ast.CompositeLit) string {
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if k, ok := kv.Key.(*ast.Ident); !ok || k.Name != "Vectors" {
			continue
		}
		if v, ok := kv.Value.(*ast.Ident); ok {
			switch v.Name {
			case "true":
				return "yes"
			case "false":
				return "no"
			}
		}
		return "varies"
	}
	return "no"
}

// driversFor returns the drivers that serve dialect n.
func driversFor(all []driverInfo, n dialect.Name) []driverInfo {
	var out []driverInfo
	for _, d := range all {
		if d.Dialects[n] {
			out = append(out, d)
		}
	}
	return out
}

var migrateImport = regexp.MustCompile(`"github\.com/azrtydxb/momo/migrate/(\w+)"`)

// cliDialects returns the migration dialects the momo command links, read
// from the imports of cmd/momo.
func cliDialects(root string) (map[string]bool, error) {
	files, err := filepath.Glob(filepath.Join(root, "cmd", "momo", "*.go"))
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := readFile(path)
		if err != nil {
			return nil, err
		}
		for _, m := range migrateImport.FindAllStringSubmatch(src, -1) {
			out[m[1]] = true
		}
	}
	return out, nil
}

// hasMigrateDialect reports whether migrate/<name> exists as a package.
func hasMigrateDialect(root, name string) bool {
	files, _ := filepath.Glob(filepath.Join(root, "migrate", name, "*.go"))
	return len(files) > 0
}
