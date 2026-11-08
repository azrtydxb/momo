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
	"encoding/json"
	"io"
	"slices"

	"github.com/azrtydxb/momo/dialect"
)

// FormatVersion is the version of the schema snapshot JSON format. A newer
// Momo reads every older format; an older Momo refuses a newer one.
const FormatVersion = 1

// Schema is a set of database objects managed by Momo for one dialect.
type Schema struct {
	FormatVersion int          `json:"format_version"`
	Dialect       dialect.Name `json:"dialect"`
	// Schemas lists PostgreSQL namespaces other than the connection's
	// current schema, which is implicit.
	Schemas    []SchemaNamespace `json:"schemas,omitempty"`
	Extensions []Extension       `json:"extensions,omitempty"`
	Enums      []Enum            `json:"enums,omitempty"`
	Tables     []Table           `json:"tables"`
	Views      []View            `json:"views,omitempty"`
	Functions  []Function        `json:"functions,omitempty"`
	Triggers   []Trigger         `json:"triggers,omitempty"`
	Policies   []Policy          `json:"policies,omitempty"`
}

// SchemaNamespace is a PostgreSQL schema (namespace) managed by Momo.
type SchemaNamespace struct {
	Name string `json:"name"`
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
	// Schema is the PostgreSQL namespace. Empty means the connection's
	// current_schema() (PostgreSQL) or main (SQLite).
	Schema      string       `json:"schema,omitempty"`
	Name        string       `json:"name"`
	Columns     []Column     `json:"columns"`
	PrimaryKey  []string     `json:"primary_key,omitempty"`
	Uniques     []Unique     `json:"uniques,omitempty"`
	Indexes     []Index      `json:"indexes,omitempty"`
	ForeignKeys []ForeignKey `json:"foreign_keys,omitempty"`
	Checks      []Check      `json:"checks,omitempty"`
	// WithoutRowID marks a SQLite WITHOUT ROWID table.
	WithoutRowID bool `json:"without_rowid,omitempty"`
	// Strict marks a SQLite STRICT table (column types are enforced).
	Strict bool `json:"strict,omitempty"`
	// Autoincrement marks a SQLite table whose INTEGER PRIMARY KEY is
	// declared AUTOINCREMENT (row ids are never reused).
	Autoincrement bool `json:"autoincrement,omitempty"`
	// RLS is nil when row-level security is off.
	RLS *RLS `json:"rls,omitempty"`
	// Partition is set on the parent of a partitioned table.
	Partition *Partition `json:"partition,omitempty"`
	// PartitionOf is set on a child partition.
	PartitionOf *PartitionOf `json:"partition_of,omitempty"`
}

// RLS holds a table's row-level security flags (PostgreSQL).
type RLS struct {
	Enabled bool `json:"enabled"`
	Forced  bool `json:"forced"`
}

// Partition describes the partitioning of a parent table.
type Partition struct {
	Strategy string `json:"strategy"` // "range", "list", "hash"
	Key      string `json:"key"`      // pg_get_partkeydef text, e.g. "RANGE (created_at)"
}

// PartitionOf describes a child partition.
type PartitionOf struct {
	Parent string `json:"parent"`
	Bound  string `json:"bound"` // e.g. "FOR VALUES FROM ('2026-01-01') TO ('2027-01-01')"
}

// View is a view or materialized view.
type View struct {
	Schema       string `json:"schema,omitempty"`
	Name         string `json:"name"`
	Materialized bool   `json:"materialized,omitempty"`
	Query        string `json:"query"` // SELECT text only, no CREATE VIEW prefix
	// Depends lists the tables and views this view reads: the bare name
	// when in the current schema, otherwise "schema.name".
	Depends []string `json:"depends,omitempty"`
	// Indexes are set on materialized views only.
	Indexes []Index `json:"indexes,omitempty"`
}

// Function is a PostgreSQL function.
type Function struct {
	Schema     string `json:"schema,omitempty"`
	Name       string `json:"name"`
	Args       string `json:"args"`       // pg_get_function_arguments text
	Returns    string `json:"returns"`    // pg_get_function_result text
	Language   string `json:"language"`   // "plpgsql", "sql", ...
	Volatility string `json:"volatility"` // "VOLATILE", "STABLE", "IMMUTABLE"
	Body       string `json:"body"`       // rendered with dollar-quote tag $momo$
	// Strict is RETURNS NULL ON NULL INPUT; SecurityDefiner is SECURITY DEFINER.
	Strict          bool `json:"strict,omitempty"`
	SecurityDefiner bool `json:"security_definer,omitempty"`
}

