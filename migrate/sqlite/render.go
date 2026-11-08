// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

var lite = dialect.For(dialect.SQLite)

func ident(s string) string { return lite.QuoteIdent(s) }

func identList(xs []string) string {
	q := make([]string, len(xs))
	for i, x := range xs {
		q[i] = ident(x)
	}
	return strings.Join(q, ", ")
}

// unsupported is the error for a change SQLite cannot express.
func unsupported(format string, args ...any) error {
	return &momo.Error{Kind: momo.ErrUnsupported, Op: "migrate/sqlite", Err: fmt.Errorf(format, args...)}
}

// invalid is the error for a change whose payload is malformed.
func invalid(format string, args ...any) error {
	return &momo.Error{Kind: momo.ErrValidation, Op: "migrate/sqlite", Err: fmt.Errorf(format, args...)}
}

// rebuildHint marks the statement Render emits for a table that needs the
// twelve-step rebuild when it has no schema to write the rebuild from.
const rebuildHint = "rebuild"

// Render turns an ordered plan into SQLite statements. The plan must already be
// dependency-ordered by migrate.OrderErr or migrate.OrderWithViews; Render never
// reorders.
//
// A change SQLite's ALTER TABLE can make (rename_table, rename_column,
// create_index, drop_index, create_view, drop_view, create_trigger,
// drop_trigger, and add_column when it meets the restrictions of
// https://www.sqlite.org/lang_altertable.html) becomes one native statement. A
// change it cannot make (alter_column, the primary key, unique, check and
// foreign key changes, drop_column, and an add_column that breaks a
// restriction) needs the rebuild of its table: Render emits one marker
// statement per table, with Hint "rebuild" and no SQL, at the position of the
// table's first such change, and folds every other change to that table into
// it. A marker is not executable; RenderSchema, which Engine.Diff and
// AutoMigrate call, writes the rebuild out in full, and Rebuild writes it from
// a live connection.
//
// Render returns an error wrapping momo.ErrUnsupported for the kinds SQLite has
// no equivalent of: create_schema, create_extension, create_enum,
// alter_enum_add_value, drop_enum, the policy and row-level security kinds and
// the function kinds. Nothing is dropped silently.
func (Dialect) Render(plan []migrate.Change) ([]migrate.Statement, error) {
	return (&renderer{}).render(plan)
}

// RenderSchema is Render with the schema the plan starts from and the one it
// ends at. With them it writes each table rebuild as the plain statements of
// the twelve-step procedure (so a migration file can hold them), decides drop_column by
// what the table text really uses, turns create_enum and drop_enum into
// nothing (an enum is only the CHECK on its TEXT columns, written with the
// column) and rebuilds the tables that use an enum when alter_enum_add_value
// widens it.
func (Dialect) RenderSchema(from, to *schema.Schema, plan []migrate.Change) ([]migrate.Statement, error) {
	if from == nil {
		from = &schema.Schema{}
	}
	if to == nil {
		return nil, invalid("RenderSchema needs the desired schema")
	}
	return (&renderer{from: from, to: to}).render(plan)
}

type renderer struct {
	from, to *schema.Schema // nil for Render
	// oldNames maps the name a renamed table has after the plan's rename_table
	// to the name it has in from.
	oldNames map[string]string
	// createdFKs holds the foreign keys that the ordering moved out of a
	// create_table into add_fk changes (to break a cycle between new tables);
	// SQLite accepts a foreign key to a table that does not exist yet, so they
	// go back into the CREATE TABLE.
	createdFKs map[string][]schema.ForeignKey
}

func describe(c migrate.Change) string {
	if c.Table != "" && c.Table != c.Name {
		return c.Table + "." + c.Name
	}
	return c.Name
}

