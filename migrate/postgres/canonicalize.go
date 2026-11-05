// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

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
// The same round trip rewrites the text of tables: check expressions, column
// defaults and generation expressions (pg_get_constraintdef and pg_get_expr add
// parentheses and casts, turn IN lists into = ANY (ARRAY[...]) and BETWEEN into
// two comparisons), and the predicate and expression parts of indexes. Without
// it a freshly applied migration that declares CHECK (length(title) > 0) reads
// back as ((length((title)::text) > 0)) and drifts at once.
//
// Foreign keys, indexes without text and the schema qualifiers of the declared
// objects are not reproduced in the scratch schema, because none of them
// affects the text read back. Only the tables that carry text are created (all
// of them when the schema also declares views, functions, triggers or
// policies, which may refer to any table). Tables of one name in different
// schemas are canonicalized in separate scratch schemas, one per round, so none
// of them is refused; two views or functions of one name in different schemas
// still are.
//
// Limit: each round's scratch schema leads the search_path alone, so a table,
// trigger or policy in a later round (one whose table or index name is taken in
// an earlier round) does not see a function created in an earlier round's
// scratch schema. It resolves the live function of that name when there is one;
// otherwise Canonicalize fails.
func (d Dialect) Canonicalize(ctx context.Context, c momo.PinnedConn, s *schema.Schema) error {
	_, err := d.canonicalize(ctx, c, s, false)
	return err
}

// CanonicalizeTables is the best-effort form Drift uses: it rewrites only the
// table text of s (check expressions, defaults, generation expressions, index
// predicates and expressions) and leaves views, functions, triggers and
// policies as declared. A table whose scratch copy cannot be created or read
// back keeps its declared text and is returned in skipped (each creation runs
// in a savepoint, so one failing table does not abort the others). When the
// whole round trip fails (no CREATE privilege, a read-only transaction, a
// failing read-back) it returns the error together with every table that
// carries text in skipped; s is then left as declared. The search_path limit of
// Canonicalize applies: a function created in an earlier round's scratch schema
// is not visible to a later round.
func (d Dialect) CanonicalizeTables(ctx context.Context, c momo.PinnedConn, s *schema.Schema) (skipped []migrate.ObjectRef, err error) {
	skipped, err = d.canonicalize(ctx, c, s, true)
	if err != nil {
		return textTables(s), err
	}
	return skipped, nil
}

// textTables lists the tables of s that carry text PostgreSQL rewrites.
func textTables(s *schema.Schema) []migrate.ObjectRef {
	var out []migrate.ObjectRef
	for _, t := range s.Tables {
		if tableHasText(t) {
			out = append(out, migrate.ObjectRef{Kind: "table", Schema: t.Schema, Name: t.Name})
		}
	}
	return out
}

// canonRound is the part of a schema canonicalized in one scratch schema:
// sub holds copies of the chosen objects, and the index lists say where each
// came from in the source schema.
type canonRound struct {
	sub                               *schema.Schema
	tables, views, funcs, trigs, pols []int
}

func (r *canonRound) empty() bool {
	return len(r.tables)+len(r.views)+len(r.funcs)+len(r.trigs)+len(r.pols) == 0
}

// planRounds splits s into the rounds that are created in one scratch schema
// each. A table goes to the first round in which neither its name nor the name
// of one of its scratch indexes or unique constraints is taken, so a round never
// holds two relations of one name (tables of one name in different schemas, or
// differently named tables whose indexes share a name); views and functions go to round 0, triggers and policies
// to the round of their table. With tablesOnly only tables with text are
// planned.
func planRounds(s *schema.Schema, tablesOnly bool) []*canonRound {
	var rounds []*canonRound
	round := func(i int) *canonRound {
		for len(rounds) <= i {
			rounds = append(rounds, &canonRound{sub: &schema.Schema{Dialect: dialect.Postgres, Enums: s.Enums}})
		}
		return rounds[i]
	}
	needAll := !tablesOnly && len(s.Views)+len(s.Functions)+len(s.Triggers)+len(s.Policies) > 0
	// used holds, per round, the relation names (tables, and the indexes and
	// unique constraints that carry an index of their own name) already
	// created there: one scratch schema has one relation namespace.
	var used []map[string]bool
	where := map[[2]string]int{}
	for i, t := range s.Tables {
		if !needAll && !tableHasText(t) {
			continue
		}
		names := scratchRelationNames(t)
		n := 0
		for ; n < len(used) && slices.ContainsFunc(names, func(name string) bool { return used[n][name] }); n++ {
		}
		if n == len(used) {
			used = append(used, map[string]bool{})
		}
		for _, name := range names {
			used[n][name] = true
		}
		where[[2]string{t.Schema, t.Name}] = n
		r := round(n)
		r.tables = append(r.tables, i)
		r.sub.Tables = append(r.sub.Tables, t)
	}
	if !tablesOnly {
		for i := range s.Views {
			r := round(0)
			r.views = append(r.views, i)
			r.sub.Views = append(r.sub.Views, s.Views[i])
		}
		for i := range s.Functions {
			r := round(0)
			r.funcs = append(r.funcs, i)
			r.sub.Functions = append(r.sub.Functions, s.Functions[i])
		}
		for i, t := range s.Triggers {
			r := round(where[[2]string{t.Schema, t.Table}])
			r.trigs = append(r.trigs, i)
			r.sub.Triggers = append(r.sub.Triggers, t)
		}
		for i, p := range s.Policies {
			r := round(where[[2]string{p.Schema, p.Table}])
			r.pols = append(r.pols, i)
			r.sub.Policies = append(r.sub.Policies, p)
		}
	}
	return slices.DeleteFunc(rounds, func(r *canonRound) bool { return r.empty() })
}

