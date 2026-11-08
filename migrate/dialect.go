// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// InspectOptions selects what Inspect reads.
type InspectOptions struct {
	Schemas []string
	Exclude []string
}

// Dialect is the per-database migration backend.
type Dialect interface {
	Dialect() *dialect.Dialect
	Inspect(ctx context.Context, c momo.PinnedConn, opts InspectOptions) (*schema.Schema, error)
	Render(plan []Change) ([]Statement, error)
	HistoryDDL() []string
	Setup(ctx context.Context, c momo.PinnedConn, s SessionOptions) (reset func() error, err error)
	Lock(ctx context.Context, c momo.PinnedConn, key string) (unlock func() error, holder func(context.Context) (string, error), err error)
}

// SchemaValidator is implemented by dialects that refuse some declarations
// their database cannot keep apart: ValidateSchema checks a desired schema, and
// Diff and AutoMigrate return its error before anything is rendered.
type SchemaValidator interface {
	ValidateSchema(s *schema.Schema) error
}

// InvalidInspector is implemented by dialects whose Inspect hides objects left
// in an invalid state. InspectWithInvalid is Inspect plus the names of the
// invalid indexes (a failed CREATE INDEX CONCURRENTLY leaves one behind), spelled
// bare in the current schema and "schema.index" elsewhere. Drift and Baseline
// use it in place of Inspect when the dialect has it.
type InvalidInspector interface {
	InspectWithInvalid(ctx context.Context, c momo.PinnedConn, opts InspectOptions) (*schema.Schema, []string, error)
}

// Canonicalizer is implemented by dialects whose database rewrites declared
// text. Canonicalize replaces, in s, the declared text of views, function
// bodies, and policy and trigger expressions with the text the database itself
// reports, so that s compares equal to an inspection of the live objects. It
// leaves nothing behind in the database. Baseline calls it on the snapshot it
// verifies. When the connected role lacks a privilege Canonicalize needs, it
// returns an error wrapping ErrCanonicalizePrivilege that names the privilege;
// it must not skip the rewrite silently.
type Canonicalizer interface {
	Canonicalize(ctx context.Context, c momo.PinnedConn, s *schema.Schema) error
}

// TableCanonicalizer is implemented by dialects that can canonicalize the table
// text of a schema (check expressions, column defaults, generation expressions,
// index predicates and expressions) best effort. CanonicalizeTables rewrites
// that text in s and leaves every other object as declared. A table it cannot
// canonicalize keeps its declared text and is returned in skipped. When it
// fails as a whole it returns the error and, in skipped, every table whose text
// it would have rewritten; s is then unchanged. Drift uses it in place of
// Canonicalizer so that a role or session that cannot run the round trip still
// gets a report.
type TableCanonicalizer interface {
	CanonicalizeTables(ctx context.Context, c momo.PinnedConn, s *schema.Schema) (skipped []ObjectRef, err error)
}