// Trigger is a table trigger.
type Trigger struct {
	Schema     string   `json:"schema,omitempty"`
	Table      string   `json:"table"`
	Name       string   `json:"name"`
	Timing     string   `json:"timing"`           // "BEFORE", "AFTER", "INSTEAD OF"; SQLite uses Definition
	Events     []string `json:"events,omitempty"` // "INSERT", "UPDATE", "DELETE", "TRUNCATE"
	ForEachRow bool     `json:"for_each_row,omitempty"`
	When       string   `json:"when,omitempty"`
	Function   string   `json:"function,omitempty"` // qualified function name
	Args       []string `json:"args,omitempty"`
	// Definition is SQLite only: the CREATE TRIGGER text from sqlite_schema.
	Definition string `json:"definition,omitempty"`
}

// Policy is a row-level security policy.
type Policy struct {
	Schema     string   `json:"schema,omitempty"`
	Table      string   `json:"table"`
	Name       string   `json:"name"`
	Command    string   `json:"command"` // "ALL", "SELECT", "INSERT", "UPDATE", "DELETE"
	Permissive bool     `json:"permissive"`
	Roles      []string `json:"roles,omitempty"` // empty means PUBLIC
	Using      string   `json:"using,omitempty"`
	Check      string   `json:"check,omitempty"`
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
	// alias is reported as "by_default". "serial" marks a PostgreSQL
	// serial column: Type stays the plain integer type and the column's
	// DEFAULT is nextval of a sequence it owns, which is rendered as
	// smallserial, serial or bigserial and never appears in Default.
	Identity string `json:"identity,omitempty"`
	// Generated is a stored generated-column expression.
	Generated string `json:"generated,omitempty"`
	// Enum names the enum type when Type refers to one (PostgreSQL) or the
	// enum whose values constrain this TEXT column (SQLite).
	Enum string `json:"enum,omitempty"`
	// Collate is the column collation; empty means the database default.
	Collate string `json:"collate,omitempty"`
	// Comment is the column comment (PostgreSQL COMMENT ON COLUMN).
	Comment string `json:"comment,omitempty"`
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
	byKey := func(a, b []string) int { return slices.Compare(a, b) }
	slices.SortFunc(s.Schemas, func(a, b SchemaNamespace) int { return byName(a.Name, b.Name) })
	slices.SortFunc(s.Extensions, func(a, b Extension) int { return byName(a.Name, b.Name) })
	slices.SortFunc(s.Enums, func(a, b Enum) int { return byName(a.Name, b.Name) })
	slices.SortFunc(s.Tables, func(a, b Table) int { return byKey([]string{a.Schema, a.Name}, []string{b.Schema, b.Name}) })
	slices.SortFunc(s.Views, func(a, b View) int { return byKey([]string{a.Schema, a.Name}, []string{b.Schema, b.Name}) })
	slices.SortFunc(s.Functions, func(a, b Function) int {
		return byKey([]string{a.Schema, a.Name, a.Args}, []string{b.Schema, b.Name, b.Args})
	})
	slices.SortFunc(s.Triggers, func(a, b Trigger) int {
		return byKey([]string{a.Schema, a.Table, a.Name}, []string{b.Schema, b.Table, b.Name})
	})
	slices.SortFunc(s.Policies, func(a, b Policy) int {
		return byKey([]string{a.Schema, a.Table, a.Name}, []string{b.Schema, b.Table, b.Name})
	})
	for i := range s.Triggers {
		s.Triggers[i].Events = sortedSet(s.Triggers[i].Events)
	}
	for i := range s.Policies {
		s.Policies[i].Roles = sortedSet(s.Policies[i].Roles)
	}
	for i := range s.Views {
		v := &s.Views[i]
		v.Depends = sortedSet(v.Depends)
		slices.SortFunc(v.Indexes, func(a, b Index) int { return byName(a.Name, b.Name) })
	}
	for i := range s.Tables {
		t := &s.Tables[i]
		slices.SortFunc(t.Uniques, func(a, b Unique) int { return byName(a.Name, b.Name) })
		slices.SortFunc(t.Indexes, func(a, b Index) int { return byName(a.Name, b.Name) })
		slices.SortFunc(t.ForeignKeys, func(a, b ForeignKey) int { return byName(a.Name, b.Name) })
		slices.SortFunc(t.Checks, func(a, b Check) int { return byName(a.Name, b.Name) })
	}
}

// sortedSet sorts and deduplicates in place; nil and empty stay as they are.
func sortedSet(in []string) []string {
	if len(in) == 0 {
		return in
	}
	slices.Sort(in)
	return slices.Compact(in)
}

// Write encodes the schema as an indented JSON snapshot with a trailing
// newline, stamping the current FormatVersion.
func (s *Schema) Write(w io.Writer) error {
	s.FormatVersion = FormatVersion
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}