// tableKinds are the change kinds that alter an existing table's definition.
// When a table is rebuilt they all fold into the rebuild, because the rebuilt
// table is created from the desired definition, which already has them.
var tableKinds = map[migrate.ChangeKind]bool{
	migrate.AddColumn: true, migrate.DropColumn: true, migrate.RenameColumn: true, migrate.AlterColumn: true,
	migrate.AddPrimaryKey: true, migrate.DropPrimaryKey: true, migrate.AddUnique: true, migrate.DropUnique: true,
	migrate.AddCheck: true, migrate.DropCheck: true, migrate.AddFK: true, migrate.DropFK: true,
	migrate.CreateIndex: true, migrate.DropIndex: true,
}

// rebuildPlan says where each table rebuild goes and which changes it absorbs.
type rebuildPlan struct {
	at     map[int][]string // plan index -> tables whose rebuild is written there, sorted
	folded map[int]bool     // plan indexes absorbed into a rebuild
	silent map[int]bool     // plan indexes dropped because their table is dropped
}

func (r *renderer) planRebuilds(plan []migrate.Change) (*rebuildPlan, error) {
	dropped := map[string]bool{}
	r.oldNames = map[string]string{}
	for _, c := range plan {
		switch c.Kind {
		case migrate.DropTable:
			dropped[c.Table] = true
		case migrate.RenameTable:
			r.oldNames[c.Name] = c.Table
		}
	}
	created := map[string]bool{}
	for _, c := range plan {
		if c.Kind == migrate.CreateTable {
			created[c.Table] = true
		}
	}
	r.createdFKs = map[string][]schema.ForeignKey{}
	anchor := map[string]int{}
	note := func(table string, i int) {
		if j, ok := anchor[table]; !ok || i < j {
			anchor[table] = i
		}
	}
	rp := &rebuildPlan{at: map[int][]string{}, folded: map[int]bool{}, silent: map[int]bool{}}
	for i, c := range plan {
		switch {
		case c.Kind == migrate.AddFK && created[c.Table]:
			// The table is created in this plan: the key belongs in its CREATE TABLE.
			if fk, err := get[schema.ForeignKey](c.After); err == nil {
				r.createdFKs[c.Table] = append(r.createdFKs[c.Table], fk)
				rp.silent[i] = true
			}
		case tableKinds[c.Kind] && dropped[c.Table]:
			// The table goes away with these: its foreign keys and indexes
			// need no statement of their own.
			rp.silent[i] = true
		case tableKinds[c.Kind] && r.needsRebuild(c):
			note(c.Table, i)
		case c.Kind == migrate.AlterEnumAddValue && r.to != nil:
			for _, t := range r.to.Tables {
				if !r.existed(t.Name) {
					continue
				}
				for _, col := range t.Columns {
					if col.Enum == c.Name {
						note(t.Name, i)
					}
				}
			}
		}
	}
	for i, c := range plan {
		if tableKinds[c.Kind] && !rp.silent[i] {
			if _, ok := anchor[c.Table]; ok {
				rp.folded[i] = true
			}
		}
	}
	for t, i := range anchor {
		rp.at[i] = append(rp.at[i], t)
	}
	for _, ts := range rp.at {
		slices.Sort(ts)
	}
	return rp, nil
}

// existed reports whether the schema the plan starts from has the table. Without
// schemas every table is assumed to.
func (r *renderer) existed(table string) bool {
	if r.from == nil {
		return true
	}
	_, ok := r.from.Table(r.startName(table))
	return ok
}

// startName is the name table has in the starting schema: its name before a
// rename_table of the plan.
func (r *renderer) startName(table string) string {
	if old, ok := r.oldNames[table]; ok {
		return old
	}
	return table
}

// needsRebuild reports whether c is a change SQLite's ALTER TABLE cannot make.
func (r *renderer) needsRebuild(c migrate.Change) bool {
	switch c.Kind {
	case migrate.AddColumn:
		col, err := get[schema.Column](c.After)
		return err != nil || !nativeAdd(col)
	case migrate.DropColumn:
		return !r.nativeDrop(c)
	case migrate.RenameColumn, migrate.CreateIndex, migrate.DropIndex:
		return false
	}
	return true // alter_column and every constraint change
}

