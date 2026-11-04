// SPDX-License-Identifier: Apache-2.0

// Package schema is Momo's normalized, dialect-aware schema representation.
//
// It is shared by the code generator (which builds it from Go models), the
// migration engine (which diffs, plans and renders it) and database
// inspectors (which build it from a live database). It has no dependencies
// beyond the standard library so it can be linked into application binaries
// through generated metadata at negligible cost.
//
// Identifiers are stored unquoted and case-preserved. Expressions (defaults,
// checks, index predicates) are stored as SQL text; comparisons go through
// Normalize* helpers in normalize.go so inspected and declared forms match.
package schema

import (
	"cmp"
	"slices"
)

// Schema is a set of database objects managed by Momo for one dialect.
type Schema struct {
	Dialect    string      `json:"dialect"`
	Extensions []Extension `json:"extensions,omitempty"`
	Enums      []Enum      `json:"enums,omitempty"`
	Tables     []Table     `json:"tables"`
}

// Extension is a PostgreSQL extension such as "vector".
type Extension struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"` // empty: whatever the server installs
}

// Enum is a PostgreSQL enum type. On SQLite enums are rendered as TEXT
// columns with a CHECK constraint.
type Enum struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// Table is a base table.
type Table struct {
	Name        string       `json:"name"`
	Columns     []Column     `json:"columns"`
	PrimaryKey  []string     `json:"primary_key,omitempty"`
	Uniques     []Unique     `json:"uniques,omitempty"`
	Indexes     []Index      `json:"indexes,omitempty"`
	ForeignKeys []ForeignKey `json:"foreign_keys,omitempty"`
	Checks      []Check      `json:"checks,omitempty"`
	// WithoutRowID marks a SQLite WITHOUT ROWID table.
	WithoutRowID bool `json:"without_rowid,omitempty"`
}

// Column is a table column.
type Column struct {
	Name string `json:"name"`
	// Type is the dialect SQL type as it should be rendered, e.g. "bigint",
	// "vector(1536)", "TEXT". Compare with NormalizeType.
	Type     string `json:"type"`
	Nullable bool   `json:"nullable,omitempty"`
	// Default is a SQL expression, empty when the column has no default.
	Default string `json:"default,omitempty"`
	// Identity is set for generated identity columns: "always" or
	// "by_default" on PostgreSQL; on SQLite an INTEGER PRIMARY KEY rowid
	// alias is reported as "by_default".
	Identity string `json:"identity,omitempty"`
	// Generated is a stored generated-column expression.
	Generated string `json:"generated,omitempty"`
	// Enum names the enum type when Type refers to one (PostgreSQL) or the
	// enum whose values constrain this TEXT column (SQLite).
	Enum string `json:"enum,omitempty"`
}

// Unique is a named UNIQUE constraint.
type Unique struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
}

// Index is a (possibly unique, partial or expression) index.
type Index struct {
	Name   string `json:"name"`
	Unique bool   `json:"unique,omitempty"`
	// Parts are columns or expressions in order.
	Parts []IndexPart `json:"parts"`
	// Method is the access method ("btree", "hnsw", "ivfflat", "gin", ...).
	// Empty means the dialect default (btree).
	Method string `json:"method,omitempty"`
	// Where is a partial-index predicate.
	Where string `json:"where,omitempty"`
	// With holds storage parameters such as m, ef_construction or lists.
	With []KV `json:"with,omitempty"`
	// Concurrently requests CREATE/DROP INDEX CONCURRENTLY (PostgreSQL).
	// It is a migration-time option, not a schema property, and is ignored
	// by comparisons.
	Concurrently bool `json:"concurrently,omitempty"`
}

// IndexPart is one key of an index.
type IndexPart struct {
	Column string `json:"column,omitempty"`
	Expr   string `json:"expr,omitempty"` // set instead of Column for expression keys
	Desc   bool   `json:"desc,omitempty"`
	// Nulls is "first" or "last" when explicitly set.
	Nulls string `json:"nulls,omitempty"`
	// OpClass is an operator class such as "vector_cosine_ops".
	OpClass string `json:"opclass,omitempty"`
}

// KV is an ordered key/value pair.
type KV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ForeignKey is a FOREIGN KEY constraint.
type ForeignKey struct {
	Name       string   `json:"name"`
	Columns    []string `json:"columns"`
	RefTable   string   `json:"ref_table"`
	RefColumns []string `json:"ref_columns"`
	OnDelete   string   `json:"on_delete,omitempty"` // "CASCADE", "SET NULL", ...; empty = NO ACTION
	OnUpdate   string   `json:"on_update,omitempty"`
}

// Check is a CHECK constraint.
type Check struct {
	Name string `json:"name"`
	Expr string `json:"expr"`
}

// Table returns the table with the given name.
func (s *Schema) Table(name string) (*Table, bool) {
	for i := range s.Tables {
		if s.Tables[i].Name == name {
			return &s.Tables[i], true
		}
	}
	return nil, false
}

// Column returns the column with the given name.
func (t *Table) Column(name string) (*Column, bool) {
	for i := range t.Columns {
		if t.Columns[i].Name == name {
			return &t.Columns[i], true
		}
	}
	return nil, false
}

// Sort puts every named collection into a canonical order so that two
// equivalent schemas serialize identically. Column order is preserved: it is
// meaningful (SELECT *, SQLite rebuilds) and part of the model definition.
func (s *Schema) Sort() {
	byName := func(a, b string) int { return cmp.Compare(a, b) }
	slices.SortFunc(s.Extensions, func(a, b Extension) int { return byName(a.Name, b.Name) })
	slices.SortFunc(s.Enums, func(a, b Enum) int { return byName(a.Name, b.Name) })
	slices.SortFunc(s.Tables, func(a, b Table) int { return byName(a.Name, b.Name) })
	for i := range s.Tables {
		t := &s.Tables[i]
		slices.SortFunc(t.Uniques, func(a, b Unique) int { return byName(a.Name, b.Name) })
		slices.SortFunc(t.Indexes, func(a, b Index) int { return byName(a.Name, b.Name) })
		slices.SortFunc(t.ForeignKeys, func(a, b ForeignKey) int { return byName(a.Name, b.Name) })
		slices.SortFunc(t.Checks, func(a, b Check) int { return byName(a.Name, b.Name) })
	}
}
