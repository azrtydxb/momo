// SPDX-License-Identifier: Apache-2.0

// Package gen implements the momo code generator: it loads model packages
// with full type information, builds a model graph and emits code.
package gen

import (
	"go/token"
	"strconv"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/schema"
)

// Version is the generator version written into generated-file headers.
const Version = "v0.1.0-dev"

// FormatVersion is the generated-file format version.
const FormatVersion = 1

// Config configures Load and later generation stages.
type Config struct {
	// Dir is the directory patterns resolve against; empty means the
	// current directory.
	Dir string
	// Naming derives table and column names; nil means naming.Momo{}.
	Naming naming.Namer
	// Dialects limits generated dialect output; nil means all dialects.
	Dialects []dialect.Name
	// Tags are build tags applied while loading packages.
	Tags []string
	// Plugins run after the built-in emitters.
	Plugins []Plugin
}

// Plugin extends generation. Task 20 completes the plugin contract.
type Plugin interface {
	Name() string
	Generate(g *Graph, w *Writer) error
}

// Diagnostic is a positioned problem found while loading or analysing input.
type Diagnostic struct {
	Pos token.Position
	Msg string
}

// String returns the diagnostic as file:line:col: msg.
func (d Diagnostic) String() string {
	return d.Pos.Filename + ":" + strconv.Itoa(d.Pos.Line) + ":" + strconv.Itoa(d.Pos.Column) + ": " + d.Msg
}

// Package describes a loaded Go package.
type Package struct {
	Path, Name, Dir string
}

// Graph is the loaded model graph.
type Graph struct {
	Models   []*Model
	Schema   *schema.Schema
	Packages []*Package
	Plugins  []Plugin
}

// Model is a discovered model type.
type Model struct {
	GoName    string
	PkgPath   string
	PkgName   string
	PkgDir    string
	File      string
	Pos       token.Position
	Table     string
	Schema    string
	Fields    []*Field
	Relations []*Relation
	// Indexes are the indexes contributed by tagged fields of embedded
	// structs and mixins, in declaration order.
	Indexes []schema.Index
	// Config is the statically read MomoConfig; never nil for a discovered
	// model.
	Config *CompanionResult
	// SubPkg is the per-model subpackage name, set in Task 13.
	SubPkg string
}

// Field is a persisted model column. Task 8 completes it.
type Field struct {
	GoName string
	// Path is the reflect-style index path from the model struct.
	Path     []int
	Column   string
	GoType   string
	Nullable bool
	Tags     tags.Tags
	Pos      token.Position
	Access   Access
	// Base is the embedded-struct prefix of a promoted field, filled in Task 8.
	Base string
}

// Relation is a model relation. Task 10 completes it.
type Relation struct{}

// Kind classifies a field's storage kind. Task 8 completes it.
type Kind int

// Access describes how generated code reaches a field. Task 12 fills the
// rules.
type Access int

// Field access modes.
const (
	Writable Access = iota
	ReadOnly
	CreateOnly
	Immutable
)

// RelKind classifies a relation. Task 10 completes it.
type RelKind int