// NeedsRebuild reports whether applying c to the schema from takes a table
// rebuild. AutoMigrate uses it to skip such a change, because the rebuild
// writes the whole desired table and would also apply what AutoMigrate skipped.
func (Dialect) NeedsRebuild(from *schema.Schema, c migrate.Change) bool {
	r := &renderer{from: from, oldNames: map[string]string{}}
	return tableKinds[c.Kind] && r.needsRebuild(c)
}

func (r *renderer) render(plan []migrate.Change) ([]migrate.Statement, error) {
	rp, err := r.planRebuilds(plan)
	if err != nil {
		return nil, err
	}
	var out []migrate.Statement
	for i, c := range plan {
		for _, t := range rp.at[i] {
			stmts, err := r.rebuild(t, plan)
			if err != nil {
				return nil, fmt.Errorf("sqlite render rebuild of %s: %w", t, err)
			}
			out = append(out, stmts...)
		}
		if rp.folded[i] || rp.silent[i] {
			continue
		}
		stmts, err := r.renderChange(c)
		if err != nil {
			return nil, fmt.Errorf("sqlite render %s %s: %w", c.Kind, describe(c), err)
		}
		out = append(out, stmts...)
	}
	return out, nil
}

func one(sql string) []migrate.Statement { return []migrate.Statement{{SQL: sql}} }

func (r *renderer) renderChange(c migrate.Change) ([]migrate.Statement, error) {
	switch c.Kind {
	case migrate.CreateTable:
		t, err := get[schema.Table](c.After)
		if err != nil {
			return nil, err
		}
		if extra := r.createdFKs[t.Name]; len(extra) > 0 {
			t.ForeignKeys = append(slices.Clone(t.ForeignKeys), extra...)
		}
		sql, err := createTableSQL(t.Name, t, r.enums())
		if err != nil {
			return nil, err
		}
		return one(sql), nil
	case migrate.DropTable:
		return one("DROP TABLE " + ident(c.Table)), nil
	case migrate.RenameTable:
		return one("ALTER TABLE " + ident(c.Table) + " RENAME TO " + ident(c.Name)), nil
	case migrate.AddColumn:
		col, err := get[schema.Column](c.After)
		if err != nil {
			return nil, err
		}
		def, err := columnSQL(nil, col)
		if err != nil {
			return nil, err
		}
		return one("ALTER TABLE " + ident(c.Table) + " ADD COLUMN " + def), nil
	case migrate.DropColumn:
		return one("ALTER TABLE " + ident(c.Table) + " DROP COLUMN " + ident(c.Name)), nil
	case migrate.RenameColumn:
		old := c.Name
		if before, err := get[schema.Column](c.Before); err == nil && before.Name != "" {
			old = before.Name
		}
		return one("ALTER TABLE " + ident(c.Table) + " RENAME COLUMN " + ident(old) + " TO " + ident(c.Name)), nil
	case migrate.CreateIndex:
		ix, err := get[schema.Index](c.After)
		if err != nil {
			return nil, err
		}
		sql, err := createIndexSQL(c.Table, ix)
		if err != nil {
			return nil, err
		}
		return one(sql), nil
	case migrate.DropIndex:
		sql := "DROP INDEX "
		if c.IfExists {
			sql += "IF EXISTS "
		}
		return one(sql + ident(c.Name)), nil
	case migrate.CreateView:
		v, err := get[schema.View](c.After)
		if err != nil {
			return nil, err
		}
		sql, err := createViewSQL(c.Name, v)
		if err != nil {
			return nil, err
		}
		return one(sql), nil
	case migrate.DropView:
		return one("DROP VIEW IF EXISTS " + ident(c.Name)), nil
	case migrate.CreateTrigger:
		t, err := get[schema.Trigger](c.After)
		if err != nil {
			return nil, err
		}
		sql, err := createTriggerSQL(t)
		if err != nil {
			return nil, err
		}
		return one(sql), nil
	case migrate.DropTrigger:
		return one("DROP TRIGGER IF EXISTS " + ident(c.Name)), nil
	case migrate.CreateEnum, migrate.DropEnum:
		if r.to != nil {
			return nil, nil // an enum is the CHECK written with each of its columns
		}
		return nil, unsupported("an enum type does not exist on SQLite; RenderSchema writes an enum as a CHECK on its TEXT columns")
	case migrate.AlterEnumAddValue:
		if r.to != nil {
			return nil, nil // the tables that use the enum are rebuilt where it is widened
		}
		return nil, unsupported("an enum type does not exist on SQLite; RenderSchema rebuilds the tables that use it")
	case migrate.CreateSchema:
		return nil, unsupported("SQLite has no schemas")
	case migrate.CreateExtension:
		return nil, unsupported("SQLite has no CREATE EXTENSION; extensions are loaded by the driver")
	case migrate.SetRLS, migrate.CreatePolicy, migrate.DropPolicy:
		return nil, unsupported("SQLite has no row-level security")
	case migrate.CreateFunction, migrate.ReplaceFunction, migrate.DropFunction:
		return nil, unsupported("SQLite has no stored functions")
	}
	return nil, unsupported("the change kind %q has no SQLite rendering (a table change that needs a rebuild reached the native path)", c.Kind)
}

