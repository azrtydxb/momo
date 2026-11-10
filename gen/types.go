// SPDX-License-Identifier: Apache-2.0

// Package gen implements the momo code generator: it loads model packages
// with full type information, builds a model graph and emits code.
package gen

import (
	"go/token"
	"go/types"
	"strconv"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/schema"
)

// FormatVersion is the generated-file format version.
const FormatVersion = 1

// Config configures Load and later generation stages.
type Config struct {
	// Dir is the directory patterns resolve against; empty means the
	// current directory.
	Dir string
	// Naming derives table and column names; nil means naming.Momo{}.
	Naming naming.Namer
	// TypeDefaults overrides the default SQL column type of a Go type. The
	// only supported key is "time.Time", whose default is timestamptz on
	// PostgreSQL (for example "timestamp(3)" for a Prisma database). An
	// explicit type tag on a field still wins. Other dialects ignore it.
	TypeDefaults map[string]string
	// Dialects limits generated dialect output; nil means all dialects.
	Dialects []dialect.Name
	// Tags are build tags applied while loading packages.
	Tags []string
	// Plugins run after the built-in emitters.
	Plugins []Plugin
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
	Models []*Model
	// Projections are the structs that embed momo.Projection. They have no
	// table, so they are not in Schema; only their MomoMeta is generated.
	Projections []*Model
	Schema      *schema.Schema
	Packages    []*Package
	Plugins     []Plugin
	// Namer is the naming preset the graph was loaded with.
	Namer naming.Namer
}

// Model is a discovered model type.
type Model struct {
	GoName  string
	PkgPath string
	PkgName string
	PkgDir  string
	// ModulePath and ModuleDir come from packages.Module; both are empty
	// when the package belongs to no module.
	ModulePath string
	ModuleDir  string
	File       string
	Pos        token.Position
	Table      string
	Schema     string
	Fields     []*Field
	Relations  []*Relation
	// Indexes are the indexes contributed by tagged fields of embedded
	// structs and mixins, in declaration order.
	Indexes []schema.Index
	// Config is the statically read MomoConfig; never nil for a discovered
	// model.
	Config *CompanionResult
	// SubPkg is the per-model subpackage name.
	SubPkg string
	// Projection is set for a struct that embeds momo.Projection: it has
	// Fields but no Table, Relations, Indexes or companion.
	Projection bool

	named *types.Named
}

// Field is a persisted model column. Task 8 completes it.
type Field struct {
	GoName string
	// Path is the reflect-style index path from the model struct.
	Path   []int
	Column string
	GoType string
	// BaseType is GoType with named types resolved to their underlying basic
	// type (type ID int64 is int64); the types the type map knows by name
	// (time.Time, sql.Null*, vector, types) are kept.
	BaseType string
	Nullable bool
	Tags     tags.Tags
	Pos      token.Position
	Access   Access
	// Base is the embedded-struct prefix of a promoted field, filled in Task 8.
	Base string
}

// Relation is a model relation field. Relation fields are not columns.
// Task 13 discovers relations and fills it; Task 10 reads it to build
// foreign keys and join tables.
type Relation struct {
	GoName string
	Kind   RelKind
	// Target is the related model; nil until the graph is resolved.
	Target *Model
	// FK is the Go name of the foreign-key field: on the declaring model for
	// belongs-to, on the target model for has-one and has-many.
	FK        string
	JoinTable string
	PolyType  string
	PolyValue string
	// OnDelete and OnUpdate are the raw on_delete and on_update tag values.
	OnDelete, OnUpdate string
	// FKName is the raw fk_name tag value: the constraint name, empty for the derived one.
	FKName string
	// NoConstraint is the no_constraint tag: the relation has no database
	// foreign key.
	NoConstraint bool
	Pos          token.Position

	// tags and ftype are kept until resolveRelations has run.
	tags  tags.Tags
	ftype types.Type
}

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

// RelKind classifies a relation.
type RelKind int

// Relation kinds.
const (
	BelongsTo RelKind = iota + 1
	HasOne
	HasMany
	ManyToMany
)
