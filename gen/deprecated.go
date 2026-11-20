// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/internal/genutil"
)

// HeaderPrefix starts the first line of every generated file.
//
// Deprecated: not part of the plugin contract; use Header for the whole line.
// Removed in the next release.
const HeaderPrefix = genutil.HeaderPrefix

// ParseHeader reports whether line is a momo generated-file header and returns
// its format number.
//
// Deprecated: not part of the plugin contract. Removed in the next release.
func ParseHeader(line string) (format int, ok bool) { return genutil.ParseHeader(line) }

// Identifier returns goName, with "_" appended until it is not in reserved.
//
// Deprecated: not part of the plugin contract. Removed in the next release.
func Identifier(goName string, reserved map[string]bool) string {
	return genutil.Identifier(goName, reserved)
}

// DescriptorReserved returns the names a generated descriptor type must not
// use for its own identifiers.
//
// Deprecated: use DescriptorIdents to get the names of the descriptors.
// Removed in the next release.
func DescriptorReserved() map[string]bool { return genutil.DescriptorReserved() }

// SubPkgName returns the lower-cased model name used as a generated
// subpackage name.
//
// Deprecated: read Model.SubPkg. Removed in the next release.
func SubPkgName(model, modelPkgName string) string { return genutil.SubPkgName(model, modelPkgName) }

// ImportPath returns the slash-separated Go import path of the package in
// pkgDir.
//
// Deprecated: not part of the plugin contract. Removed in the next release.
func ImportPath(modulePath, moduleDir, pkgDir string) (string, error) {
	return genutil.ImportPath(modulePath, moduleDir, pkgDir)
}

// ValidTimeType reports whether s is an accepted value for the time.Time entry
// of Config.TypeDefaults.
//
// Deprecated: not part of the plugin contract. Removed in the next release.
func ValidTimeType(s string) bool { return genutil.ValidTimeType(s) }

// EmitLookup returns the source of the allowlisted ColumnByName function.
//
// Deprecated: Emit runs the emitters. Removed in the next release.
func EmitLookup(m *Model, g *Graph) ([]byte, []Diagnostic) { return emitLookup(m, g) }

// EmitMeta returns the source of the generated Meta declarations.
//
// Deprecated: Emit runs the emitters. Removed in the next release.
func EmitMeta(models []*Model, g *Graph) ([]byte, []Diagnostic) { return emitMeta(models, g) }

// EmitDescriptors returns the source of the per-model descriptor subpackage.
//
// Deprecated: Emit runs the emitters. Removed in the next release.
func EmitDescriptors(m *Model, g *Graph) ([]byte, []Diagnostic) { return emitDescriptors(m, g) }

// DescriptorType returns the momo descriptor type and constructor names for
// an access mode.
//
// Deprecated: not part of the plugin contract. Removed in the next release.
func DescriptorType(a Access, pointer bool) (typeName, ctor string) {
	return descriptorType(a, pointer)
}

// VectorDescriptorType returns the momo vector descriptor type and
// constructor names for an access mode.
//
// Deprecated: not part of the plugin contract. Removed in the next release.
func VectorDescriptorType(a Access, pointer bool) (typeName, ctor string) {
	return vectorDescriptorType(a, pointer)
}

// AccessFor derives a field's write access from its tags.
//
// Deprecated: read Field.Access. Removed in the next release.
func AccessFor(t tags.Tags, field string, pointer bool) (Access, []Diagnostic) {
	return accessFor(t, field, pointer)
}

// DetectHooks finds the hook methods of a model statically.
//
// Deprecated: read Model.Hooks. Removed in the next release.
func DetectHooks(named *types.Named, fset *token.FileSet) (map[string]bool, []Diagnostic) {
	return detectHooks(named, fset)
}

// ReadCompanion reads the companion declarations of a model.
//
// Deprecated: read Model.Config. Removed in the next release.
func ReadCompanion(pkg *packages.Package, m *Model) (*CompanionResult, []Diagnostic) {
	return readCompanion(pkg, m)
}
