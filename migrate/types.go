// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"time"

	"github.com/azrtydxb/momo/schema"
)

// Kind is the migration kind: "sql" or "go".
type Kind string

// Statement is one SQL statement of a migration.
type Statement struct {
	SQL  string
	NoTx bool // statement must run outside a transaction (CREATE INDEX CONCURRENTLY)
	Hint string
}

// ChangeKind names the kind of a schema change.
type ChangeKind string

// Change kinds produced by Diff.
const (
	CreateSchema      ChangeKind = "create_schema"
	CreateExtension   ChangeKind = "create_extension"
	CreateEnum        ChangeKind = "create_enum"
	AlterEnumAddValue ChangeKind = "alter_enum_add_value"
	DropEnum          ChangeKind = "drop_enum"
	CreateTable       ChangeKind = "create_table"
	DropTable         ChangeKind = "drop_table"
	RenameTable       ChangeKind = "rename_table"
	AddColumn         ChangeKind = "add_column"
	DropColumn        ChangeKind = "drop_column"
	RenameColumn      ChangeKind = "rename_column"
	AlterColumn       ChangeKind = "alter_column"
	AddPrimaryKey     ChangeKind = "add_primary_key"
	DropPrimaryKey    ChangeKind = "drop_primary_key"
	AddUnique         ChangeKind = "add_unique"
	DropUnique        ChangeKind = "drop_unique"
	AddCheck          ChangeKind = "add_check"
	DropCheck         ChangeKind = "drop_check"
	AddFK             ChangeKind = "add_fk"
	DropFK            ChangeKind = "drop_fk"
	CreateIndex       ChangeKind = "create_index"
	DropIndex         ChangeKind = "drop_index"
	SetRLS            ChangeKind = "set_rls"
	CreatePolicy      ChangeKind = "create_policy"
	DropPolicy        ChangeKind = "drop_policy"
	CreateView        ChangeKind = "create_view"
	DropView          ChangeKind = "drop_view"
	CreateFunction    ChangeKind = "create_function"
	ReplaceFunction   ChangeKind = "replace_function"
	DropFunction      ChangeKind = "drop_function"
	CreateTrigger     ChangeKind = "create_trigger"
	DropTrigger       ChangeKind = "drop_trigger"
)

// Change is one schema change in a plan.
type Change struct {
	Kind        ChangeKind
	Schema      string
	Table       string
	Name        string // object name, or new name for renames
	Before      any    // *schema.Column, *schema.Index, *schema.View, ... (kind-specific)
	After       any
	Destructive bool
	Rebuild     bool // SQLite: requires 12-step rebuild
}

// Migration is one migration version.
type Migration struct {
	Version     string // 14-digit UTC timestamp
	Name        string
	Kind        Kind
	Parent      string
	Up          []Statement
	Down        []Statement
	NoTx        bool
	Destructive bool
	Changes     []Change
	Snapshot    *schema.Schema
	Files       map[string][]byte // relative file name -> content, for the CLI to write
	Hints       []string
}

// Pending is a migration that has not been applied yet.
type Pending struct {
	Version    string
	Name       string
	Kind       Kind
	Statements []Statement
	NoTx       bool
	Checksum   string // hex sha256 of the up file, recorded in the history row
}

// State is the status of one migration version: "pending", "applied", "dirty", "mismatch" or "missing".
type State string

// VersionStatus reports the state of one migration version.
type VersionStatus struct {
	Version, Name string
	State         State
	Checksum      string
	Error         string
}

// Applied reports one migration applied by an Apply call.
type Applied struct {
	Version    string
	Statements int
	Duration   time.Duration
}

// Report is the result of an Apply call.
type Report struct {
	Applied []Applied
	ApplyID string
}

// ApplyOptions controls Apply.
type ApplyOptions struct {
	To          string // empty: all pending
	ForceUnlock bool
	LockWait    time.Duration // default 30s
}

// ResolveState is the state a dirty migration is resolved to: "applied" or "rolled_back".
type ResolveState string

// Resolve states for Engine.Resolve.
const (
	ResolveApplied    ResolveState = "applied"
	ResolveRolledBack ResolveState = "rolled_back"
)

// Severity is a lint severity: "error" or "warning".
type Severity string

// Finding is one lint finding.
type Finding struct {
	Severity    Severity
	Rule        string
	Version     string
	Statement   int
	Message     string
	Alternative string
}

// ObjectRef names a database object.
type ObjectRef struct{ Kind, Schema, Name string }

// ObjectDiff describes how one object differs.
type ObjectDiff struct {
	Object ObjectRef
	Detail string
}

// DriftReport lists the differences between the live database and the expected schema.
type DriftReport struct {
	Changed   []ObjectDiff
	Missing   []ObjectRef
	Unmanaged []ObjectRef
	// Unverified lists the views, triggers and policies present on both sides
	// whose text (view query, trigger condition, policy expressions) Drift did
	// not compare, because that needs DDL to canonicalize. A body changed
	// outside the migrations does not appear in Changed. An empty list does not
	// mean those bodies were verified, and Clean ignores it.
	Unverified []ObjectRef
	// CanonicalizeSkipped lists the tables whose declared text (check
	// expressions, column defaults, generation expressions, index predicates
	// and expressions) Drift compared as declared, because it could not
	// rewrite it to the form the database reports: the role lacks CREATE on
	// the database, the session is read-only, or the scratch copy of that
	// table failed. Skipping can only add false differences (a table shown
	// in Changed that matches the database), never hide a real one, because
	// the declared text is then compared against the stored text as it is. So
	// Clean ignores it, unlike Unverified, whose bodies are not compared at
	// all and so can hide a real difference.
	CanonicalizeSkipped []ObjectRef
}

// Clean reports whether there is no drift. It ignores Unverified, which lists
// text that was not compared and so can hide a difference, and
// CanonicalizeSkipped, which can only add false differences to Changed and
// never hide a real one.
func (d *DriftReport) Clean() bool {
	return len(d.Changed) == 0 && len(d.Missing) == 0 && len(d.Unmanaged) == 0
}

// Skipped is a change AutoMigrate did not apply, with the reason.
type Skipped struct {
	Change Change
	Reason string
}

// AutoReport is the result of an automatic migration.
type AutoReport struct {
	Applied []Change
	Skipped []Skipped
}

// DiffOptions controls Diff.
type DiffOptions struct {
	RenameTables     map[string]string // old -> new
	RenameColumns    map[string]string // "table.old" -> "new"
	AllowDestructive bool
	Stage            bool
	Name             string // not used; name is passed to Diff
}

// Config is the engine configuration.
type Config struct {
	// Schemas lists the PostgreSQL schemas the engine manages. Default (nil or empty): the single schema
	// returned by `SELECT current_schema()` on the pinned connection, resolved when the engine starts
	// (Plan/Apply/Status/Diff/Drift/Inspect each resolve it once per call). An app with search_path=app
	// therefore migrates `app` by default. Schemas[0] hosts the history table and must equal
	// current_schema(); otherwise the engine returns a configuration error naming both.
	Schemas          []string
	LockTimeout      time.Duration // default 5s, PostgreSQL only
	StatementTimeout time.Duration // default 0 (unset)
	Exclude          []string      // glob patterns for unmanaged objects (drift)
}

// SessionOptions are the per-session timeouts a Dialect applies in Setup.
type SessionOptions struct {
	LockTimeout, StatementTimeout time.Duration
}
