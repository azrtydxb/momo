// SPDX-License-Identifier: Apache-2.0

// Package gosrc holds the small pieces of Go source assembly the API
// generators (graphql, grpc, openapi) share.
package gosrc

import (
	"maps"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/gen"
)

// Imports renders an import block for the packages of candidates (alias to
// import path) whose alias qualifies an identifier in body. A package the
// body does not use is left out, so the generated file never carries an unused
// import. The result ends with a blank line, or is empty when nothing is used.
func Imports(body string, candidates map[string]string) string {
	type imp struct{ alias, path string }
	var used []imp
	for alias, path := range candidates {
		re := regexp.MustCompile(`(^|[^A-Za-z0-9_.])` + regexp.QuoteMeta(alias) + `\.[A-Za-z_]`)
		if re.MatchString(body) {
			used = append(used, imp{alias, path})
		}
	}
	if len(used) == 0 {
		return ""
	}
	sort.Slice(used, func(i, j int) bool {
		si, sj := isStd(used[i].path), isStd(used[j].path)
		if si != sj {
			return si
		}
		return used[i].path < used[j].path
	})
	var b strings.Builder
	b.WriteString("import (\n")
	std := true
	for _, u := range used {
		if std && !isStd(u.path) {
			b.WriteString("\n")
		}
		std = isStd(u.path)
		last := u.path[strings.LastIndex(u.path, "/")+1:]
		if last == u.alias {
			b.WriteString("\t" + strconv.Quote(u.path) + "\n")
		} else {
			b.WriteString("\t" + u.alias + " " + strconv.Quote(u.path) + "\n")
		}
	}
	b.WriteString(")\n\n")
	return b.String()
}

// isStd reports whether path looks like a standard library import path: its
// first element has no dot.
func isStd(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

// ModelImport returns the import path of the package that holds m.
func ModelImport(m *gen.Model) (string, error) {
	if m.ModulePath == "" {
		return m.PkgPath, nil
	}
	return gen.ImportPath(m.ModulePath, m.ModuleDir, m.PkgDir)
}

// DescImport returns the import path of m's descriptor subpackage.
func DescImport(m *gen.Model) (string, error) {
	p, err := ModelImport(m)
	if err != nil {
		return "", err
	}
	return p + "/" + m.SubPkg, nil
}

// SinglePK returns the primary key field of m when the table has exactly one
// primary key column and a field of m maps it, else nil.
func SinglePK(g *gen.Graph, m *gen.Model) *gen.Field {
	if g.Schema == nil {
		return nil
	}
	t, ok := g.Schema.Table(m.Table)
	if !ok || len(t.PrimaryKey) != 1 {
		return nil
	}
	for _, f := range m.Fields {
		if f.Column == t.PrimaryKey[0] {
			return f
		}
	}
	return nil
}

// SortedModels returns the models of g ordered by name, then package path.
func SortedModels(g *gen.Graph) []*gen.Model {
	ms := append([]*gen.Model(nil), g.Models...)
	sort.SliceStable(ms, func(i, j int) bool {
		if ms[i].GoName != ms[j].GoName {
			return ms[i].GoName < ms[j].GoName
		}
		return ms[i].PkgPath < ms[j].PkgPath
	})
	return ms
}

// LowerFirst lower-cases the first letter of s.
func LowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// PkgName matches a Go package name the generators accept.
var PkgName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// BasePath matches the base path of an HTTP API: empty, or segments of
// unreserved characters, each behind a slash and none of them dot-only.
var BasePath = regexp.MustCompile(`^(/[A-Za-z0-9_~-][A-Za-z0-9._~-]*)*$`)

// Aliases allocates import aliases for generated code. It starts from the
// aliases the generator's own code uses, so a model package named like one of
// them gets a numbered alias instead of redeclaring it.
type Aliases struct {
	imports map[string]string // alias -> path
	byPath  map[string]string // path -> alias
}

// NewAliases returns an allocator that already holds fixed (alias to path).
func NewAliases(fixed map[string]string) *Aliases {
	a := &Aliases{imports: maps.Clone(fixed), byPath: map[string]string{}}
	for alias, path := range fixed {
		a.byPath[path] = alias
	}
	return a
}

// Alias returns the alias of path, allocating want (or want with a number
// appended) the first time.
func (a *Aliases) Alias(path, want string) string {
	if alias, ok := a.byPath[path]; ok {
		return alias
	}
	alias := want
	for n := 2; a.imports[alias] != ""; n++ {
		alias = want + strconv.Itoa(n)
	}
	a.imports[alias] = path
	a.byPath[path] = alias
	return alias
}

// Candidates returns every alias with its path, for Imports.
func (a *Aliases) Candidates() map[string]string { return a.imports }