// scratchRelationNames lists the relation names t creates in a scratch schema:
// the table, each index that scratchModel keeps (the ones with text) and each
// named unique constraint.
func scratchRelationNames(t schema.Table) []string {
	names := []string{t.Name}
	for _, ix := range t.Indexes {
		if indexHasText(ix) && ix.Name != "" {
			names = append(names, ix.Name)
		}
	}
	for _, u := range t.Uniques {
		if u.Name != "" {
			names = append(names, u.Name)
		}
	}
	return names
}

// savepoint names the savepoint around one scratch table in best-effort mode.
const savepoint = "momo_canon_sp"

func (d Dialect) canonicalize(ctx context.Context, c momo.PinnedConn, s *schema.Schema, tolerant bool) (skipped []migrate.ObjectRef, err error) {
	rounds := planRounds(s, tolerant)
	if len(rounds) == 0 {
		return nil, nil
	}
	if err := requireCreateOnDatabase(ctx, c); err != nil {
		return nil, err
	}
	if _, err := c.Exec(ctx, "BEGIN", nil); err != nil {
		return nil, fmt.Errorf("postgres canonicalize: begin: %w", err)
	}
	defer func() {
		// The rollback runs detached from ctx so a cancelled context cannot
		// leave the scratch schema or an open transaction on the connection.
		rctx := context.WithoutCancel(ctx)
		if _, rbErr := c.Exec(rctx, "ROLLBACK", nil); rbErr != nil {
			err = errors.Join(err, fmt.Errorf("postgres canonicalize: rollback: %w", rbErr), discard(c))
		}
	}()
	var path string
	if err := queryOne(ctx, c, `SELECT current_setting('search_path')`, nil, &path); err != nil {
		return nil, fmt.Errorf("postgres canonicalize: read search_path: %w", err)
	}
	// Rewrite a copy so a failure in a later round leaves s as declared.
	out := *s
	out.Tables, out.Views = slices.Clone(s.Tables), slices.Clone(s.Views)
	out.Functions, out.Triggers, out.Policies = slices.Clone(s.Functions), slices.Clone(s.Triggers), slices.Clone(s.Policies)
	for _, r := range rounds {
		sk, err := d.canonicalizeRound(ctx, c, r, path, tolerant)
		if err != nil {
			return nil, err
		}
		for k, i := range r.tables {
			if slices.Contains(sk, k) {
				t := s.Tables[i]
				skipped = append(skipped, migrate.ObjectRef{Kind: "table", Schema: t.Schema, Name: t.Name})
				continue
			}
			out.Tables[i] = r.sub.Tables[k]
		}
		for k, i := range r.views {
			out.Views[i] = r.sub.Views[k]
		}
		for k, i := range r.funcs {
			out.Functions[i] = r.sub.Functions[k]
		}
		for k, i := range r.trigs {
			out.Triggers[i] = r.sub.Triggers[k]
		}
		for k, i := range r.pols {
			out.Policies[i] = r.sub.Policies[k]
		}
	}
	*s = out
	return skipped, nil
}

