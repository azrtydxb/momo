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
	SQL string
	// NoTx marks a statement that must run outside a transaction (CREATE INDEX
	// CONCURRENTLY) or should not hold the locks of the transactional DDL
	// before it (VALIDATE CONSTRAINT). Engine.Diff writes such statements to a
	// trailing no-transaction migration, apart from the transactional ones.
	NoTx bool
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
	// Risky marks a change that is not destructive but can take long or heavy
	// locks, scan the table, or fail on existing data: dropping an index or
	// constraint, adding a unique or check constraint. Diff reports each as a
	// "risky:" hint; it never needs --allow-destructive.
	Risky bool
	// Partitioned is set on a change to a partitioned table. PostgreSQL rejects
	// CREATE INDEX CONCURRENTLY there, so the renderer avoids it.
	Partitioned bool
	// IfExists asks a drop to be idempotent (DROP ... IF EXISTS): Diff sets it
	// when it cannot tell whether a column dropped in the same diff already
	// took the object with it.
	IfExists bool
	Rebuild  bool // SQLite: requires 12-step rebuild
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
	// Next is the trailing no-transaction migration that Engine.Diff splits off
	// when the diff holds statements that must run outside a transaction next
	// to transactional ones; nil otherwise. Files of the first migration also
	// hold Next's files, so writing Files writes the pair.
	Next     *Migration
	Changes  []Change
	Snapshot *schema.Schema
	Files    map[string][]byte // relative file name -> content, for the CLI to write
	Hints    []string
}

// Pending is a migration that has not been applied yet.
type Pending struct {
	Version    string
	Name       string
	Kind       Kind
	Statements []Statement
	NoTx       bool
	Checksum   string // hex sha256 of the up file, recorded in the history row
	// ForeignKeysOff is the -- momo:sqlite-foreign-keys-off directive: Apply
	// turns foreign-key enforcement off around this migration on SQLite.
	ForeignKeysOff bool
	// Go is the migration to run when Kind is "go"; nil for SQL.
	Go *GoMigration
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
	To       string        // empty: all pending
	LockWait time.Duration // default 30s
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
	// not compare. Drift compares that text when it can canonicalize the
	// snapshot (PostgreSQL with CREATE on the database and a writable session):
	// a changed body is then in Changed and this list is empty. When it cannot,
	// and DriftOptions.Strict is off, the objects are listed here and a body
	// changed outside the migrations does not appear in Changed. An empty list
	// means no object was left uncompared, and Clean ignores it.
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
	// EnforceNotNull emits SET NOT NULL for every existing column that the
	// snapshot has as nullable and the model has as NOT NULL without a
	// default: the follow-up to a staged change, run after the backfill. It
	// does not know which columns were staged, so a column the user made
	// NOT NULL by hand-editing the model is enforced too. New NOT NULL
	// columns stay refused (ErrStagedRequired). Confirm no NULLs remain
	// first, or the migration fails when applied.
	EnforceNotNull bool
	Name           string // not used; name is passed to Diff
}

// Config is the engine configuration.
type Config struct {
	// Schemas lists the PostgreSQL schemas the engine manages. Default (nil or empty): the single schema
	// returned by `SELECT current_schema()` on the pinned connection, resolved when the engine starts
	// (Plan/Apply/Status/Diff/Drift/Inspect each resolve it once per call). An app with search_path=app
	// therefore migrates `app` by default. Schemas[0] must equal current_schema(), the schema in which a new
	// history table is created (an existing one is found through the search_path); otherwise the engine returns a configuration error naming both.
	Schemas []string
	// LockTimeout is the PostgreSQL lock_timeout of the migration session;
	// zero keeps the 5s default. WithLockTimeout(0) disables it.
	LockTimeout time.Duration
	// StatementTimeout is the PostgreSQL statement_timeout of the migration
	// session; zero leaves the server setting. WithStatementTimeout(0) sets it
	// to 0 explicitly.
	StatementTimeout time.Duration
	Exclude          []string // glob patterns for unmanaged objects (drift)
	// OnlyTables limits the managed tables to those whose name (or
	// schema.name) matches a glob: every other table, with its triggers and
	// policies, is hidden like an Exclude match. Empty manages every table.
	OnlyTables []string
	// ExternalExtensions names extensions that are created outside momo, for
	// example by a superuser. Diff and AutoMigrate never emit CREATE EXTENSION
	// for them and leave them out of the snapshot; drift, baseline and Inspect
	// do not report them. Exact names, compared case-sensitively.
	ExternalExtensions []string
}

// SessionOptions are the per-session timeouts a Dialect applies in Setup.
//
// A timeout above zero is applied. A zero timeout leaves the setting alone
// unless its Set field is true, in which case the dialect sets it to zero
// (PostgreSQL treats 0 as off).
//
// ForeignKeysOff asks a SQLite session to turn foreign-key enforcement off,
// which a table rebuild needs; Setup must then run outside any transaction and
// the returned reset restores the setting. Other dialects ignore it.
type SessionOptions struct {
	LockTimeout, StatementTimeout       time.Duration
	LockTimeoutSet, StatementTimeoutSet bool
	ForeignKeysOff                      bool
}
