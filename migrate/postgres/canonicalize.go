// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// Canonicalize rewrites the declared views, functions, policies and triggers of
// s to the text PostgreSQL itself reports: pg_get_viewdef qualifies and
// re-parenthesizes a view query, pg_get_function_arguments and
// pg_get_function_result respell a signature, and pg_get_expr and
// pg_get_triggerdef respell policy and trigger conditions. Without this a
// declared object never compares equal to its inspection, and the first Diff
// after a baseline would drop and recreate every view and its dependents, which
// loses the data of a materialized view.
//
// It creates the declared objects in a scratch schema inside a transaction
// (tables first, dependencies ordered as a migration would order them), reads
// them back with the inspector and replaces the declared text in s, then rolls
// the transaction back, so the scratch schema and everything in it vanish and
// the application schema is untouched. The scratch schema comes first on the
// search_path and the connection's own path follows, so unqualified names in the
// declared text resolve as they do in the live database. A schema without
// views, functions, triggers or policies is left alone and no statement runs.
//
// Table indexes, foreign keys and the schema qualifiers of the declared objects
// are not reproduced in the scratch schema, because none of them affects the
// text read back; two declared objects of one kind and name in different
// namespaces are therefore refused.
func (d Dialect) Canonicalize(ctx context.Context, c momo.PinnedConn, s *schema.Schema) (err error) {
	if len(s.Views) == 0 && len(s.Functions) == 0 && len(s.Triggers) == 0 && len(s.Policies) == 0 {
		return nil
	}
	if err := requireCreateOnDatabase(ctx, c); err != nil {
		return err
	}
	mini, err := scratchModel(s)
	if err != nil {
		return err
	}
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Errorf("postgres canonicalize: scratch name: %w", err)
	}
	scratch := "momo_canon_" + hex.EncodeToString(b[:])
	stmts, err := scratchStatements(d, mini)
	if err != nil {
		return err
	}

	if _, err := c.Exec(ctx, "BEGIN", nil); err != nil {
		return fmt.Errorf("postgres canonicalize: begin: %w", err)
	}
	defer func() {
		// The rollback runs detached from ctx so a cancelled context cannot
		// leave the scratch schema or an open transaction on the connection.
		rctx := context.WithoutCancel(ctx)
		if _, rbErr := c.Exec(rctx, "ROLLBACK", nil); rbErr != nil {
			err = errors.Join(err, fmt.Errorf("postgres canonicalize: rollback: %w", rbErr), discard(c))
		}
	}()
	if _, err := c.Exec(ctx, "CREATE SCHEMA "+ident(scratch), nil); err != nil {
		return fmt.Errorf("postgres canonicalize: create scratch schema: %w", err)
	}
	// is_local: the path reverts at ROLLBACK.
	if err := execOne(ctx, c, `SELECT set_config('search_path', quote_ident($1) || ', ' || current_setting('search_path'), true)`, scratch); err != nil {
		return fmt.Errorf("postgres canonicalize: set search_path: %w", err)
	}
	for _, st := range stmts {
		if _, err := c.Exec(ctx, st.SQL, nil); err != nil {
			return fmt.Errorf("postgres canonicalize: create declared objects in scratch schema: %w", err)
		}
	}
	read := &schema.Schema{Dialect: dialect.Postgres}
	if err := d.inspectObjects(ctx, c, read, []string{scratch}); err != nil {
		return fmt.Errorf("postgres canonicalize: read back: %w", err)
	}
	return rewriteText(s, read)
}

// requireCreateOnDatabase fails with an error wrapping
// migrate.ErrCanonicalizePrivilege when the connected role cannot CREATE a
// schema in the current database. Skipping canonicalization instead would
// compare raw declared text with text PostgreSQL rewrote and give false
// mismatches or false matches.
func requireCreateOnDatabase(ctx context.Context, c momo.PinnedConn) error {
	var ok bool
	if err := queryOne(ctx, c, `SELECT has_database_privilege(current_database(), 'CREATE')`, nil, &ok); err != nil {
		return fmt.Errorf("postgres canonicalize: check privilege: %w", err)
	}
	if !ok {
		return fmt.Errorf("%w: the role needs the CREATE privilege on the database to create the scratch schema that canonicalizes views, functions, triggers and policies (GRANT CREATE ON DATABASE ... or run the baseline with a role that has it)", migrate.ErrCanonicalizePrivilege)
	}
	return nil
}

