// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// Inspect reads the live database into a schema.Schema: tables with their
// columns, primary key, unique constraints, indexes, foreign keys and checks,
// views, and triggers. opts is not used: SQLite has one namespace per
// database file (the engine applies Config.Exclude itself), and the bookkeeping
// tables momo_migrations and momo_migration_lock, the sqlite_ internal tables,
// virtual tables and the shadow tables of virtual tables are left out.
//
// Objects that SQLite cannot hold (extensions, native enums, functions,
// policies, partitions, row-level security, schemas) are never reported.
// A TEXT column whose table carries CHECK ("col" IN ('a', 'b')) is reported as
// an enum column: Column.Enum is "<table>_<column>" and Schema.Enums lists the
// values, which is how Render writes such a column back.
func (Dialect) Inspect(ctx context.Context, c momo.PinnedConn, _ migrate.InspectOptions) (*schema.Schema, error) {
	in := &inspector{ctx: ctx, c: c}
	return in.run()
}

type inspector struct {
	ctx context.Context
	c   momo.PinnedConn

	indexSQL map[string]string // index name -> CREATE INDEX text
}

type masterRow struct{ typ, name, tbl, sql string }

func (in *inspector) run() (*schema.Schema, error) {
	var rows []masterRow
	err := forEach(in.ctx, in.c, `SELECT type, name, tbl_name, sql FROM sqlite_schema
WHERE name NOT LIKE 'sqlite\_%' ESCAPE '\' AND sql IS NOT NULL ORDER BY type, name`, nil, func(r momo.Rows) error {
		var m masterRow
		var s sql.NullString
		if err := r.Scan(&m.typ, &m.name, &m.tbl, &s); err != nil {
			return err
		}
		m.sql = s.String
		rows = append(rows, m)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("migrate/sqlite: read sqlite_schema: %w", err)
	}
	skip, err := in.skippedTables(rows)
	if err != nil {
		return nil, err
	}
	in.indexSQL = map[string]string{}
	for _, m := range rows {
		if m.typ == "index" {
			in.indexSQL[m.name] = m.sql
		}
	}

	out := &schema.Schema{Dialect: dialect.SQLite}
	var tableRows []masterRow
	for _, m := range rows {
		if m.typ == "table" && !skip[m.name] {
			tableRows = append(tableRows, m)
		}
	}
	for _, m := range tableRows {
		t, enums, err := in.table(m)
		if err != nil {
			return nil, fmt.Errorf("migrate/sqlite: inspect table %q: %w", m.name, err)
		}
		out.Tables = append(out.Tables, *t)
		out.Enums = append(out.Enums, enums...)
	}

	known := map[string]string{} // lower-case name -> name, tables and views
	for _, t := range out.Tables {
		known[strings.ToLower(t.Name)] = t.Name
	}
	for _, m := range rows {
		if m.typ == "view" {
			known[strings.ToLower(m.name)] = m.name
		}
	}
	for _, m := range rows {
		switch m.typ {
		case "view":
			q, err := viewQuery(m.sql)
			if err != nil {
				return nil, fmt.Errorf("migrate/sqlite: inspect view %q: %w", m.name, err)
			}
			out.Views = append(out.Views, schema.View{Name: m.name, Query: q, Depends: dependsOf(q, m.name, known)})
		case "trigger":
			out.Triggers = append(out.Triggers, schema.Trigger{Name: m.name, Table: m.tbl, Definition: m.sql})
		}
	}
	out.Sort()
	return out, nil
}

// skippedTables names the tables Inspect leaves out: the bookkeeping tables,
// virtual tables and their shadow tables.
func (in *inspector) skippedTables(rows []masterRow) (map[string]bool, error) {
	skip := map[string]bool{historyTable: true, lockTable: true}
	for _, m := range rows {
		if m.typ == "table" && strings.HasPrefix(strings.ToUpper(strings.Join(strings.Fields(m.sql), " ")), "CREATE VIRTUAL TABLE") {
			skip[m.name] = true
		}
	}
	// pragma_table_list (SQLite 3.37) tells shadow tables apart. An older
	// library has no such function; its shadow tables are then reported as
	// ordinary tables.
	err := forEach(in.ctx, in.c, `SELECT name FROM pragma_table_list WHERE type IN ('shadow', 'virtual')`, nil, func(r momo.Rows) error {
		var n string
		if err := r.Scan(&n); err != nil {
			return err
		}
		skip[n] = true
		return nil
	})
	if err != nil && !isNoSuchTable(err, "pragma_table_list") {
		return nil, fmt.Errorf("migrate/sqlite: read pragma_table_list: %w", err)
	}
	return skip, nil
}

// xinfoColumn is one row of pragma_table_xinfo.
type xinfoColumn struct {
	name, typ string
	notNull   bool
	dflt      sql.NullString
	pk        int
	hidden    int
}

func (in *inspector) table(m masterRow) (*schema.Table, []schema.Enum, error) {
	var cols []xinfoColumn
	err := forEach(in.ctx, in.c, `SELECT name, type, "notnull", dflt_value, pk, hidden FROM pragma_table_xinfo(?) ORDER BY cid`, []any{m.name}, func(r momo.Rows) error {
		var x xinfoColumn
		var nn int
		if err := r.Scan(&x.name, &x.typ, &nn, &x.dflt, &x.pk, &x.hidden); err != nil {
			return err
		}
		x.notNull = nn != 0
		if x.hidden != 1 { // 1 is a hidden column of a virtual table
			cols = append(cols, x)
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read the columns: %w", err)
	}
	def := parseTableSQL(m.sql)

	t := &schema.Table{Name: m.name, WithoutRowID: def.withoutRowID, Strict: def.strict, Autoincrement: def.autoinc}
	type pkCol struct {
		ord  int
		name string
	}
	var pks []pkCol
	for _, x := range cols {
		col := schema.Column{Name: x.name, Type: x.typ, Nullable: !x.notNull, Default: x.dflt.String}
		if d, ok := def.columns[x.name]; ok {
			col.Collate = d.collate
			if x.hidden == 2 || x.hidden == 3 {
				col.Generated = d.generated
			}
		}
		t.Columns = append(t.Columns, col)
		if x.pk > 0 {
			pks = append(pks, pkCol{x.pk, x.name})
		}
	}
	slices.SortFunc(pks, func(a, b pkCol) int { return a.ord - b.ord })
	for _, p := range pks {
		t.PrimaryKey = append(t.PrimaryKey, p.name)
	}
	if len(t.PrimaryKey) == 1 && !t.WithoutRowID && !def.pkDesc {
		if col, ok := t.Column(t.PrimaryKey[0]); ok && strings.EqualFold(strings.TrimSpace(col.Type), "INTEGER") {
			col.Identity = "by_default" // an INTEGER PRIMARY KEY is the rowid
		}
	}

	if err := in.indexes(t, def); err != nil {
		return nil, nil, err
	}
	if err := in.foreignKeys(t, def); err != nil {
		return nil, nil, err
	}
	enums := applyChecks(t, def)
	return t, enums, nil
}

// indexes fills t.Uniques and t.Indexes from pragma_index_list.
func (in *inspector) indexes(t *schema.Table, def tableDef) error {
	type listed struct {
		name, origin string
		unique       bool
	}
	var list []listed
	err := forEach(in.ctx, in.c, `SELECT name, "unique", origin FROM pragma_index_list(?) ORDER BY seq`, []any{t.Name}, func(r momo.Rows) error {
		var l listed
		var u int
		if err := r.Scan(&l.name, &u, &l.origin); err != nil {
			return err
		}
		l.unique = u != 0
		list = append(list, l)
		return nil
	})
	if err != nil {
		return fmt.Errorf("read the index list: %w", err)
	}
	for _, l := range list {
		if l.origin == "pk" {
			continue
		}
		keys, err := in.indexKeys(l.name)
		if err != nil {
			return err
		}
		if l.origin == "u" {
			var cols []string
			for _, k := range keys {
				cols = append(cols, k.name)
			}
			name, ok := def.uniqueNames[strings.Join(cols, "\x00")]
			if !ok {
				name = t.Name + "_" + strings.Join(cols, "_") + "_key"
			}
			t.Uniques = append(t.Uniques, schema.Unique{Name: name, Columns: cols})
			continue
		}
		if strings.HasPrefix(l.name, "sqlite_autoindex_") {
			continue
		}
		ix, err := in.index(l.name, l.unique, keys)
		if err != nil {
			return fmt.Errorf("index %q: %w", l.name, err)
		}
		t.Indexes = append(t.Indexes, *ix)
	}
	return nil
}

// indexKey is one key row of pragma_index_xinfo.
type indexKey struct {
	cid        int
	name, coll string
	desc       bool
}

func (in *inspector) indexKeys(index string) ([]indexKey, error) {
	var keys []indexKey
	err := forEach(in.ctx, in.c, `SELECT cid, name, "desc", coll FROM pragma_index_xinfo(?) WHERE key = 1 ORDER BY seqno`, []any{index}, func(r momo.Rows) error {
		var k indexKey
		var name, coll sql.NullString
		var d int
		if err := r.Scan(&k.cid, &name, &d, &coll); err != nil {
			return err
		}
		k.name, k.coll, k.desc = name.String, coll.String, d != 0
		keys = append(keys, k)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read the keys of index %q: %w", index, err)
	}
	return keys, nil
}

// index builds an Index from its key rows and its CREATE INDEX text, which
// holds the expressions and the WHERE predicate.
func (in *inspector) index(name string, unique bool, keys []indexKey) (*schema.Index, error) {
	ix := &schema.Index{Name: name, Unique: unique}
	text := in.indexSQL[name]
	parts, where := parseIndexSQL(text)
	for i, k := range keys {
		p := schema.IndexPart{Desc: k.desc}
		if k.cid >= 0 {
			p.Column = k.name
		} else {
			if i >= len(parts) || parts[i] == "" {
				return nil, fmt.Errorf("the expression of key %d is not in the index text %q", i+1, text)
			}
			p.Expr = parts[i]
		}
		ix.Parts = append(ix.Parts, p)
	}
	ix.Where = where
	return ix, nil
}

// foreignKeys fills t.ForeignKeys from pragma_foreign_key_list.
func (in *inspector) foreignKeys(t *schema.Table, def tableDef) error {
	type fkRow struct {
		id                 int
		table, from, to    string
		toNull             bool
		onUpdate, onDelete string
	}
	var rows []fkRow
	err := forEach(in.ctx, in.c, `SELECT id, "table", "from", "to", on_update, on_delete FROM pragma_foreign_key_list(?) ORDER BY id, seq`, []any{t.Name}, func(r momo.Rows) error {
		var f fkRow
		var to sql.NullString
		if err := r.Scan(&f.id, &f.table, &f.from, &to, &f.onUpdate, &f.onDelete); err != nil {
			return err
		}
		f.to, f.toNull = to.String, !to.Valid
		rows = append(rows, f)
		return nil
	})
	if err != nil {
		return fmt.Errorf("read the foreign keys: %w", err)
	}
	for i := 0; i < len(rows); {
		j := i
		for j < len(rows) && rows[j].id == rows[i].id {
			j++
		}
		group := rows[i:j]
		fk := schema.ForeignKey{RefTable: group[0].table, OnDelete: fkAction(group[0].onDelete), OnUpdate: fkAction(group[0].onUpdate)}
		for _, g := range group {
			fk.Columns = append(fk.Columns, g.from)
			if !g.toNull {
				fk.RefColumns = append(fk.RefColumns, g.to)
			}
		}
		if len(fk.RefColumns) == 0 {
			// REFERENCES parent with no column list targets parent's primary key.
			pk, err := in.primaryKey(fk.RefTable)
			if err != nil {
				return err
			}
			fk.RefColumns = pk
		}
		key := strings.Join(fk.Columns, "\x00") + "\x01" + strings.ToLower(fk.RefTable)
		if name, ok := def.fkNames[key]; ok {
			fk.Name = name
		} else {
			fk.Name = t.Name + "_" + strings.Join(fk.Columns, "_") + "_fkey"
		}
		t.ForeignKeys = append(t.ForeignKeys, fk)
		i = j
	}
	return nil
}

// primaryKey returns the primary key columns of table, in key order; empty when
// the table is missing or has none.
func (in *inspector) primaryKey(table string) ([]string, error) {
	type pk struct {
		ord  int
		name string
	}
	var pks []pk
	err := forEach(in.ctx, in.c, `SELECT name, pk FROM pragma_table_info(?) WHERE pk > 0`, []any{table}, func(r momo.Rows) error {
		var p pk
		if err := r.Scan(&p.name, &p.ord); err != nil {
			return err
		}
		pks = append(pks, p)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read the primary key of %q: %w", table, err)
	}
	slices.SortFunc(pks, func(a, b pk) int { return a.ord - b.ord })
	out := make([]string, len(pks))
	for i, p := range pks {
		out[i] = p.name
	}
	return out, nil
}

// fkAction maps a pragma action to the schema spelling: NO ACTION is empty.
func fkAction(a string) string {
	if strings.EqualFold(a, "NO ACTION") {
		return ""
	}
	return strings.ToUpper(a)
}

// applyChecks fills t.Checks from the CHECK clauses in the table text and turns
// a TEXT column's membership check into an enum column. It returns the enums it
// created.
func applyChecks(t *schema.Table, def tableDef) []schema.Enum {
	var enums []schema.Enum
	seen := map[string]bool{}
	unnamed := 0
	for _, ck := range def.checks {
		if col, values, ok := enumCheck(ck.expr); ok && !seen[col] {
			if c, found := t.Column(col); found && c.Enum == "" && strings.EqualFold(strings.TrimSpace(c.Type), "TEXT") {
				seen[col] = true
				c.Enum = t.Name + "_" + col
				enums = append(enums, schema.Enum{Name: c.Enum, Values: values})
				continue
			}
		}
		name := ck.name
		if name == "" {
			name = t.Name + "_check"
			if unnamed > 0 {
				name = fmt.Sprintf("%s_check%d", t.Name, unnamed)
			}
			unnamed++
		}
		t.Checks = append(t.Checks, schema.Check{Name: name, Expr: ck.expr})
	}
	return enums
}

// viewQuery returns the SELECT text of a stored CREATE VIEW statement: the text
// after the AS keyword that follows the view name and its optional column list.
func viewQuery(createSQL string) (string, error) {
	toks := tokenize(createSQL)
	for i, t := range toks {
		if t.upper() == "AS" && i >= 2 {
			q := strings.TrimSpace(createSQL[t.end:])
			return strings.TrimRight(q, "; \t\r\n"), nil
		}
	}
	return "", fmt.Errorf("no AS in %q", createSQL)
}

// dependsOf lists the known tables and views that query names, in the form the
// inspector reports them (a bare name). The view's own name is left out.
func dependsOf(query, self string, known map[string]string) []string {
	found := map[string]bool{}
	collectNames(query, known, found, 0)
	delete(found, self)
	out := make([]string, 0, len(found))
	for n := range found {
		out = append(out, n)
	}
	slices.Sort(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

// maxNesting bounds the recursion into parenthesised groups.
const maxNesting = 64

func collectNames(src string, known map[string]string, out map[string]bool, depth int) {
	if depth > maxNesting {
		return
	}
	for _, t := range tokenize(src) {
		switch t.kind {
		case tokGroup:
			collectNames(groupInner(t), known, out, depth+1)
		case tokWord, tokIdent:
			n := strings.ToLower(strings.TrimPrefix(strings.ToLower(t.name()), "main."))
			if real, ok := known[n]; ok {
				out[real] = true
			}
		}
	}
}

// isNoSuchTable reports whether err is SQLite's "no such table" error for the
// named table (SQLite may spell it main.<name>).
func isNoSuchTable(err error, name string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no such table: "+name) || strings.Contains(msg, "no such table: main."+name)
}