// canonicalizeRound creates one round in a fresh scratch schema inside the open
// transaction, reads it back and rewrites r.sub. It returns the positions in
// r.sub.Tables left as declared (tolerant mode only).
func (d Dialect) canonicalizeRound(ctx context.Context, c momo.PinnedConn, r *canonRound, path string, tolerant bool) ([]int, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, fmt.Errorf("postgres canonicalize: scratch name: %w", err)
	}
	scratch := "momo_canon_" + hex.EncodeToString(b[:])
	if _, err := c.Exec(ctx, "CREATE SCHEMA "+ident(scratch), nil); err != nil {
		return nil, fmt.Errorf("postgres canonicalize: create scratch schema: %w", err)
	}
	// is_local: the path reverts at ROLLBACK. The scratch schema leads and the
	// connection's own path follows, so unqualified names resolve as they do
	// in the live database.
	if err := execOne(ctx, c, `SELECT set_config('search_path', quote_ident($1) || ', ' || $2, true)`, scratch, path); err != nil {
		return nil, fmt.Errorf("postgres canonicalize: set search_path: %w", err)
	}
	var failed []int
	if tolerant {
		var err error
		if failed, err = d.createEachTable(ctx, c, r.sub); err != nil {
			return nil, err
		}
	} else {
		mini, err := scratchModel(r.sub)
		if err != nil {
			return nil, err
		}
		stmts, err := scratchStatements(d, &schema.Schema{Dialect: dialect.Postgres}, mini)
		if err != nil {
			return nil, err
		}
		if err := execAll(ctx, c, stmts); err != nil {
			return nil, fmt.Errorf("postgres canonicalize: create declared objects in scratch schema: %w", err)
		}
	}
	// The scratch schema leads the search_path, so it is the current schema and
	// its tables are reported unqualified.
	read, err := d.Inspect(ctx, c, migrate.InspectOptions{Schemas: []string{scratch}})
	if err != nil {
		return nil, fmt.Errorf("postgres canonicalize: read back: %w", err)
	}
	if !tolerant {
		return nil, rewriteText(r.sub, read)
	}
	tables := tablesByName(read)
	for k := range r.sub.Tables {
		if slices.Contains(failed, k) {
			continue
		}
		if err := rewriteTable(&r.sub.Tables[k], tables); err != nil {
			failed = append(failed, k)
		}
	}
	return failed, nil
}

// createEachTable creates the enums of sub, then each table in a savepoint of
// its own, and returns the positions of the tables whose creation failed (their
// savepoint is rolled back, so they leave nothing behind). An error that is not
// a failure of one table's statements aborts the round.
func (d Dialect) createEachTable(ctx context.Context, c momo.PinnedConn, sub *schema.Schema) ([]int, error) {
	empty := &schema.Schema{Dialect: dialect.Postgres}
	base, err := scratchModel(&schema.Schema{Dialect: dialect.Postgres, Enums: sub.Enums})
	if err != nil {
		return nil, err
	}
	stmts, err := scratchStatements(d, empty, base)
	if err != nil {
		return nil, err
	}
	if err := execAll(ctx, c, stmts); err != nil {
		return nil, fmt.Errorf("postgres canonicalize: create declared types in scratch schema: %w", err)
	}
	var failed []int
	for k, t := range sub.Tables {
		one, err := scratchModel(&schema.Schema{Dialect: dialect.Postgres, Enums: sub.Enums, Tables: []schema.Table{t}})
		if err != nil {
			return nil, err
		}
		stmts, err := scratchStatements(d, base, one)
		if err != nil {
			failed = append(failed, k)
			continue
		}
		if _, err := c.Exec(ctx, "SAVEPOINT "+savepoint, nil); err != nil {
			return nil, fmt.Errorf("postgres canonicalize: savepoint: %w", err)
		}
		if err := execAll(ctx, c, stmts); err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
			if _, rbErr := c.Exec(ctx, "ROLLBACK TO SAVEPOINT "+savepoint, nil); rbErr != nil {
				return nil, fmt.Errorf("postgres canonicalize: roll back savepoint: %w", errors.Join(err, rbErr))
			}
			failed = append(failed, k)
		}
		if _, err := c.Exec(ctx, "RELEASE SAVEPOINT "+savepoint, nil); err != nil {
			return nil, fmt.Errorf("postgres canonicalize: release savepoint: %w", err)
		}
	}
	return failed, nil
}

func execAll(ctx context.Context, c momo.PinnedConn, stmts []migrate.Statement) error {
	for _, st := range stmts {
		if _, err := c.Exec(ctx, st.SQL, nil); err != nil {
			return err
		}
	}
	return nil
}

// hasTableText reports whether s declares table text PostgreSQL rewrites: a
// check expression, a column default or generation expression, or an index
// predicate or expression part.
func hasTableText(s *schema.Schema) bool {
	return slices.ContainsFunc(s.Tables, tableHasText)
}

