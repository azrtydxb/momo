// SPDX-License-Identifier: Apache-2.0

// Package tags defines the parsed form of the momo struct tag: the Tags value
// that gen.Field carries in its Tags field. The tag grammar and the option
// vocabulary are described in docs/ai-guide.md; the parser lives in an
// internal package of this module.
package tags

// Tags is the parsed form of one `momo` struct tag.
type Tags struct {
	Skip bool

	Column, Type, Collate, Default, Check, Comment, Enum, Table, Schema, Prefix       string
	Fk, FkName, Join, OnDelete, OnUpdate, SoftDelete, TimeUnit, IndexName, UniqueName string
	Sort, Nulls, Method, View                                                         string

	// Generated is the generated option: the expression of a stored generated
	// column (GENERATED ALWAYS AS (expr) STORED). It needs read_only.
	Generated string

	// GeneratedVirtual is the virtual option: the generated column is VIRTUAL
	// (computed on read) instead of STORED. It is valid only together with
	// generated (or the MomoConfig Generated hooks), which BuildSchema checks.
	GeneratedVirtual bool

	// IDGen is the idgen option: the name of a Go-side id generator.
	IDGen string

	// EmbedFrom is the embed_from option on a vector field: the source
	// columns or fields of its embedding, as one value joined by "|". It is
	// split and resolved by BuildSchema.
	EmbedFrom string

	Materialized bool

	Rel, PolyType, PolyValue string

	Size, Precision, Scale, Dim int

	// IdentityAlways is the identity:always option: GENERATED ALWAYS AS
	// IDENTITY. It implies Identity.
	IdentityAlways bool

	PK, NotNull, Nullable, Identity, Serial, Version, CreatedAt, UpdatedAt bool
	ReadOnly, CreateOnly, Immutable, Sensitive, Embedded                   bool
	NoConstraint, BelongsTo, HasOne, HasMany, ManyToMany                   bool
	Index, Unique, Search                                                  bool

	// DBDefault is the dbdefault option: a Model insert leaves the column to
	// the database default while the field holds its zero value.
	DBDefault bool

	// Autoincrement, WithoutRowID and Strict are the SQLite table options of
	// the momo.Model marker: the INTEGER PRIMARY KEY is declared AUTOINCREMENT,
	// the table is WITHOUT ROWID, the table is STRICT. They mean nothing on a
	// field.
	Autoincrement, WithoutRowID, Strict bool
}
