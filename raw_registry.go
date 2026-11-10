// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/azrtydxb/momo/dialect"
)

// Compilable is a statement that renders to SQL without running it. The
// query builders (SelectQuery, InsertQuery, UpdateQuery, DeleteQuery,
// MergeQuery) implement it through their SQL method, and RawQuery wraps a
// hand-written statement.
type Compilable interface {
	SQL() (Compiled, error)
}

// RegisteredQuery is the compiled SQL of one named query for one dialect.
type RegisteredQuery struct {
	// Name is the name the query was registered under.
	Name string
	// SQL is the statement text with the dialect's placeholders. Argument
	// values are not part of it.
	SQL string
}

// QueryRegistry holds named query builders so a test can list the SQL an
// application issues without a database. It replaces the .sql files of a code
// generator: lint the listed SQL, freeze it in golden files (see
// momotest.QueryGolden), or run the previous release's list against a new
// schema. It is safe for concurrent use.
type QueryRegistry struct {
	mu     sync.Mutex
	builds map[string]func(Querier) Compilable
}

// NewQueryRegistry returns an empty registry, for a test that wants one
// isolated from the process-wide DefaultQueries.
func NewQueryRegistry() *QueryRegistry {
	return &QueryRegistry{builds: map[string]func(Querier) Compilable{}}
}

var defaultQueries = NewQueryRegistry()

// DefaultQueries returns the process-wide registry that RegisterQuery fills.
func DefaultQueries() *QueryRegistry { return defaultQueries }

// RegisterQuery registers build under name in the DefaultQueries registry. See
// QueryRegistry.Register.
func RegisterQuery(name string, build func(q Querier) Compilable) {
	defaultQueries.Register(name, build)
}

// Register adds a named query. build receives a Querier that only compiles
// (see NewOffline) and returns the statement, for example a SelectQuery or a
// RawQuery. It runs once per Compile and dialect, so it must not touch a
// database, and the argument values it binds are placeholders that never
// reach the listed SQL. Register panics when name is empty, build is nil or
// name is already registered, programming errors like a duplicate
// http.HandleFunc pattern.
func (r *QueryRegistry) Register(name string, build func(q Querier) Compilable) {
	switch {
	case name == "":
		panic("momo: RegisterQuery: the query name is empty")
	case build == nil:
		panic(fmt.Sprintf("momo: RegisterQuery(%q): the builder is nil", name))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.builds[name]; dup {
		panic(fmt.Sprintf("momo: RegisterQuery: query %q is already registered", name))
	}
	r.builds[name] = build
}

// Names returns the registered names in sorted order.
func (r *QueryRegistry) Names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.builds))
	for n := range r.builds {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// Compile renders every registered query for dialect d, sorted by name,
// without a database. A builder that fails, returns nil or renders an error
// (for example ErrUnsupported on a dialect that lacks the feature) fails the
// whole call with an error naming the query.
func (r *QueryRegistry) Compile(d dialect.Name) ([]RegisteredQuery, error) {
	names := r.Names()
	q := NewOffline(d)
	out := make([]RegisteredQuery, 0, len(names))
	for _, n := range names {
		r.mu.Lock()
		build := r.builds[n]
		r.mu.Unlock()
		c := build(q)
		if c == nil {
			return nil, validation("registry", "", "registered query %q (%s): the builder returned nil", n, d)
		}
		sql, err := c.SQL()
		if err != nil {
			return nil, fmt.Errorf("momo: registered query %q (%s): %w", n, d, err)
		}
		out = append(out, RegisteredQuery{Name: n, SQL: sql.SQL})
	}
	return out, nil
}

// RawQuery wraps a hand-written statement as a Compilable, with the
// placeholder rules of Exec ('?' binds, "??" is a literal '?', on PostgreSQL
// native $n placeholders). It is for QueryRegistry builders:
//
//	momo.RegisterQuery("open_tasks", func(q momo.Querier) momo.Compilable {
//		return momo.RawQuery(q, "SELECT id, title FROM tasks WHERE owner = $1 AND NOT done", 0)
//	})
//
// A malformed statement shows as the error of SQL.
func RawQuery(q Querier, sql string, args ...any) Compilable {
	c, err := compile(q.Dialect(), func(b *Builder) { b.appendRaw(sql, args) })
	return rawCompiled{c, err}
}

type rawCompiled struct {
	c   Compiled
	err error
}

func (r rawCompiled) SQL() (Compiled, error) { return r.c, r.err }

// NewOffline returns a *DB for dialect d that compiles statements but never
// reaches a database: every execution returns an error matching
// ErrUnsupported. Use it to call SQL on a query builder in a unit test. The
// dialect is the default one for d, without a MySQL flavor or a server
// version.
func NewOffline(d dialect.Name) *DB { return New(offlineDriver{dialect.For(d)}) }

var errOffline = errors.New("the offline handle has no database")

type offlineDriver struct{ d *dialect.Dialect }

func (o offlineDriver) Dialect() *dialect.Dialect { return o.d }

func (offlineDriver) Query(context.Context, string, []any) (Rows, error) {
	return nil, &Error{Kind: ErrUnsupported, Op: "offline", Err: errOffline}
}

func (offlineDriver) Exec(context.Context, string, []any) (int64, error) {
	return 0, &Error{Kind: ErrUnsupported, Op: "offline", Err: errOffline}
}

func (offlineDriver) Begin(context.Context, TxOptions) (DriverTx, error) {
	return nil, &Error{Kind: ErrUnsupported, Op: "offline", Err: errOffline}
}
