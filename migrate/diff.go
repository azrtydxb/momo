// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// Diff compares the snapshot schema (from) with the desired schema (to). It is
// pure: no I/O, and neither argument is modified. Comparisons use normalized
// types and expressions, so differences of spelling are no diff; the Before
// and After values of the returned changes hold the caller's original
// spelling.
//
// The output is deterministic but only grouped, not dependency-ordered:
// Order (order.go) sorts it into an executable sequence. Groups appear in
// this order: schemas, extensions, enums, tables (with their columns,
// constraints and indexes), views, functions, triggers, policies.
//
// A new table is one create_table change that carries the whole table
// (columns, primary key, uniques, checks, foreign keys) in After, followed by
// create_index and set_rls changes for its indexes and row-level security.
//
// Errors: ErrDestructive when a destructive change exists and
// opts.AllowDestructive is false; ErrStagedRequired when a NOT NULL column
// without default is added to (or an existing column becomes NOT NULL on) an
// existing table and opts.Stage is false. With opts.Stage the NOT NULL part is
// left out and a hint describes the remaining steps; Engine.Diff records the
// column as nullable in the new snapshot, so a later Diff with
// opts.EnforceNotNull emits the SET NOT NULL once the data is backfilled.
//
// Drops are never paired with adds into renames: only opts.RenameTables and
// opts.RenameColumns produce renames. A hint names likely rename candidates.
//
// An added NOT NULL column that is a rename candidate (same table and type as
// a dropped column) follows the staging rule like any other; its refusal names
// the rename flag.
//
// An ADD COLUMN on an existing table whose default calls a volatile function
// (random(), gen_random_uuid(), ...) gets a risk hint: PostgreSQL rewrites the
// table under ACCESS EXCLUSIVE.
func Diff(from, to *schema.Schema, opts DiffOptions) (changes []Change, hints []string, err error) {
	if from == nil {
		from = &schema.Schema{}
	}
	if to == nil {
		to = &schema.Schema{}
	}
	d := &differ{
		d:        to.Dialect,
		opts:     opts,
		usedRT:   map[string]bool{},
		usedRC:   map[string]bool{},
		hintSet:  map[string]bool{},
		fromNorm: schema.Normalized(from),
		toNorm:   schema.Normalized(to),
	}
	if d.d == "" {
		d.d = from.Dialect
	}
	d.run(from, to)
	for _, k := range sortedMapKeys(opts.RenameTables) {
		if !d.usedRT[k] {
			d.errs = append(d.errs, fmt.Errorf("migrate: rename table %s=%s matches no dropped and added table pair", k, opts.RenameTables[k]))
		}
	}
	for _, k := range sortedMapKeys(opts.RenameColumns) {
		if !d.usedRC[k] {
			d.errs = append(d.errs, fmt.Errorf("migrate: rename column %s=%s matches no dropped and added column pair", k, opts.RenameColumns[k]))
		}
	}
	if len(d.destructive) > 0 && !opts.AllowDestructive {
		d.errs = append(d.errs, fmt.Errorf("%w: %s", ErrDestructive, strings.Join(d.destructive, ", ")))
	}
	if len(d.staged) > 0 && !opts.Stage {
		d.errs = append(d.errs, fmt.Errorf("%w: %s", ErrStagedRequired, strings.Join(d.staged, ", ")))
	}
	if len(d.errs) > 0 {
		return nil, nil, errors.Join(d.errs...)
	}
	hints = d.hints
	sort.Strings(hints)
	return d.changes, hints, nil
}

type differ struct {
	d    dialect.Name
	opts DiffOptions

	fromNorm, toNorm *schema.Schema

	changes     []Change
	hints       []string
	hintSet     map[string]bool
	errs        []error
	destructive []string
	staged      []string
	usedRT      map[string]bool
	usedRC      map[string]bool
	tblRen      map[string]string            // from table key -> to table key
	colRen      map[string]map[string]string // from table key -> old column -> new column
}

func (d *differ) emit(c Change) {
	if c.Destructive {
		d.destructive = append(d.destructive, fmt.Sprintf("%s %s", c.Kind, qualified(c.Schema, c.Table, c.Name)))
	}
	d.changes = append(d.changes, c)
}

func (d *differ) hint(h string) {
	if !d.hintSet[h] {
		d.hintSet[h] = true
		d.hints = append(d.hints, h)
	}
}

