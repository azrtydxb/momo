// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// A SQLite table rebuild follows the procedure of
// https://www.sqlite.org/lang_altertable.html#otheralter:
//
//  1. Turn foreign-key enforcement off (Dialect.Setup with ForeignKeysOff,
//     outside the transaction: PRAGMA foreign_keys is a no-op inside one).
//  2. Start a transaction (the runner does).
//  3. Remember the indexes, triggers and views of the table.
//  4. CREATE TABLE new_X with the desired definition.
//  5. INSERT INTO new_X SELECT ... FROM X.
//  6. DROP TABLE X.
//  7. ALTER TABLE new_X RENAME TO X.
//  8. Recreate the indexes, triggers and views.
//  9. Drop and recreate the views that used a column that changed (every view
//     and trigger that mentions the table is dropped before step 4, because
//     DROP TABLE leaves views behind and RENAME fails while one names a
//     missing table).
//  10. PRAGMA foreign_key_check, and fail the transaction on any row.
//  11. Commit.
//  12. Turn foreign-key enforcement back on (the reset Setup returned).
//
// RebuildStatements and the rebuild RenderSchema writes cover steps 3 to 10;
// the runner supplies 2 and 11, and Dialect.Setup 1 and 12.

// Dependent is a view or trigger that a table rebuild must drop and recreate.
type Dependent struct {
	Type  string // "view" or "trigger"
	Name  string
	Table string // the trigger's table (sqlite_schema.tbl_name); the view's own name for a view
	SQL   string // the stored CREATE statement
}