// enums returns the values of each enum of the desired schema, by name.
func (r *renderer) enums() map[string][]string {
	if r.to == nil {
		return nil
	}
	m := make(map[string][]string, len(r.to.Enums))
	for _, e := range r.to.Enums {
		m[e.Name] = e.Values
	}
	return m
}

// get unwraps a Change payload that may be a pointer or a value.
func get[T any](v any) (T, error) {
	switch x := v.(type) {
	case T:
		return x, nil
	case *T:
		if x != nil {
			return *x, nil
		}
	}
	var zero T
	return zero, invalid("unexpected change payload %T, want %T", v, zero)
}

// ------------------------------------------------------------ native ALTER

// nativeAdd reports whether ALTER TABLE ADD COLUMN can add col. SQLite refuses
// a PRIMARY KEY or UNIQUE column (those arrive as their own changes, which
// rebuild), a NOT NULL column without a non-NULL default, a default that is
// CURRENT_TIME, CURRENT_DATE, CURRENT_TIMESTAMP or a parenthesised expression,
// and a stored generated column. A column that needs a CHECK (an enum column)
// or an identity cannot be added either.
func nativeAdd(col schema.Column) bool {
	if col.Generated != "" || col.Enum != "" || col.Identity != "" || col.Comment != "" {
		return false
	}
	if col.Default != "" {
		_, bare := defaultSQL(col.Default)
		if !bare || isCurrentTime(col.Default) {
			return false
		}
	}
	if !col.Nullable && (col.Default == "" || strings.EqualFold(strings.TrimSpace(col.Default), "NULL")) {
		return false
	}
	return true
}

func isCurrentTime(def string) bool {
	switch strings.ToUpper(strings.TrimSpace(def)) {
	case "CURRENT_TIME", "CURRENT_DATE", "CURRENT_TIMESTAMP":
		return true
	}
	return false
}

// nativeDrop reports whether ALTER TABLE DROP COLUMN can drop the column. It
// can only when the table text names the column nowhere but its own
// definition: not in the primary key, a UNIQUE constraint, an index, a CHECK, a
// foreign key (either side), a generated column's expression, a trigger or a
// view. Without the starting schema Render cannot tell, and says no.
func (r *renderer) nativeDrop(c migrate.Change) bool {
	if r.from == nil {
		return false
	}
	t, ok := r.from.Table(r.startName(c.Table))
	if !ok {
		return false
	}
	return !columnUsed(r.from, t, c.Name)
}

