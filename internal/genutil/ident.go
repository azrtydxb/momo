// SPDX-License-Identifier: Apache-2.0

// Package genutil holds the small pure helpers the generator packages share:
// identifier choice for generated code, module-relative import paths, the
// time type allowlist and the generated-file header.
package genutil

import (
	"go/types"
	"strings"
)

// goKeywords are the 25 Go keywords.
var goKeywords = []string{
	"break", "default", "func", "interface", "select",
	"case", "defer", "go", "map", "struct",
	"chan", "else", "goto", "package", "switch",
	"const", "fallthrough", "if", "range", "type",
	"continue", "for", "import", "return", "var",
}

// helperNames are names the generated package itself declares or imports.
var helperNames = []string{
	"Table", "As", "Columns", "ColumnByName", "ColumnNames",
	"base", "fmt", "momo", "time", "vector", "types",
}

func isKeyword(s string) bool {
	for _, k := range goKeywords {
		if k == s {
			return true
		}
	}
	return false
}

// Identifier returns goName unless it is in reserved; otherwise it appends
// "_" until the result is not in reserved. The caller adds the result to the
// set it passes on.
func Identifier(goName string, reserved map[string]bool) string {
	for reserved[goName] {
		goName += "_"
	}
	return goName
}

// DescriptorReserved returns the names a generated descriptor type must not
// use for its own identifiers: helper names, Go keywords and every
// predeclared identifier. The returned map is fresh and may be modified.
func DescriptorReserved() map[string]bool {
	r := make(map[string]bool, 64)
	for _, n := range helperNames {
		r[n] = true
	}
	for _, k := range goKeywords {
		r[k] = true
	}
	for _, n := range types.Universe.Names() {
		r[n] = true
	}
	return r
}

// SubPkgName returns the lower-cased model name used as a generated
// subpackage name, with "_" appended when it is a Go keyword, a predeclared
// name, or equal to modelPkgName.
func SubPkgName(model, modelPkgName string) string {
	n := strings.ToLower(model)
	if isKeyword(n) || types.Universe.Lookup(n) != nil || n == modelPkgName {
		n += "_"
	}
	return n
}