func tableHasText(t schema.Table) bool {
	if len(t.Checks) > 0 {
		return true
	}
	for _, c := range t.Columns {
		if c.Default != "" || c.Generated != "" {
			return true
		}
	}
	return slices.ContainsFunc(t.Indexes, indexHasText)
}

// indexHasText reports whether ix has a predicate or an expression part.
func indexHasText(ix schema.Index) bool {
	if ix.Where != "" {
		return true
	}
	for _, p := range ix.Parts {
		if p.Expr != "" {
			return true
		}
	}
	return false
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
		t.Schema, t.ForeignKeys = "", nil
		t.Indexes = slices.DeleteFunc(t.Indexes, func(ix schema.Index) bool { return !indexHasText(ix) })
		for j := range t.Indexes {
			t.Indexes[j].Concurrently = false
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

// scratchStatements renders the creation of every object of m that from lacks,
// in migration order.
func scratchStatements(d Dialect, from, m *schema.Schema) ([]migrate.Statement, error) {
	changes, _, err := migrate.Diff(from, m, migrate.DiffOptions{})
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
	if err := rewriteTables(s, read); err != nil {
		return err
	}
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

// rewriteTables replaces, in s, the check expressions, column defaults,
// generation expressions and index predicates and expressions with those of the
// table of the same name in read.
func rewriteTables(s, read *schema.Schema) error {
	tables := tablesByName(read)
	for i := range s.Tables {
		if err := rewriteTable(&s.Tables[i], tables); err != nil {
			return err
		}
	}
	return nil
}

func tablesByName(read *schema.Schema) map[string]*schema.Table {
	tables := map[string]*schema.Table{}
	for i := range read.Tables {
		tables[read.Tables[i].Name] = &read.Tables[i]
	}
	return tables
}

// rewriteTable rewrites the text of t from the table of the same name in tables.
// It changes t only when the whole table could be rewritten.
func rewriteTable(t *schema.Table, tables map[string]*schema.Table) error {
	if !tableHasText(*t) {
		return nil
	}
	r, ok := tables[t.Name]
	if !ok {
		return fmt.Errorf("postgres canonicalize: table %q was not created in the scratch schema", t.Name)
	}
	n := *t
	n.Columns, n.Checks = slices.Clone(t.Columns), slices.Clone(t.Checks)
	n.Indexes = slices.Clone(t.Indexes)
	for j := range n.Columns {
		c := &n.Columns[j]
		for _, rc := range r.Columns {
			if rc.Name != c.Name {
				continue
			}
			if c.Default != "" {
				c.Default = rc.Default
			}
			if c.Generated != "" {
				c.Generated = rc.Generated
			}
		}
	}
	for j := range n.Checks {
		rc, ok := findCheck(r.Checks, n.Checks[j].Name)
		if !ok {
			return fmt.Errorf("postgres canonicalize: check %q on %q was not created in the scratch schema", n.Checks[j].Name, t.Name)
		}
		n.Checks[j].Expr = rc.Expr
	}
	for j := range n.Indexes {
		ix := &n.Indexes[j]
		if !indexHasText(*ix) {
			continue
		}
		ri, ok := findIndex(r.Indexes, ix.Name)
		if !ok || len(ri.Parts) != len(ix.Parts) {
			return fmt.Errorf("postgres canonicalize: index %q on %q was not created in the scratch schema", ix.Name, t.Name)
		}
		if ix.Where != "" {
			ix.Where = ri.Where
		}
		ix.Parts = slices.Clone(ix.Parts)
		for k := range ix.Parts {
			if ix.Parts[k].Expr != "" {
				// PostgreSQL may store an expression key as a plain column
				// (v::vector(3) with an operator class on a vector(3)
				// column), so take the whole part as read back.
				ix.Parts[k] = ri.Parts[k]
			}
		}
	}
	*t = n
	return nil
}

func findCheck(cs []schema.Check, name string) (schema.Check, bool) {
	for _, c := range cs {
		if c.Name == name {
			return c, true
		}
	}
	return schema.Check{}, false
}

func findIndex(ixs []schema.Index, name string) (schema.Index, bool) {
	for _, ix := range ixs {
		if ix.Name == name {
			return ix, true
		}
	}
	return schema.Index{}, false
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

// execOne runs a statement with text arguments and discards the result.
func execOne(ctx context.Context, c momo.PinnedConn, sql string, args ...any) error {
	rows, err := c.Query(ctx, sql, args)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
	}
	return rows.Err()
}