// Capture reads, in rowid order, every view and every trigger whose SQL mentions
// table (a whole-word, case-insensitive match that ignores string literals),
// plus every trigger on table, plus every view or trigger that mentions a view
// already captured: SQLite fails ALTER TABLE ... RENAME while any view names a
// missing object, so a view that sits on top of a captured view has to go too.
// Over-inclusion is safe: recreating an unaffected object verbatim does not
// change behaviour.
func Capture(ctx context.Context, c momo.PinnedConn, table string) ([]Dependent, error) {
	var all []Dependent
	err := forEach(ctx, c, `SELECT type, name, tbl_name, sql FROM sqlite_schema WHERE type IN ('view', 'trigger') AND sql IS NOT NULL ORDER BY rowid`, nil, func(r momo.Rows) error {
		var d Dependent
		if err := r.Scan(&d.Type, &d.Name, &d.Table, &d.SQL); err != nil {
			return err
		}
		all = append(all, d)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("migrate/sqlite: read the views and triggers: %w", err)
	}
	names := map[string]string{strings.ToLower(table): table}
	taken := make([]bool, len(all))
	for changed := true; changed; {
		changed = false
		for i, d := range all {
			if taken[i] {
				continue
			}
			if d.Type == "trigger" && strings.EqualFold(d.Table, table) || mentionsAny(d.SQL, names) {
				taken[i] = true
				changed = true
				if d.Type == "view" {
					names[strings.ToLower(d.Name)] = d.Name
				}
			}
		}
	}
	var out []Dependent
	for i, d := range all {
		if taken[i] {
			out = append(out, d)
		}
	}
	return out, nil
}

// mentionsAny reports whether src names any of names (lower-case key, real value).
func mentionsAny(src string, names map[string]string) bool {
	found := map[string]bool{}
	collectNames(src, names, found, 0)
	return len(found) > 0
}

// Rebuild writes the statements that rebuild table to the desired definition
// on the live database c: it captures the views and triggers that mention the
// table, reads the table's current columns, and calls RebuildStatements. Rows
// are copied for the columns that exist in both.
//
// Precondition: the caller ran Setup(ctx, c, SessionOptions{ForeignKeysOff:
// true}) outside any transaction, and runs the result with migrate.RunStatements
// (one transaction; the fk-check statement fails it on a violation).
func Rebuild(ctx context.Context, c momo.PinnedConn, table string, desired schema.Table, desiredIndexes []schema.Index) ([]migrate.Statement, error) {
	var old []string
	err := forEach(ctx, c, `SELECT name FROM pragma_table_xinfo(?) WHERE hidden <> 1 ORDER BY cid`, []any{table}, func(r momo.Rows) error {
		var n string
		if err := r.Scan(&n); err != nil {
			return err
		}
		old = append(old, n)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("migrate/sqlite: read the columns of %q: %w", table, err)
	}
	if len(old) == 0 {
		return nil, &momo.Error{Kind: momo.ErrValidation, Op: "migrate/sqlite", Table: table, Err: fmt.Errorf("the table %q does not exist", table)}
	}
	deps, err := Capture(ctx, c, table)
	if err != nil {
		return nil, err
	}
	return RebuildStatements(table, old, desired, desiredIndexes, deps)
}

// RebuildStatements is Rebuild without I/O. The statements, in order:
//
//  1. DROP VIEW / DROP TRIGGER IF EXISTS for each dependent, in reverse order.
//  2. CREATE TABLE "new_<table>" from desired.
//  3. INSERT INTO "new_<table>" (common) SELECT common FROM "<table>", where
//     common is the desired columns (generated ones excepted) that are in
//     oldColumns, in desired order; omitted when no column is common.
//  4. DROP TABLE "<table>".
//  5. ALTER TABLE "new_<table>" RENAME TO "<table>".
//  6. CREATE [UNIQUE] INDEX for each of desiredIndexes.
//  7. Each dependent's stored SQL, in the order given, hinted
//     "dependent:<type>:<name>" so that a failure wraps migrate.ErrRebuildBlocked
//     and names the object; after a view, a SELECT that forces SQLite to
//     resolve the view's names (CREATE VIEW does not), with the same hint.
//  8. PRAGMA foreign_key_check, hinted "fk-check".
//
// It returns an error when an enum column's values are needed: desired is
// written without the schema's enums, so give it TEXT columns with a Checks
// entry instead, or render through RenderSchema.
func RebuildStatements(table string, oldColumns []string, desired schema.Table, desiredIndexes []schema.Index, deps []Dependent) ([]migrate.Statement, error) {
	var copyCols [][2]string
	for _, col := range desired.Columns {
		if col.Generated == "" && slices.Contains(oldColumns, col.Name) {
			copyCols = append(copyCols, [2]string{col.Name, col.Name})
		}
	}
	return rebuildScript(table, desired, desiredIndexes, copyCols, nil, deps, deps)
}

// rebuildScript writes the eight steps. copyCols pairs each destination column
// with the source expression's column; drops are removed before the rebuild and
// recreates are created after it.
func rebuildScript(table string, desired schema.Table, indexes []schema.Index, copyCols [][2]string, enums map[string][]string, drops, recreates []Dependent) ([]migrate.Statement, error) {
	tmp := "new_" + table
	var out []migrate.Statement
	for i := len(drops) - 1; i >= 0; i-- {
		d := drops[i]
		switch d.Type {
		case "view":
			out = append(out, migrate.Statement{SQL: "DROP VIEW IF EXISTS " + ident(d.Name)})
		case "trigger":
			out = append(out, migrate.Statement{SQL: "DROP TRIGGER IF EXISTS " + ident(d.Name)})
		default:
			return nil, invalid("dependent %q has the type %q, want view or trigger", d.Name, d.Type)
		}
	}
	create, err := createTableSQL(tmp, desired, enums)
	if err != nil {
		return nil, err
	}
	out = append(out, migrate.Statement{SQL: create})
	if len(copyCols) > 0 {
		dst, src := make([]string, len(copyCols)), make([]string, len(copyCols))
		for i, p := range copyCols {
			dst[i], src[i] = p[0], p[1]
		}
		out = append(out, migrate.Statement{SQL: "INSERT INTO " + ident(tmp) + " (" + identList(dst) + ") SELECT " + identList(src) + " FROM " + ident(table)})
	}
	if desired.Autoincrement {
		// AUTOINCREMENT remembers the highest id ever used in sqlite_sequence,
		// which DROP TABLE removes with the old table. Carry the old value to
		// the new table first, so ids of deleted rows are not handed out again.
		oldName, newName := literalOrEmpty(table), literalOrEmpty(tmp)
		out = append(out,
			migrate.Statement{SQL: "INSERT INTO sqlite_sequence (name, seq) SELECT " + newName + ", seq FROM sqlite_sequence WHERE name = " + oldName +
				" AND NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = " + newName + ")"},
			migrate.Statement{SQL: "UPDATE sqlite_sequence SET seq = (SELECT seq FROM sqlite_sequence WHERE name = " + oldName + ") WHERE name = " + newName +
				" AND seq < (SELECT seq FROM sqlite_sequence WHERE name = " + oldName + ")"},
		)
	}
	out = append(out,
		migrate.Statement{SQL: "DROP TABLE " + ident(table)},
		migrate.Statement{SQL: "ALTER TABLE " + ident(tmp) + " RENAME TO " + ident(table)},
	)
	for _, ix := range indexes {
		s, err := createIndexSQL(table, ix)
		if err != nil {
			return nil, err
		}
		out = append(out, migrate.Statement{SQL: s})
	}
	for _, d := range recreates {
		hint := "dependent:" + d.Type + ":" + d.Name
		out = append(out, migrate.Statement{SQL: strings.TrimRight(strings.TrimSpace(d.SQL), "; \t\r\n"), Hint: hint})
		if d.Type == "view" {
			out = append(out, migrate.Statement{SQL: "SELECT 1 FROM " + ident(d.Name) + " LIMIT 0", Hint: hint})
		}
	}
	out = append(out, migrate.Statement{SQL: "PRAGMA foreign_key_check", Hint: "fk-check"})
	return out, nil
}

// rebuild writes the rebuild of table for the renderer. Without schemas it can
// only mark the table.
func (r *renderer) rebuild(table string, plan []migrate.Change) ([]migrate.Statement, error) {
	if r.to == nil {
		return []migrate.Statement{{Hint: rebuildHint}}, nil
	}
	desired, ok := r.to.Table(table)
	if !ok {
		return nil, fmt.Errorf("%w: %q is not in the desired schema", errNoTable, table)
	}
	oldName := table
	renamedCols := map[string]string{} // new column name -> old
	for _, c := range plan {
		switch {
		case c.Kind == migrate.RenameTable && c.Name == table:
			oldName = c.Table
		case c.Kind == migrate.RenameColumn && c.Table == table:
			if before, err := get[schema.Column](c.Before); err == nil {
				renamedCols[c.Name] = before.Name
			}
		}
	}
	old, ok := r.from.Table(oldName)
	if !ok {
		return nil, fmt.Errorf("%w: %q is not in the starting schema", errNoTable, oldName)
	}
	// A model cannot declare WITHOUT ROWID, STRICT or AUTOINCREMENT yet, so the
	// rebuilt table keeps the ones the table has.
	target := *desired
	target.WithoutRowID = target.WithoutRowID || old.WithoutRowID
	target.Strict = target.Strict || old.Strict
	target.Autoincrement = target.Autoincrement || old.Autoincrement
	var copyCols [][2]string
	for _, col := range desired.Columns {
		if col.Generated != "" {
			continue
		}
		src := col.Name
		if o, ok := renamedCols[col.Name]; ok {
			src = o
		}
		if _, exists := old.Column(src); exists {
			copyCols = append(copyCols, [2]string{col.Name, src})
		}
	}
	drops, recreates, err := r.dependents(table, oldName)
	if err != nil {
		return nil, err
	}
	return rebuildScript(table, target, desired.Indexes, copyCols, r.enums(), drops, recreates)
}

// dependents finds the views and triggers of the starting schema that a rebuild
// of the table has to drop (every one that mentions the table, directly or
// through a view) and the ones to recreate afterwards: those the desired
// schema holds unchanged. A changed or removed one has its own change in the
// plan.
func (r *renderer) dependents(table, oldName string) (drops, recreates []Dependent, err error) {
	names := map[string]string{strings.ToLower(table): table, strings.ToLower(oldName): oldName}
	views := slices.Clone(r.from.Views)
	slices.SortFunc(views, func(a, b schema.View) int { return strings.Compare(a.Name, b.Name) })
	triggers := slices.Clone(r.from.Triggers)
	slices.SortFunc(triggers, func(a, b schema.Trigger) int { return strings.Compare(a.Name, b.Name) })

	viewIn := make([]bool, len(views))
	trigIn := make([]bool, len(triggers))
	for changed := true; changed; {
		changed = false
		for i, v := range views {
			if !viewIn[i] && mentionsAny(v.Query, names) {
				viewIn[i], changed = true, true
				names[strings.ToLower(v.Name)] = v.Name
			}
		}
		for i, t := range triggers {
			if !trigIn[i] && (t.Table == table || t.Table == oldName || mentionsAny(t.Definition, names)) {
				trigIn[i], changed = true, true
			}
		}
	}

	// Views first, a view after the views it reads.
	var inViews []schema.View
	for i, v := range views {
		if viewIn[i] {
			inViews = append(inViews, v)
		}
	}
	ordered := orderViews(inViews)
	for _, v := range ordered {
		drops = append(drops, Dependent{Type: "view", Name: v.Name, Table: v.Name})
		want, ok := lookupView(r.to, v.Name)
		if !ok || want.Materialized || schema.NormalizeExpr(dialect.SQLite, want.Query) != schema.NormalizeExpr(dialect.SQLite, v.Query) {
			continue
		}
		sql, err := createViewSQL(want.Name, *want)
		if err != nil {
			return nil, nil, err
		}
		recreates = append(recreates, Dependent{Type: "view", Name: v.Name, Table: v.Name, SQL: sql})
	}
	for i, t := range triggers {
		if !trigIn[i] {
			continue
		}
		drops = append(drops, Dependent{Type: "trigger", Name: t.Name, Table: t.Table})
		want, ok := lookupTrigger(r.to, t.Name)
		if !ok || want.Table != t.Table || schema.NormalizeExpr(dialect.SQLite, want.Definition) != schema.NormalizeExpr(dialect.SQLite, t.Definition) {
			continue
		}
		sql, err := createTriggerSQL(*want)
		if err != nil {
			return nil, nil, err
		}
		recreates = append(recreates, Dependent{Type: "trigger", Name: t.Name, Table: t.Table, SQL: sql})
	}
	return drops, recreates, nil
}

func lookupView(s *schema.Schema, name string) (*schema.View, bool) {
	for i := range s.Views {
		if s.Views[i].Name == name {
			return &s.Views[i], true
		}
	}
	return nil, false
}

func lookupTrigger(s *schema.Schema, name string) (*schema.Trigger, bool) {
	for i := range s.Triggers {
		if s.Triggers[i].Name == name {
			return &s.Triggers[i], true
		}
	}
	return nil, false
}

// orderViews puts every view after the views its query names. A cycle (which
// SQLite would reject on use) keeps its name order.
func orderViews(views []schema.View) []schema.View {
	remaining := slices.Clone(views)
	var out []schema.View
	for len(remaining) > 0 {
		progressed := false
		var next []schema.View
		for _, v := range remaining {
			blocked := false
			for _, w := range remaining {
				if w.Name != v.Name && mentionsAny(v.Query, map[string]string{strings.ToLower(w.Name): w.Name}) {
					blocked = true
					break
				}
			}
			if blocked {
				next = append(next, v)
			} else {
				out = append(out, v)
				progressed = true
			}
		}
		if !progressed {
			return append(out, next...)
		}
		remaining = next
	}
	return out
}

// literalOrEmpty renders s as an SQL string literal; a NUL byte, which no
// table name holds, is dropped.
func literalOrEmpty(s string) string {
	l, err := literal(strings.ReplaceAll(s, "\x00", ""))
	if err != nil {
		return "''"
	}
	return l
}