// columnUsed reports whether anything in s other than the column's own
// definition names column of table t.
func columnUsed(s *schema.Schema, t *schema.Table, column string) bool {
	if slices.Contains(t.PrimaryKey, column) {
		return true
	}
	for _, u := range t.Uniques {
		if slices.Contains(u.Columns, column) {
			return true
		}
	}
	for _, fk := range t.ForeignKeys {
		if slices.Contains(fk.Columns, column) {
			return true
		}
	}
	for _, other := range s.Tables {
		for _, fk := range other.ForeignKeys {
			if strings.EqualFold(fk.RefTable, t.Name) && slices.Contains(fk.RefColumns, column) {
				return true
			}
		}
	}
	for _, ck := range t.Checks {
		if mentions(ck.Expr, column) {
			return true
		}
	}
	for _, col := range t.Columns {
		if col.Name != column && mentions(col.Generated, column) {
			return true
		}
	}
	for _, ix := range t.Indexes {
		if mentions(ix.Where, column) {
			return true
		}
		for _, p := range ix.Parts {
			if p.Column == column || mentions(p.Expr, column) {
				return true
			}
		}
	}
	for _, tr := range s.Triggers {
		if mentions(tr.Definition, column) && (tr.Table == t.Name || mentions(tr.Definition, t.Name)) {
			return true
		}
	}
	for _, v := range s.Views {
		if mentions(v.Query, t.Name) && mentions(v.Query, column) {
			return true
		}
	}
	return false
}

// mentions reports whether name occurs in src as a word or quoted identifier,
// compared without regard to case. Text inside a string literal does not count.
func mentions(src, name string) bool {
	if src == "" || name == "" {
		return false
	}
	found := map[string]bool{}
	collectNames(src, map[string]string{strings.ToLower(name): name}, found, 0)
	return found[name]
}

// ------------------------------------------------------------ DDL builders

// createTableSQL renders CREATE TABLE for t under name, with every constraint
// inline. enums holds the values of the enum columns' enums.
func createTableSQL(name string, t schema.Table, enums map[string][]string) (string, error) {
	if t.Autoincrement && (t.WithoutRowID || len(t.PrimaryKey) != 1) {
		return "", unsupported("table %q is AUTOINCREMENT; that needs an INTEGER primary key of one column on a rowid table", t.Name)
	}
	var parts []string
	for _, col := range t.Columns {
		def, err := columnSQL(&t, col)
		if err != nil {
			return "", err
		}
		if t.Autoincrement && col.Name == t.PrimaryKey[0] {
			if !strings.EqualFold(strings.TrimSpace(col.Type), "INTEGER") {
				return "", unsupported("table %q is AUTOINCREMENT but column %q is %q, not INTEGER", t.Name, col.Name, col.Type)
			}
			def += " PRIMARY KEY AUTOINCREMENT" // only the column form takes AUTOINCREMENT
		}
		parts = append(parts, def)
	}
	if len(t.PrimaryKey) > 0 && !t.Autoincrement {
		parts = append(parts, "PRIMARY KEY ("+identList(t.PrimaryKey)+")")
	}
	for _, u := range t.Uniques {
		parts = append(parts, constraintName(u.Name)+"UNIQUE ("+identList(u.Columns)+")")
	}
	for _, col := range t.Columns {
		if col.Enum == "" {
			continue
		}
		values, ok := enums[col.Enum]
		if !ok || len(values) == 0 {
			return "", invalid("column %q uses the enum %q, whose values are not known here; render through RenderSchema", col.Name, col.Enum)
		}
		lits := make([]string, len(values))
		for i, v := range values {
			l, err := literal(v)
			if err != nil {
				return "", err
			}
			lits[i] = l
		}
		parts = append(parts, "CHECK ("+ident(col.Name)+" IN ("+strings.Join(lits, ", ")+"))")
	}
	for _, ck := range t.Checks {
		parts = append(parts, constraintName(ck.Name)+"CHECK ("+ck.Expr+")")
	}
	for _, fk := range t.ForeignKeys {
		def, err := fkSQL(fk)
		if err != nil {
			return "", err
		}
		parts = append(parts, def)
	}
	sql := "CREATE TABLE " + ident(name) + " (" + strings.Join(parts, ", ") + ")"
	switch {
	case t.WithoutRowID && t.Strict:
		sql += " WITHOUT ROWID, STRICT"
	case t.WithoutRowID:
		sql += " WITHOUT ROWID"
	case t.Strict:
		sql += " STRICT"
	}
	if t.Partition != nil || t.PartitionOf != nil || t.RLS != nil && (t.RLS.Enabled || t.RLS.Forced) {
		return "", unsupported("table %q uses partitioning or row-level security, which SQLite does not have", t.Name)
	}
	return sql, nil
}