// scratchModel returns a copy of s that can be created in one schema: no
// namespaces, extensions (the live database has them), schema qualifiers, table
// indexes or foreign keys.
func scratchModel(s *schema.Schema) (*schema.Schema, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("postgres canonicalize: copy schema: %w", err)
	}
	var m schema.Schema
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("postgres canonicalize: copy schema: %w", err)
	}
	m.Dialect = dialect.Postgres
	m.Schemas, m.Extensions = nil, nil
	seen := map[string]bool{}
	dup := func(kind, name string) error {
		k := kind + "\x00" + name
		if seen[k] {
			return fmt.Errorf("postgres canonicalize: two %ss named %q in different schemas cannot share the scratch schema", kind, name)
		}
		seen[k] = true
		return nil
	}
	for i := range m.Tables {
		t := &m.Tables[i]
		t.Schema, t.Indexes, t.ForeignKeys = "", nil, nil
		if err := dup("table", t.Name); err != nil {
			return nil, err
		}
	}
	for i := range m.Views {
		v := &m.Views[i]
		v.Schema, v.Indexes = "", nil
		if err := dup("view", v.Name); err != nil {
			return nil, err
		}
	}
	for i := range m.Functions {
		f := &m.Functions[i]
		f.Schema = ""
		if err := dup("function", f.Name+"("+f.Args+")"); err != nil {
			return nil, err
		}
	}
	for i := range m.Triggers {
		m.Triggers[i].Schema = ""
	}
	for i := range m.Policies {
		m.Policies[i].Schema = ""
	}
	return &m, nil
}

// scratchStatements renders the creation of every object of m, in migration order.
func scratchStatements(d Dialect, m *schema.Schema) ([]migrate.Statement, error) {
	empty := &schema.Schema{Dialect: dialect.Postgres}
	changes, _, err := migrate.Diff(empty, m, migrate.DiffOptions{})
	if err != nil {
		return nil, fmt.Errorf("postgres canonicalize: plan scratch objects: %w", err)
	}
	ordered, err := migrate.OrderWithViews(changes, nil, m.Views)
	if err != nil {
		return nil, fmt.Errorf("postgres canonicalize: order scratch objects: %w", err)
	}
	stmts, err := d.Render(ordered)
	if err != nil {
		return nil, fmt.Errorf("postgres canonicalize: render scratch objects: %w", err)
	}
	for _, st := range stmts {
		if st.NoTx {
			return nil, errors.New("postgres canonicalize: a scratch statement cannot run in a transaction")
		}
	}
	return stmts, nil
}

// rewriteText replaces, in s, the text of each view, function, trigger and
// policy with that of the object of the same identity in read.
func rewriteText(s, read *schema.Schema) error {
	views := map[string]schema.View{}
	for _, v := range read.Views {
		views[v.Name] = v
	}
	for i := range s.Views {
		v, ok := views[s.Views[i].Name]
		if !ok {
			return fmt.Errorf("postgres canonicalize: view %q was not created in the scratch schema", s.Views[i].Name)
		}
		s.Views[i].Query = v.Query
	}
	byName := map[string][]schema.Function{}
	for _, f := range read.Functions {
		byName[f.Name] = append(byName[f.Name], f)
	}
	for i := range s.Functions {
		f, err := matchFunction(s.Functions[i], byName[s.Functions[i].Name])
		if err != nil {
			return err
		}
		s.Functions[i].Args, s.Functions[i].Returns, s.Functions[i].Body = f.Args, f.Returns, f.Body
	}
	triggers := map[string]schema.Trigger{}
	for _, t := range read.Triggers {
		triggers[t.Table+"\x00"+t.Name] = t
	}
	for i := range s.Triggers {
		t, ok := triggers[s.Triggers[i].Table+"\x00"+s.Triggers[i].Name]
		if !ok {
			return fmt.Errorf("postgres canonicalize: trigger %q on %q was not created in the scratch schema", s.Triggers[i].Name, s.Triggers[i].Table)
		}
		s.Triggers[i].When = t.When
	}
	policies := map[string]schema.Policy{}
	for _, p := range read.Policies {
		policies[p.Table+"\x00"+p.Name] = p
	}
	for i := range s.Policies {
		p, ok := policies[s.Policies[i].Table+"\x00"+s.Policies[i].Name]
		if !ok {
			return fmt.Errorf("postgres canonicalize: policy %q on %q was not created in the scratch schema", s.Policies[i].Name, s.Policies[i].Table)
		}
		s.Policies[i].Using, s.Policies[i].Check = p.Using, p.Check
	}
	return nil
}

// matchFunction picks, among the functions of one name read back, the one whose
// normalized argument list equals the declared one.
func matchFunction(declared schema.Function, cands []schema.Function) (schema.Function, error) {
	if len(cands) == 1 {
		return cands[0], nil
	}
	norm := func(f schema.Function) string {
		return schema.Normalized(&schema.Schema{Dialect: dialect.Postgres, Functions: []schema.Function{f}}).Functions[0].Args
	}
	want := norm(declared)
	for _, f := range cands {
		if norm(f) == want {
			return f, nil
		}
	}
	return schema.Function{}, fmt.Errorf("postgres canonicalize: function %s(%s) was not created in the scratch schema", declared.Name, declared.Args)
}

// execOne runs a statement with one text argument and discards the result.
func execOne(ctx context.Context, c momo.PinnedConn, sql, arg string) error {
	rows, err := c.Query(ctx, sql, []any{arg})
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
	}
	return rows.Err()
}
