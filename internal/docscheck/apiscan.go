// SPDX-License-Identifier: Apache-2.0

package docscheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// APIPackages lists, in reference order, the directories whose exported
// identifiers must carry a doc comment and appear in docs/reference.md: the
// core, its public leaf packages, the feature packages split out of it and cli,
// the command line a project builds its own momo binary from.
var APIPackages = []string{
	".", "dialect", "schema", "types", "vector", "migrate", "momolog", "momotest",
	"advisory", "agg", "arrayop", "cli", "fts", "jsonop", "obs", "sqlfn", "timeop", "vecop", "vecsearch",
}

// Ident is one exported identifier of a package.
type Ident struct {
	Name string // "Select", or "Relation.Has" for a method
	Kind string // const, var, type, func, method
	Doc  string // the doc comment text, empty when there is none
}

// Key returns the identifier qualified by its package name, as docs write it.
func (i Ident) Key(pkg string) string { return pkg + "." + i.Name }

// Synopsis returns the first sentence of the doc comment on one line, with
// backticks removed so it cannot start a code span in generated Markdown.
func (i Ident) Synopsis() string {
	para := strings.TrimSpace(strings.SplitN(strings.TrimSpace(i.Doc), "\n\n", 2)[0])
	s := strings.Join(strings.Fields(para), " ")
	for k := 0; k+1 < len(s); k++ {
		if s[k] == '.' && s[k+1] == ' ' {
			s = s[:k+1]
			break
		}
	}
	return strings.ReplaceAll(s, "`", "")
}

// Package is the scanned API of one package directory.
type Package struct {
	Name   string // the Go package name; "momo" for the root
	Dir    string // slash-separated path relative to the module root; "." for the root
	Idents []Ident

	top     map[string]bool
	members map[string]map[string]bool
	embeds  map[string][]string
}

// HasTop reports whether the package declares the exported top-level name.
func (p *Package) HasTop(name string) bool { return p.top[name] }

// HasMember reports whether the type typ has an exported method or field
// name, directly or through a type embedded from the same package.
func (p *Package) HasMember(typ, name string) bool { return p.hasMember(typ, name, 0) }

func (p *Package) hasMember(typ, name string, depth int) bool {
	if depth > 4 {
		return false
	}
	if p.members[typ][name] {
		return true
	}
	for _, e := range p.embeds[typ] {
		if strings.Contains(e, ".") {
			return true // embeds a type of another package: its members cannot be checked here
		}
		if p.hasMember(e, name, depth+1) {
			return true
		}
	}
	return false
}

// ScanFiles builds a Package from the parsed non-test files of one package.
func ScanFiles(dir string, files []*ast.File) *Package {
	p := &Package{
		Dir:     dir,
		top:     map[string]bool{},
		members: map[string]map[string]bool{},
		embeds:  map[string][]string{},
	}
	for _, f := range files {
		p.Name = f.Name.Name
	}
	if dir == "." {
		p.Name = "momo"
	}
	add := func(name, kind, doc string) {
		p.Idents = append(p.Idents, Ident{Name: name, Kind: kind, Doc: doc})
	}
	for _, f := range files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				p.scanFunc(d, add)
			case *ast.GenDecl:
				p.scanGen(d, add)
			}
		}
	}
	return p
}

func (p *Package) addMember(typ, name string) {
	if p.members[typ] == nil {
		p.members[typ] = map[string]bool{}
	}
	p.members[typ][name] = true
}

func (p *Package) scanFunc(d *ast.FuncDecl, add func(name, kind, doc string)) {
	if !d.Name.IsExported() {
		return
	}
	if d.Recv == nil {
		p.top[d.Name.Name] = true
		add(d.Name.Name, "func", d.Doc.Text())
		return
	}
	recv := recvName(d.Recv)
	if recv == "" || !ast.IsExported(recv) {
		return
	}
	p.addMember(recv, d.Name.Name)
	add(recv+"."+d.Name.Name, "method", d.Doc.Text())
}

func (p *Package) scanGen(d *ast.GenDecl, add func(name, kind, doc string)) {
	for _, s := range d.Specs {
		switch s := s.(type) {
		case *ast.TypeSpec:
			if !s.Name.IsExported() {
				continue
			}
			p.top[s.Name.Name] = true
			doc := s.Doc.Text()
			if doc == "" {
				doc = d.Doc.Text()
			}
			add(s.Name.Name, "type", doc)
			p.scanType(s)
			if s.Assign.IsValid() { // an alias has the members of its target
				if e := embeddedName(s.Type); e != "" {
					p.embeds[s.Name.Name] = append(p.embeds[s.Name.Name], e)
				}
			}
		case *ast.ValueSpec:
			kind := "var"
			if d.Tok == token.CONST {
				kind = "const"
			}
			doc := s.Doc.Text()
			if doc == "" {
				doc = d.Doc.Text()
			}
			if doc == "" {
				doc = s.Comment.Text()
			}
			for _, n := range s.Names {
				if n.IsExported() {
					p.top[n.Name] = true
					add(n.Name, kind, doc)
				}
			}
		}
	}
}

// scanType records the fields, interface methods and embedded types of a type.
func (p *Package) scanType(s *ast.TypeSpec) {
	name := s.Name.Name
	var fields *ast.FieldList
	switch t := s.Type.(type) {
	case *ast.StructType:
		fields = t.Fields
	case *ast.InterfaceType:
		fields = t.Methods
	}
	if fields == nil {
		return
	}
	for _, f := range fields.List {
		if len(f.Names) == 0 {
			if e := embeddedName(f.Type); e != "" {
				p.embeds[name] = append(p.embeds[name], e)
			}
			continue
		}
		for _, n := range f.Names {
			if n.IsExported() {
				p.addMember(name, n.Name)
			}
		}
	}
}

func recvName(l *ast.FieldList) string {
	if l == nil || len(l.List) == 0 {
		return ""
	}
	return embeddedName(l.List[0].Type)
}

// embeddedName returns the type name behind *T, T[P] and T.
func embeddedName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return embeddedName(t.X)
	case *ast.IndexExpr:
		return embeddedName(t.X)
	case *ast.IndexListExpr:
		return embeddedName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
	}
	return ""
}

// skipDir reports directories ScanModule does not enter: tooling, test data,
// internal packages, runnable examples and hidden directories.
func skipDir(name string) bool {
	switch name {
	case "internal", "testdata", "vendor", "examples", "bench", "tools", "node_modules", "site", "scripts":
		return true
	}
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// ScanModule scans every importable non-main package of the module at root.
// Nested modules are skipped.
func ScanModule(root string) ([]*Package, error) {
	var out []*Package
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			}
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		p, err := scanDir(path, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		if p != nil {
			out = append(out, p)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out, err
}

func scanDir(path, rel string) (*Package, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(path, n), nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", filepath.ToSlash(filepath.Join(rel, n)), err)
		}
		if f.Name.Name == "main" || strings.HasSuffix(f.Name.Name, "_test") {
			continue
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		return nil, nil
	}
	return ScanFiles(rel, files), nil
}

// ReferencePackages returns the scanned packages of APIPackages, in order.
func ReferencePackages(root string) ([]*Package, error) {
	all, err := ScanModule(root)
	if err != nil {
		return nil, err
	}
	byDir := map[string]*Package{}
	for _, p := range all {
		byDir[p.Dir] = p
	}
	var out []*Package
	for _, d := range APIPackages {
		p := byDir[d]
		if p == nil {
			return nil, fmt.Errorf("API package %q has no Go files", d)
		}
		out = append(out, p)
	}
	return out, nil
}