func constraintName(name string) string {
	if name == "" {
		return ""
	}
	return "CONSTRAINT " + ident(name) + " "
}

// columnSQL renders one column definition. t is the table the column belongs to,
// nil when the column is added to an existing table.
func columnSQL(t *schema.Table, col schema.Column) (string, error) {
	if col.Comment != "" {
		return "", unsupported("column %q has a comment; SQLite has no column comments", col.Name)
	}
	switch col.Identity {
	case "":
	case "always", "by_default":
		// SQLite has one kind of generated key: an INTEGER PRIMARY KEY is the
		// rowid and is assigned when the insert gives none.
		if t == nil || t.WithoutRowID || len(t.PrimaryKey) != 1 || t.PrimaryKey[0] != col.Name || !strings.EqualFold(strings.TrimSpace(col.Type), "INTEGER") {
			return "", unsupported("column %q is an identity column; on SQLite that needs an INTEGER column that is the whole primary key of a rowid table", col.Name)
		}
	default:
		return "", unsupported("column %q: the identity kind %q has no SQLite equivalent", col.Name, col.Identity)
	}
	if col.Enum != "" && !strings.EqualFold(strings.TrimSpace(col.Type), "TEXT") {
		return "", unsupported("column %q: an enum column is a TEXT column with a CHECK on SQLite, not %q", col.Name, col.Type)
	}
	var b strings.Builder
	b.WriteString(ident(col.Name))
	if col.Type != "" {
		b.WriteString(" " + col.Type)
	}
	if col.Collate != "" {
		b.WriteString(" COLLATE " + ident(col.Collate))
	}
	if col.Generated != "" {
		b.WriteString(" GENERATED ALWAYS AS (" + col.Generated + ") STORED")
	} else if col.Default != "" {
		d, _ := defaultSQL(col.Default)
		b.WriteString(" DEFAULT " + d)
	}
	if !col.Nullable {
		b.WriteString(" NOT NULL")
	}
	return b.String(), nil
}

// defaultSQL renders a default expression for a DEFAULT clause. A number, a
// string, a blob literal, NULL, TRUE, FALSE and CURRENT_* are written as they
// are (bare is true); anything else is wrapped in parentheses, which SQLite
// requires for an expression.
func defaultSQL(def string) (sql string, bare bool) {
	def = strings.TrimSpace(def)
	toks := tokenize(def)
	simple := false
	switch {
	case len(toks) == 1 && toks[0].kind == tokString:
		simple = true
	case len(toks) == 1 && toks[0].kind == tokWord:
		w := toks[0].upper()
		simple = isNumber(w) || w == "NULL" || w == "TRUE" || w == "FALSE" || w == "CURRENT_TIME" || w == "CURRENT_DATE" || w == "CURRENT_TIMESTAMP"
	case len(toks) == 2 && toks[0].kind == tokPunct && (toks[0].text == "-" || toks[0].text == "+") && toks[1].kind == tokWord:
		simple = isNumber(toks[1].text)
	case len(toks) == 2 && toks[0].kind == tokWord && strings.EqualFold(toks[0].text, "x") && toks[1].kind == tokString:
		simple = true
	}
	if simple {
		return def, true
	}
	if len(toks) == 1 && toks[0].kind == tokGroup {
		return def, false
	}
	return "(" + def + ")", false
}

