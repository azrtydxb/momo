// SPDX-License-Identifier: Apache-2.0

// Command record_ent prints Ent's naming of the fixture models as JSON. It
// reads the exported Table and Field constants of the generated per-entity
// packages with go/parser, so no name is hard-coded here. Run it from the
// internal/namingrec directory.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
)

// entities maps a fixture model name to its generated package directory.
var entities = []struct{ model, dir string }{
	{"UserProfile", "ent/userprofile"},
	{"Tag", "ent/tag"},
	{"Status", "ent/status"},
	{"Person", "ent/person"},
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// consts returns the exported string constants declared in the generated
// package directory.
func consts(dir string) map[string]string {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, 0)
	if err != nil {
		fail(err)
	}
	out := map[string]string{}
	for _, p := range pkgs {
		for _, f := range p.Files {
			for _, d := range f.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok || gd.Tok != token.CONST {
					continue
				}
				for _, s := range gd.Specs {
					vs := s.(*ast.ValueSpec)
					for i, n := range vs.Names {
						if i >= len(vs.Values) || !n.IsExported() {
							continue
						}
						lit, ok := vs.Values[i].(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							continue
						}
						v, err := strconv.Unquote(lit.Value)
						if err != nil {
							fail(err)
						}
						out[n.Name] = v
					}
				}
			}
		}
	}
	return out
}

func main() {
	tables := map[string]string{}
	columns := map[string]string{}
	joins := map[string]string{}
	for _, e := range entities {
		c := consts(e.dir)
		tables[e.model] = c["Table"]
		if e.model != "UserProfile" {
			continue
		}
		for _, f := range []struct{ goName, sym string }{
			{"UserID", "FieldUserID"},
			{"HTTPURL", "FieldHTTPURL"},
			{"CreatedAt", "FieldCreatedAt"},
		} {
			if v, ok := c[f.sym]; ok {
				columns[e.model+"."+f.goName] = v
			}
		}
		// The many-to-many join table of the "tags" edge is the generated
		// TagsTable constant.
		if v, ok := c["TagsTable"]; ok {
			joins[e.model+".Tags"] = v
		}
	}
	out := map[string]any{
		"orm": "ent", "version": "v0.14.6",
		"tables": tables, "columns": columns, "join_tables": joins,
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fail(err)
	}
	fmt.Println(string(b))
}