func qualified(schemaName, table, name string) string {
	var parts []string
	for _, p := range []string{schemaName, table, name} {
		if p != "" && (len(parts) == 0 || parts[len(parts)-1] != p) {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ".")
}

func (d *differ) run(from, to *schema.Schema) {
	d.namespaces(from, to)
	d.enums()
	d.tables(from, to)
	d.views(from, to)
	d.functions(from, to)
	d.triggers(from, to)
	d.policies(from, to)
}

// ---------------------------------------------------------------- helpers

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (d *differ) typ(t string) string  { return schema.NormalizeType(d.d, t) }
func (d *differ) expr(e string) string { return schema.NormalizeExpr(d.d, e) }

func tableKey(t *schema.Table) string { return t.Schema + "\x00" + t.Name }

func cp[T any](v T) *T { return &v }

// ------------------------------------------------- schemas and extensions

func (d *differ) namespaces(from, to *schema.Schema) {
	have := map[string]bool{}
	for _, s := range d.fromNorm.Schemas {
		have[s.Name] = true
	}
	want := map[string]bool{}
	for _, s := range d.toNorm.Schemas {
		want[s.Name] = true
		if !have[s.Name] {
			d.emit(Change{Kind: CreateSchema, Schema: s.Name, Name: s.Name, After: cp(s)})
		}
	}
	for _, s := range d.fromNorm.Schemas {
		if !want[s.Name] {
			d.hint(fmt.Sprintf("schema %s is no longer managed; it is not dropped", s.Name))
		}
	}
	haveExt := map[string]bool{}
	for _, e := range d.fromNorm.Extensions {
		haveExt[e.Name] = true
	}
	wantExt := map[string]bool{}
	for _, e := range d.toNorm.Extensions {
		wantExt[e.Name] = true
		if !haveExt[e.Name] {
			d.emit(Change{Kind: CreateExtension, Name: e.Name, After: cp(e)})
		}
	}
	for _, e := range d.fromNorm.Extensions {
		if !wantExt[e.Name] {
			d.hint(fmt.Sprintf("extension %s is no longer managed; it is not dropped", e.Name))
		}
	}
}

// ------------------------------------------------------------------ enums

func (d *differ) enums() {
	from := map[string]*schema.Enum{}
	for i := range d.fromNorm.Enums {
		from[d.fromNorm.Enums[i].Name] = &d.fromNorm.Enums[i]
	}
	to := map[string]*schema.Enum{}
	for i := range d.toNorm.Enums {
		to[d.toNorm.Enums[i].Name] = &d.toNorm.Enums[i]
	}
	for _, e := range d.toNorm.Enums {
		old, ok := from[e.Name]
		if !ok {
			d.emit(Change{Kind: CreateEnum, Name: e.Name, After: cp(e)})
			continue
		}
		for _, v := range old.Values {
			if !slices.Contains(e.Values, v) {
				d.errs = append(d.errs, fmt.Errorf("migrate: enum %s loses value %q; removing enum values is not supported, write a custom migration", e.Name, v))
			}
		}
		for _, v := range e.Values {
			if !slices.Contains(old.Values, v) {
				d.emit(Change{Kind: AlterEnumAddValue, Name: e.Name, Before: cp(*old), After: v})
			}
		}
	}
	for _, e := range d.fromNorm.Enums {
		if _, ok := to[e.Name]; !ok {
			d.emit(Change{Kind: DropEnum, Name: e.Name, Before: cp(e), Destructive: true})
		}
	}
}

// ----------------------------------------------------------------- tables

func (d *differ) tables(from, to *schema.Schema) {
	fromN := map[string]*schema.Table{}
	fromO := map[string]*schema.Table{}
	for i := range d.fromNorm.Tables {
		fromN[tableKey(&d.fromNorm.Tables[i])] = &d.fromNorm.Tables[i]
	}
	for i := range from.Tables {
		fromO[tableKey(&from.Tables[i])] = &from.Tables[i]
	}
	toN := map[string]*schema.Table{}
	toO := map[string]*schema.Table{}
	for i := range d.toNorm.Tables {
		toN[tableKey(&d.toNorm.Tables[i])] = &d.toNorm.Tables[i]
	}
	for i := range to.Tables {
		toO[tableKey(&to.Tables[i])] = &to.Tables[i]
	}

	// Resolve explicit table renames: from key -> to key. A rename flag key is
	// "table" or "schema.table"; the new table lives in the old table's schema.
	// Tables are visited in sorted order, so the result does not depend on map order.
	renamed := map[string]string{}
	renamedTo := map[string]bool{}
	for _, flag := range sortedMapKeys(d.opts.RenameTables) {
		newName := d.opts.RenameTables[flag]
		var matches []*schema.Table
		for i := range d.fromNorm.Tables {
			t := &d.fromNorm.Tables[i]
			if flag == t.Name || (t.Schema != "" && flag == t.Schema+"."+t.Name) {
				matches = append(matches, t)
			}
		}
		if len(matches) > 1 {
			d.errs = append(d.errs, fmt.Errorf("migrate: rename table %s=%s is ambiguous: the table exists in several schemas; qualify it as schema.table", flag, newName))
			d.usedRT[flag] = true
			continue
		}
		if len(matches) == 0 {
			continue // reported as an unmatched rename flag
		}
		fk := tableKey(matches[0])
		tk := matches[0].Schema + "\x00" + newName
		_, oldKept := toN[fk]
		_, newExisted := fromN[tk]
		if _, newWanted := toN[tk]; !newWanted || oldKept || newExisted || fk == tk {
			continue
		}
		d.usedRT[flag] = true
		renamed[fk] = tk
		renamedTo[tk] = true
	}
	d.tblRen = renamed

	// Resolve column renames for every table pair up front, so that constraints
	// elsewhere (foreign keys) can be remapped through them.
	d.colRen = map[string]map[string]string{}
	for _, t := range d.fromNorm.Tables {
		k := tableKey(&t)
		tk := k
		if r, ok := renamed[k]; ok {
			tk = r
		}
		if tn, ok := toN[tk]; ok {
			d.colRen[k] = d.resolveColumnRenames(fromN[k], tn)
		}
	}

	// Drops (in sorted order) and rename candidates.
	var dropped, created []*schema.Table
	for _, t := range d.fromNorm.Tables {
		k := tableKey(&t)
		if _, ok := toN[k]; ok {
			continue
		}
		if _, ok := renamed[k]; ok {
			continue
		}
		dropped = append(dropped, fromN[k])
	}
	for _, t := range d.toNorm.Tables {
		k := tableKey(&t)
		if _, ok := fromN[k]; ok || renamedTo[k] {
			continue
		}
		created = append(created, toN[k])
	}
	for _, dt := range dropped {
		for _, ct := range created {
			if dt.Schema == ct.Schema && d.sameColumns(dt, ct) {
				d.hint(fmt.Sprintf("possible rename: drop+create of table %s -> %s; pass --rename-table %s=%s to rename", dt.Name, ct.Name, dt.Name, ct.Name))
			}
		}
	}

	// Renames first, then per-table diffs in sorted order of the new key.
	for _, k := range sortedMapKeys(renamed) {
		ft, tt := fromO[k], toO[renamed[k]]
		d.emit(Change{Kind: RenameTable, Schema: tt.Schema, Table: ft.Name, Name: tt.Name, Before: cp(*ft), After: cp(*tt)})
	}
	renamedFrom := map[string]string{}
	for a, b := range renamed {
		renamedFrom[b] = a
	}
	for _, t := range d.toNorm.Tables {
		k := tableKey(&t)
		switch {
		case renamedTo[k]:
			fk := renamedFrom[k]
			d.diffTable(fk, fromN[fk], toN[k], fromO[fk], toO[k])
		case fromN[k] != nil:
			d.diffTable(k, fromN[k], toN[k], fromO[k], toO[k])
		default:
			d.createTable(toO[k])
		}
	}
	for _, t := range dropped {
		o := fromO[tableKey(t)]
		// The table's own foreign keys are dropped explicitly first, so that Order
		// can break a cycle between two dropped tables.
		for i := range t.ForeignKeys {
			fk := t.ForeignKeys[i]
			d.emit(Change{Kind: DropFK, Schema: o.Schema, Table: o.Name, Name: fk.Name, Before: cp(fk)})
		}
		d.emit(Change{Kind: DropTable, Schema: o.Schema, Table: o.Name, Name: o.Name, Before: cp(*o), Destructive: true})
	}
}

func (d *differ) sameColumns(a, b *schema.Table) bool {
	if len(a.Columns) == 0 || len(a.Columns) != len(b.Columns) {
		return false
	}
	for i := range a.Columns {
		if a.Columns[i].Name != b.Columns[i].Name || d.typ(a.Columns[i].Type) != d.typ(b.Columns[i].Type) {
			return false
		}
	}
	return true
}

func (d *differ) createTable(t *schema.Table) {
	d.emit(Change{Kind: CreateTable, Schema: t.Schema, Table: t.Name, Name: t.Name, After: cp(*t)})
	for i := range t.Indexes {
		ix := &t.Indexes[i]
		d.emit(Change{Kind: CreateIndex, Schema: t.Schema, Table: t.Name, Name: ix.Name, After: cp(*ix)})
	}
	if rls := effectiveRLS(t.RLS); rls != (schema.RLS{}) {
		d.emit(Change{Kind: SetRLS, Schema: t.Schema, Table: t.Name, Name: t.Name, After: cp(rls)})
	}
}

func effectiveRLS(r *schema.RLS) schema.RLS {
	if r == nil {
		return schema.RLS{}
	}
	return *r
}

func (d *differ) diffTable(fk string, fn, tn, fo, to *schema.Table) {
	renameTo := d.colRen[fk]
	fn = d.remapFrom(fn, renameTo)
	if !reflect.DeepEqual(fn.Partition, tn.Partition) || !reflect.DeepEqual(fn.PartitionOf, tn.PartitionOf) {
		d.errs = append(d.errs, fmt.Errorf("migrate: table %s changes its partitioning; this cannot be altered in place, write a custom migration", tn.Name))
	}
	if fn.WithoutRowID != tn.WithoutRowID {
		d.errs = append(d.errs, fmt.Errorf("migrate: table %s changes WITHOUT ROWID; write a custom migration", tn.Name))
	}
	d.columns(fn, tn, fo, to, renameTo)
	d.primaryKey(fn, tn, to)
	d.uniques(fn, tn, to)
	d.checks(fn, tn, to)
	d.foreignKeys(fn, tn, to)
	d.indexes(fn, tn, to)
	if effectiveRLS(fn.RLS) != effectiveRLS(tn.RLS) {
		d.emit(Change{Kind: SetRLS, Schema: to.Schema, Table: to.Name, Name: to.Name, Before: cp(effectiveRLS(fo.RLS)), After: cp(effectiveRLS(to.RLS))})
	}
}

// ---------------------------------------------------------------- columns

// columnAttrsDiffer compares normalized columns. A collation change is not
// destructive, but it can change the ordering a unique index enforces and so
// make existing rows collide; the rendering step must keep that in mind and
// the unique index may need a rebuild.
func columnAttrsDiffer(a, b *schema.Column) bool {
	return a.Type != b.Type || a.Nullable != b.Nullable || a.Default != b.Default ||
		a.Identity != b.Identity || a.Generated != b.Generated || a.Enum != b.Enum ||
		a.Collate != b.Collate || a.Comment != b.Comment
}

func colByName(t *schema.Table) map[string]*schema.Column {
	m := make(map[string]*schema.Column, len(t.Columns))
	for i := range t.Columns {
		m[t.Columns[i].Name] = &t.Columns[i]
	}
	return m
}

// volatileFuncs are the built-in and common extension functions whose use in
// a column default makes ADD COLUMN rewrite the table.
var volatileFuncs = []string{
	"random", "random_normal", "setseed", "clock_timestamp", "timeofday",
	"gen_random_uuid", "uuidv4", "uuidv7", "uuid_generate_v1", "uuid_generate_v1mc", "uuid_generate_v4",
	"gen_random_bytes", "nextval", "txid_current", "pg_current_xact_id",
}

var volatileCall = regexp.MustCompile(`(?i)\b(` + strings.Join(volatileFuncs, "|") + `)\s*\(`)

var quotedLiteral = regexp.MustCompile(`'(?:[^']|'')*'`)

// volatileDefault returns the name of the first known volatile function the
// default expression calls, or "" when it calls none.
func volatileDefault(expr string) string {
	expr = quotedLiteral.ReplaceAllString(expr, "''")
	if m := volatileCall.FindStringSubmatch(expr); m != nil {
		return strings.ToLower(m[1])
	}
	return ""
}

func needsStaging(c *schema.Column) bool {
	return !c.Nullable && c.Default == "" && c.Identity == "" && c.Generated == ""
}

func (d *differ) columns(fn, tn, fo, to *schema.Table, renameTo map[string]string) {
	fromN, toN := colByName(fn), colByName(tn)
	fromO, toO := colByName(fo), colByName(to)

	renamedTo := map[string]bool{}
	for _, n := range renameTo {
		renamedTo[n] = true
	}

	// Unpaired drops and adds, for rename hints.
	var droppedCols, addedCols []string
	for _, c := range fn.Columns {
		if _, ok := toN[c.Name]; !ok && renameTo[c.Name] == "" {
			droppedCols = append(droppedCols, c.Name)
		}
	}
	for _, c := range tn.Columns {
		if _, ok := fromN[c.Name]; !ok && !renamedTo[c.Name] {
			addedCols = append(addedCols, c.Name)
		}
	}
	candidate := map[string]bool{}
	for _, dc := range droppedCols {
		for _, ac := range addedCols {
			if d.typ(fromN[dc].Type) == d.typ(toN[ac].Type) {
				candidate[ac] = true
				d.hint(fmt.Sprintf("possible rename: drop+add of %s.%s -> %s.%s; pass --rename-column %s.%s=%s to rename", to.Name, dc, to.Name, ac, to.Name, dc, ac))
			}
		}
	}

	// Renames (in table column order), each followed by an alter if other attributes changed.
	for _, c := range fn.Columns {
		nn, ok := renameTo[c.Name]
		if !ok {
			continue
		}
		d.emit(Change{Kind: RenameColumn, Schema: to.Schema, Table: to.Name, Name: nn, Before: cp(*fromO[c.Name]), After: cp(*toO[nn])})
		d.alterColumn(to, &c, toN[nn], fromO[c.Name], toO[nn])
	}
	// Adds in desired column order.
	for _, c := range tn.Columns {
		if _, ok := fromN[c.Name]; ok || renamedTo[c.Name] {
			continue
		}
		col := toO[c.Name]
		if needsStaging(&c) {
			if !d.opts.Stage {
				entry := fmt.Sprintf("%s.%s", to.Name, c.Name)
				if candidate[c.Name] {
					entry += " (possible rename: pass --rename-column)"
				}
				d.staged = append(d.staged, entry)
			} else {
				staged := *col
				staged.Nullable = true
				d.emit(Change{Kind: AddColumn, Schema: to.Schema, Table: to.Name, Name: c.Name, After: &staged})
				d.hint(fmt.Sprintf("staged: %s.%s was added as nullable; backfill it, then run migrate diff with --enforce-not-null to emit ALTER TABLE %s ALTER COLUMN %s SET NOT NULL", to.Name, c.Name, to.Name, c.Name))
				continue
			}
		}
		if v := volatileDefault(col.Default); v != "" && d.d == dialect.Postgres {
			d.hint(fmt.Sprintf("risk: %s.%s adds a column with the volatile default %s(); PostgreSQL rewrites the table under ACCESS EXCLUSIVE; add the column nullable, backfill in batches, then set the default", to.Name, c.Name, v))
		}
		d.emit(Change{Kind: AddColumn, Schema: to.Schema, Table: to.Name, Name: c.Name, After: cp(*col)})
	}
	// Alters for columns kept under their name.
	for _, c := range tn.Columns {
		if oc, ok := fromN[c.Name]; ok {
			d.alterColumn(to, oc, &c, fromO[c.Name], toO[c.Name])
		}
	}
	// Drops in snapshot column order.
	for _, c := range fn.Columns {
		if _, ok := toN[c.Name]; ok || renameTo[c.Name] != "" {
			continue
		}
		d.emit(Change{Kind: DropColumn, Schema: to.Schema, Table: to.Name, Name: c.Name, Before: cp(*fromO[c.Name]), Destructive: true})
	}
}

// alterColumn emits an alter_column when the normalized attributes differ.
// a and b are the normalized columns, ao and bo the originals.
func (d *differ) alterColumn(t *schema.Table, a, b *schema.Column, ao, bo *schema.Column) {
	na, nb := *a, *b
	na.Type, nb.Type = d.typ(a.Type), d.typ(b.Type)
	na.Default, nb.Default = d.expr(a.Default), d.expr(b.Default)
	na.Generated, nb.Generated = d.expr(a.Generated), d.expr(b.Generated)
	if !columnAttrsDiffer(&na, &nb) {
		return
	}
	after := *bo
	if a.Nullable && !b.Nullable && needsStaging(b) && !d.opts.EnforceNotNull {
		if !d.opts.Stage {
			d.staged = append(d.staged, fmt.Sprintf("%s.%s", t.Name, b.Name))
		} else {
			after.Nullable = true
			nb.Nullable = true
			d.hint(fmt.Sprintf("staged: %s.%s stays nullable; backfill it, then run migrate diff with --enforce-not-null to emit ALTER TABLE %s ALTER COLUMN %s SET NOT NULL", t.Name, b.Name, t.Name, b.Name))
			if !columnAttrsDiffer(&na, &nb) {
				return
			}
		}
	}
	d.emit(Change{Kind: AlterColumn, Schema: t.Schema, Table: t.Name, Name: b.Name, Before: cp(*ao), After: &after, Destructive: na.Type != nb.Type})
}

// ------------------------------------------------------------ constraints

func (d *differ) primaryKey(fn, tn, to *schema.Table) {
	if slices.Equal(fn.PrimaryKey, tn.PrimaryKey) {
		return
	}
	name := to.Name + "_pkey"
	if len(fn.PrimaryKey) > 0 {
		d.emit(Change{Kind: DropPrimaryKey, Schema: to.Schema, Table: to.Name, Name: name, Before: slices.Clone(fn.PrimaryKey)})
	}
	if len(tn.PrimaryKey) > 0 {
		d.emit(Change{Kind: AddPrimaryKey, Schema: to.Schema, Table: to.Name, Name: name, After: slices.Clone(tn.PrimaryKey)})
	}
}

func (d *differ) uniques(fn, tn, to *schema.Table) {
	diffNamed(d, fn.Uniques, tn.Uniques, to, func(u schema.Unique) string { return u.Name },
		func(a, b schema.Unique) bool { return slices.Equal(a.Columns, b.Columns) }, DropUnique, AddUnique)
}

func (d *differ) checks(fn, tn, to *schema.Table) {
	diffNamed(d, fn.Checks, tn.Checks, to, func(c schema.Check) string { return c.Name },
		func(a, b schema.Check) bool { return a.Expr == b.Expr }, DropCheck, AddCheck)
}

func (d *differ) foreignKeys(fn, tn, to *schema.Table) {
	diffNamed(d, fn.ForeignKeys, tn.ForeignKeys, to, func(f schema.ForeignKey) string { return f.Name },
		func(a, b schema.ForeignKey) bool {
			return slices.Equal(a.Columns, b.Columns) && a.RefTable == b.RefTable &&
				slices.Equal(a.RefColumns, b.RefColumns) &&
				fkAction(a.OnDelete) == fkAction(b.OnDelete) && fkAction(a.OnUpdate) == fkAction(b.OnUpdate)
		}, DropFK, AddFK)
}

func fkAction(a string) string {
	a = strings.ToUpper(strings.Join(strings.Fields(a), " "))
	if a == "NO ACTION" {
		return ""
	}
	return a
}

func (d *differ) indexes(fn, tn, to *schema.Table) {
	diffNamed(d, fn.Indexes, tn.Indexes, to, func(i schema.Index) string { return i.Name },
		indexEqual, DropIndex, CreateIndex)
}

func indexEqual(a, b schema.Index) bool {
	a.Concurrently, b.Concurrently = false, false
	if a.Method == "btree" {
		a.Method = ""
	}
	if b.Method == "btree" {
		b.Method = ""
	}
	return reflect.DeepEqual(a, b)
}

// diffNamed compares two collections of named table objects, which are
// already normalized and sorted by name. A changed object is a drop followed
// by an add; a dropped one is a drop; a new one is an add. The emitted Before
// and After values are the normalized objects.
func diffNamed[T any](d *differ, from, to []T, t *schema.Table, name func(T) string, equal func(a, b T) bool, drop, add ChangeKind) {
	byName := make(map[string]T, len(from))
	for _, o := range from {
		byName[name(o)] = o
	}
	for _, o := range from {
		nw, ok := lookup(to, name, name(o))
		if !ok || !equal(o, nw) {
			d.emit(Change{Kind: drop, Schema: t.Schema, Table: t.Name, Name: name(o), Before: cp(o)})
		}
	}
	for _, n := range to {
		o, ok := byName[name(n)]
		if !ok || !equal(o, n) {
			d.emit(Change{Kind: add, Schema: t.Schema, Table: t.Name, Name: name(n), After: cp(n)})
		}
	}
}

func lookup[T any](xs []T, name func(T) string, n string) (T, bool) {
	for _, x := range xs {
		if name(x) == n {
			return x, true
		}
	}
	var zero T
	return zero, false
}

// ------------------------------------------------- views and other objects

func viewKey(v *schema.View) string { return v.Schema + "\x00" + v.Name }

func (d *differ) views(from, to *schema.Schema) {
	fromN := map[string]*schema.View{}
	for i := range d.fromNorm.Views {
		fromN[viewKey(&d.fromNorm.Views[i])] = &d.fromNorm.Views[i]
	}
	toN := map[string]*schema.View{}
	for i := range d.toNorm.Views {
		toN[viewKey(&d.toNorm.Views[i])] = &d.toNorm.Views[i]
	}
	fromO, toO := map[string]*schema.View{}, map[string]*schema.View{}
	for i := range from.Views {
		fromO[viewKey(&from.Views[i])] = &from.Views[i]
	}
	for i := range to.Views {
		toO[viewKey(&to.Views[i])] = &to.Views[i]
	}
	for _, v := range d.toNorm.Views {
		k := viewKey(&v)
		o, ok := fromN[k]
		switch {
		case !ok:
			d.emit(Change{Kind: CreateView, Schema: v.Schema, Name: v.Name, After: cp(*toO[k])})
		case o.Query != v.Query || o.Materialized != v.Materialized || !reflect.DeepEqual(o.Indexes, v.Indexes):
			d.emit(Change{Kind: DropView, Schema: v.Schema, Name: v.Name, Before: cp(*fromO[k])})
			d.emit(Change{Kind: CreateView, Schema: v.Schema, Name: v.Name, After: cp(*toO[k])})
		}
	}
	for _, v := range d.fromNorm.Views {
		k := viewKey(&v)
		if _, ok := toN[k]; !ok {
			d.emit(Change{Kind: DropView, Schema: v.Schema, Name: v.Name, Before: cp(*fromO[k])})
		}
	}
}

func funcKey(f *schema.Function) string { return f.Schema + "\x00" + f.Name + "\x00" + f.Args }

func (d *differ) functions(from, to *schema.Schema) {
	fromN := map[string]*schema.Function{}
	for i := range d.fromNorm.Functions {
		fromN[funcKey(&d.fromNorm.Functions[i])] = &d.fromNorm.Functions[i]
	}
	toN := map[string]*schema.Function{}
	for i := range d.toNorm.Functions {
		toN[funcKey(&d.toNorm.Functions[i])] = &d.toNorm.Functions[i]
	}
	fromO, toO := map[string]*schema.Function{}, map[string]*schema.Function{}
	for i := range from.Functions {
		fromO[funcKey(&from.Functions[i])] = &from.Functions[i]
	}
	for i := range to.Functions {
		toO[funcKey(&to.Functions[i])] = &to.Functions[i]
	}
	for _, f := range d.toNorm.Functions {
		k := funcKey(&f)
		o, ok := fromN[k]
		switch {
		case !ok:
			d.emit(Change{Kind: CreateFunction, Schema: f.Schema, Name: f.Name, After: cp(*toO[k])})
		case *o != f:
			d.emit(Change{Kind: ReplaceFunction, Schema: f.Schema, Name: f.Name, Before: cp(*fromO[k]), After: cp(*toO[k])})
		}
	}
	for _, f := range d.fromNorm.Functions {
		k := funcKey(&f)
		if _, ok := toN[k]; !ok {
			d.emit(Change{Kind: DropFunction, Schema: f.Schema, Name: f.Name, Before: cp(*fromO[k])})
		}
	}
}

func triggerKey(t *schema.Trigger) string { return t.Schema + "\x00" + t.Table + "\x00" + t.Name }

func (d *differ) triggers(from, to *schema.Schema) {
	fromN := map[string]*schema.Trigger{}
	for i := range d.fromNorm.Triggers {
		fromN[triggerKey(&d.fromNorm.Triggers[i])] = &d.fromNorm.Triggers[i]
	}
	toN := map[string]*schema.Trigger{}
	for i := range d.toNorm.Triggers {
		toN[triggerKey(&d.toNorm.Triggers[i])] = &d.toNorm.Triggers[i]
	}
	fromO, toO := map[string]*schema.Trigger{}, map[string]*schema.Trigger{}
	for i := range from.Triggers {
		fromO[triggerKey(&from.Triggers[i])] = &from.Triggers[i]
	}
	for i := range to.Triggers {
		toO[triggerKey(&to.Triggers[i])] = &to.Triggers[i]
	}
	for _, t := range d.toNorm.Triggers {
		k := triggerKey(&t)
		o, ok := fromN[k]
		switch {
		case !ok:
			d.emit(Change{Kind: CreateTrigger, Schema: t.Schema, Table: t.Table, Name: t.Name, After: cp(*toO[k])})
		case !reflect.DeepEqual(*o, t):
			d.emit(Change{Kind: DropTrigger, Schema: t.Schema, Table: t.Table, Name: t.Name, Before: cp(*fromO[k])})
			d.emit(Change{Kind: CreateTrigger, Schema: t.Schema, Table: t.Table, Name: t.Name, After: cp(*toO[k])})
		}
	}
	for _, t := range d.fromNorm.Triggers {
		k := triggerKey(&t)
		if _, ok := toN[k]; !ok {
			d.emit(Change{Kind: DropTrigger, Schema: t.Schema, Table: t.Table, Name: t.Name, Before: cp(*fromO[k])})
		}
	}
}

func (d *differ) policies(from, to *schema.Schema) {
	key := func(p *schema.Policy) string { return p.Schema + "\x00" + p.Table + "\x00" + p.Name }
	fromN := map[string]*schema.Policy{}
	for i := range d.fromNorm.Policies {
		fromN[key(&d.fromNorm.Policies[i])] = &d.fromNorm.Policies[i]
	}
	toN := map[string]*schema.Policy{}
	for i := range d.toNorm.Policies {
		toN[key(&d.toNorm.Policies[i])] = &d.toNorm.Policies[i]
	}
	fromO, toO := map[string]*schema.Policy{}, map[string]*schema.Policy{}
	for i := range from.Policies {
		fromO[key(&from.Policies[i])] = &from.Policies[i]
	}
	for i := range to.Policies {
		toO[key(&to.Policies[i])] = &to.Policies[i]
	}
	for _, p := range d.toNorm.Policies {
		k := key(&p)
		o, ok := fromN[k]
		switch {
		case !ok:
			d.emit(Change{Kind: CreatePolicy, Schema: p.Schema, Table: p.Table, Name: p.Name, After: cp(*toO[k])})
		case !reflect.DeepEqual(*o, p):
			d.emit(Change{Kind: DropPolicy, Schema: p.Schema, Table: p.Table, Name: p.Name, Before: cp(*fromO[k])})
			d.emit(Change{Kind: CreatePolicy, Schema: p.Schema, Table: p.Table, Name: p.Name, After: cp(*toO[k])})
		}
	}
	for _, p := range d.fromNorm.Policies {
		k := key(&p)
		if _, ok := toN[k]; !ok {
			d.emit(Change{Kind: DropPolicy, Schema: p.Schema, Table: p.Table, Name: p.Name, Before: cp(*fromO[k])})
		}
	}
}

// resolveColumnRenames returns old column -> new column for the explicit
// --rename-column flags that apply to the table pair (fn is the snapshot side,
// tn the desired side, both normalized). A flag key is "table.old" with either
// the old or the new table name, optionally schema-qualified.
func (d *differ) resolveColumnRenames(fn, tn *schema.Table) map[string]string {
	fromN, toN := colByName(fn), colByName(tn)
	out := map[string]string{}
	prefixes := []string{tn.Name, fn.Name}
	if tn.Schema != "" {
		prefixes = append(prefixes, tn.Schema+"."+tn.Name)
	}
	if fn.Schema != "" {
		prefixes = append(prefixes, fn.Schema+"."+fn.Name)
	}
	for _, tbl := range prefixes {
		for _, key := range sortedMapKeys(d.opts.RenameColumns) {
			oldCol, ok := strings.CutPrefix(key, tbl+".")
			if !ok {
				continue
			}
			newCol := d.opts.RenameColumns[key]
			_, hasOld := fromN[oldCol]
			_, hasNew := toN[newCol]
			_, newExisted := fromN[newCol]
			_, oldKept := toN[oldCol]
			if !hasOld || !hasNew || newExisted || oldKept {
				continue
			}
			d.usedRC[key] = true
			out[oldCol] = newCol
		}
	}
	return out
}

// remapFrom returns a copy of the snapshot-side table whose constraints name
// the columns and referenced tables under their new names, so that a rename is
// not also reported as a drop and re-add of every constraint that mentions the
// column. The input is not modified.
func (d *differ) remapFrom(fn *schema.Table, renameTo map[string]string) *schema.Table {
	out := *fn
	col := func(c string) string {
		if n, ok := renameTo[c]; ok {
			return n
		}
		return c
	}
	cols := func(in []string) []string {
		if in == nil {
			return nil
		}
		r := make([]string, len(in))
		for i, c := range in {
			r[i] = col(c)
		}
		return r
	}
	out.PrimaryKey = cols(fn.PrimaryKey)
	out.Uniques = slices.Clone(fn.Uniques)
	for i := range out.Uniques {
		out.Uniques[i].Columns = cols(out.Uniques[i].Columns)
	}
	out.Checks = slices.Clone(fn.Checks)
	for i := range out.Checks {
		out.Checks[i].Expr = rewriteIdents(out.Checks[i].Expr, renameTo)
	}
	out.Indexes = slices.Clone(fn.Indexes)
	for i := range out.Indexes {
		ix := &out.Indexes[i]
		ix.Where = rewriteIdents(ix.Where, renameTo)
		ix.Parts = slices.Clone(ix.Parts)
		for j := range ix.Parts {
			if ix.Parts[j].Column != "" {
				ix.Parts[j].Column = col(ix.Parts[j].Column)
			}
			ix.Parts[j].Expr = rewriteIdents(ix.Parts[j].Expr, renameTo)
		}
	}
	out.ForeignKeys = slices.Clone(fn.ForeignKeys)
	for i := range out.ForeignKeys {
		fk := &out.ForeignKeys[i]
		fk.Columns = cols(fk.Columns)
		refKey := fn.Schema + "\x00" + fk.RefTable
		refCols := d.colRen[refKey]
		fk.RefColumns = slices.Clone(fk.RefColumns)
		for j, c := range fk.RefColumns {
			if n, ok := refCols[c]; ok {
				fk.RefColumns[j] = n
			}
		}
		if nk, ok := d.tblRen[refKey]; ok {
			fk.RefTable = strings.TrimPrefix(nk, fn.Schema+"\x00")
		}
	}
	return &out
}

// rewriteIdents replaces every unquoted identifier of a normalized SQL
// expression that is a key of m. Quoted text is left alone.
func rewriteIdents(expr string, m map[string]string) string {
	if expr == "" || len(m) == 0 {
		return expr
	}
	var sb strings.Builder
	var quote rune
	rs := []rune(expr)
	isIdent := func(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
	for i := 0; i < len(rs); {
		r := rs[i]
		if quote != 0 {
			sb.WriteRune(r)
			if r == quote {
				quote = 0
			}
			i++
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			sb.WriteRune(r)
			i++
			continue
		}
		if isIdent(r) {
			j := i
			for j < len(rs) && isIdent(rs[j]) {
				j++
			}
			word := string(rs[i:j])
			// A word directly after a dot is a qualified name of something else.
			if n, ok := m[word]; ok && (i == 0 || rs[i-1] != '.') {
				word = n
			}
			sb.WriteString(word)
			i = j
			continue
		}
		sb.WriteRune(r)
		i++
	}
	return sb.String()
}

// stagedSnapshot returns desired with every still-staged column recorded as
// nullable, the way the database has it, so a later Diff with
// opts.EnforceNotNull can emit SET NOT NULL after the backfill. A column is
// staged when this diff stages it (add_column or alter_column whose After is
// nullable while the model says NOT NULL), or when from already has it
// nullable, the model has it NOT NULL without a default and EnforceNotNull is
// off: a diff that carries an earlier staged column along must not record it
// as NOT NULL. desired is never modified; it is returned as is when nothing
// is staged.
func stagedSnapshot(desired, from *schema.Schema, changes []Change, opts DiffOptions) *schema.Schema {
	type key struct{ schema, table, column string }
	staged := map[key]bool{}
	for _, c := range changes {
		if c.Kind != AddColumn && c.Kind != AlterColumn {
			continue
		}
		if col, ok := c.After.(*schema.Column); ok && col.Nullable {
			staged[key{c.Schema, c.Table, c.Name}] = true
		}
	}
	if !opts.EnforceNotNull && from != nil {
		for _, t := range from.Tables {
			for _, col := range t.Columns {
				if col.Nullable {
					staged[key{t.Schema, t.Name, col.Name}] = true
				}
			}
		}
	}
	if len(staged) == 0 {
		return desired
	}
	out := *desired
	out.Tables = slices.Clone(desired.Tables)
	changed := false
	for i := range out.Tables {
		t := &out.Tables[i]
		var cols []schema.Column
		for j, col := range t.Columns {
			if col.Nullable || !needsStaging(&col) || !staged[key{t.Schema, t.Name, col.Name}] {
				continue
			}
			if cols == nil {
				cols = slices.Clone(t.Columns)
			}
			cols[j].Nullable = true
			changed = true
		}
		if cols != nil {
			t.Columns = cols
		}
	}
	if !changed {
		return desired
	}
	return &out
}