// isNumber reports whether w is an unsigned decimal, hexadecimal or floating
// point literal.
func isNumber(w string) bool {
	if w == "" || w[0] < '0' || w[0] > '9' {
		return false
	}
	for _, c := range w {
		if !(c >= '0' && c <= '9' || c == '.' || c == 'x' || c == 'X' || c == 'e' || c == 'E' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' || c == '_') {
			return false
		}
	}
	return true
}

// literal renders s as an SQL string literal.
func literal(s string) (string, error) {
	if strings.ContainsRune(s, 0) {
		return "", invalid("a string literal cannot contain a NUL byte")
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'", nil
}

var fkActions = map[string]bool{"CASCADE": true, "SET NULL": true, "SET DEFAULT": true, "RESTRICT": true, "NO ACTION": true}

func fkAct(a string) (string, error) {
	n := strings.ToUpper(strings.Join(strings.Fields(a), " "))
	if n != "" && !fkActions[n] {
		return "", invalid("unknown foreign key action %q", a)
	}
	return n, nil
}

func fkSQL(fk schema.ForeignKey) (string, error) {
	del, err := fkAct(fk.OnDelete)
	if err != nil {
		return "", err
	}
	upd, err := fkAct(fk.OnUpdate)
	if err != nil {
		return "", err
	}
	s := constraintName(fk.Name) + "FOREIGN KEY (" + identList(fk.Columns) + ") REFERENCES " + ident(fk.RefTable)
	if len(fk.RefColumns) > 0 {
		s += " (" + identList(fk.RefColumns) + ")"
	}
	if del != "" {
		s += " ON DELETE " + del
	}
	if upd != "" {
		s += " ON UPDATE " + upd
	}
	return s, nil
}

func createIndexSQL(table string, ix schema.Index) (string, error) {
	if ix.Method != "" && !strings.EqualFold(ix.Method, "btree") {
		return "", unsupported("index %q uses the access method %q; SQLite indexes are b-trees", ix.Name, ix.Method)
	}
	if len(ix.With) > 0 {
		return "", unsupported("index %q has storage parameters, which SQLite indexes do not take", ix.Name)
	}
	parts := make([]string, len(ix.Parts))
	for i, p := range ix.Parts {
		var s string
		switch {
		case p.Expr != "":
			s = "(" + p.Expr + ")"
		case p.Column != "":
			s = ident(p.Column)
		default:
			return "", invalid("index %q part %d has neither a column nor an expression", ix.Name, i)
		}
		if p.OpClass != "" || p.Nulls != "" {
			return "", unsupported("index %q part %d sets an operator class or NULLS ordering, which SQLite indexes do not have", ix.Name, i)
		}
		if p.Desc {
			s += " DESC"
		}
		parts[i] = s
	}
	if len(parts) == 0 {
		return "", invalid("index %q has no parts", ix.Name)
	}
	sql := "CREATE "
	if ix.Unique {
		sql += "UNIQUE "
	}
	sql += "INDEX " + ident(ix.Name) + " ON " + ident(table) + " (" + strings.Join(parts, ", ") + ")"
	if ix.Where != "" {
		sql += " WHERE " + ix.Where
	}
	return sql, nil
}

func createViewSQL(name string, v schema.View) (string, error) {
	if v.Materialized {
		return "", unsupported("view %q is materialized; SQLite has no materialized views", name)
	}
	if len(v.Indexes) > 0 {
		return "", unsupported("view %q has indexes; SQLite cannot index a view", name)
	}
	q := strings.TrimRight(strings.TrimSpace(v.Query), "; \t\r\n")
	if q == "" {
		return "", invalid("view %q has no query", name)
	}
	return "CREATE VIEW " + ident(name) + " AS " + q, nil
}

func createTriggerSQL(t schema.Trigger) (string, error) {
	def := strings.TrimRight(strings.TrimSpace(t.Definition), "; \t\r\n")
	if def == "" {
		return "", invalid("trigger %q has no Definition; on SQLite a trigger is its CREATE TRIGGER text", t.Name)
	}
	return def, nil
}

var errNoTable = errors.New("the table is not in the schema")
