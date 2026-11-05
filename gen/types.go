// SPDX-License-Identifier: Apache-2.0

// Package gen implements the momo code generator: it loads model packages
// with full type information, builds a model graph and emits code.
package gen

import (
	"go/token"
	"strconv"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/naming"
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

// Model is a discovered model type. Task 7 completes it.
type Model struct {
	GoName  string
	PkgPath string
	PkgName string
	PkgDir  string
	File    string
	Pos     token.Position
	Fields  []*Field
}

// Field is a model field. Task 8 completes it.
type Field struct {
	GoName string
	Column string
}

// Relation is a model relation. Task 10 completes it.
type Relation struct{}

// Kind classifies a field's storage kind. Task 8 completes it.
type Kind int

// Access describes how generated code reaches a field. Task 8 completes it.
type Access int

// RelKind classifies a relation. Task 10 completes it.
type RelKind int
