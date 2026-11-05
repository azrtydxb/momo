// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

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
// left out and a hint describes the remaining steps.
//
// Drops are never paired with adds into renames: only opts.RenameTables and
// opts.RenameColumns produce renames. A hint names likely rename candidates.
//
// One deliberate exception to the staging rule: an added NOT NULL column that
// is a rename candidate (same table and type as a dropped column) is not
// reported as ErrStagedRequired, because the hint already tells the user to
// pass the rename flag. Applying the result without the flag would fail on a
// populated table, which is why the hint is always emitted.
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

	// Resolve explicit table renames: from key -> to key.
	renamed := map[string]string{}
	renamedTo := map[string]bool{}
	for _, oldName := range sortedMapKeys(d.opts.RenameTables) {
		newName := d.opts.RenameTables[oldName]
		var fk, tk string
		for k, t := range fromN {
			if t.Name == oldName {
				fk = k
			}
		}
		for k, t := range toN {
			if t.Name == newName {
				tk = k
			}
		}
		_, oldKept := toN[fk]
		_, newExisted := fromN[tk]
		if fk == "" || tk == "" || oldKept || newExisted {
			continue // reported as an unmatched rename flag
		}
		d.usedRT[oldName] = true
		renamed[fk] = tk
		renamedTo[tk] = true
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
	for _, t := range d.toNorm.Tables {
		k := tableKey(&t)
		switch {
		case renamedTo[k]:
			var fk string
			for a, b := range renamed {
				if b == k {
					fk = a
				}
			}
			d.diffTable(fromN[fk], toN[k], fromO[fk], toO[k])
		case fromN[k] != nil:
			d.diffTable(fromN[k], toN[k], fromO[k], toO[k])
		default:
			d.createTable(toO[k])
		}
	}
	for _, t := range dropped {
		o := fromO[tableKey(t)]
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

func (d *differ) diffTable(fn, tn, fo, to *schema.Table) {
	if !reflect.DeepEqual(fn.Partition, tn.Partition) || !reflect.DeepEqual(fn.PartitionOf, tn.PartitionOf) {
		d.errs = append(d.errs, fmt.Errorf("migrate: table %s changes its partitioning; this cannot be altered in place, write a custom migration", tn.Name))
	}
	if fn.WithoutRowID != tn.WithoutRowID {
		d.errs = append(d.errs, fmt.Errorf("migrate: table %s changes WITHOUT ROWID; write a custom migration", tn.Name))
	}
	d.columns(fn, tn, fo, to)
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

func needsStaging(c *schema.Column) bool {
	return !c.Nullable && c.Default == "" && c.Identity == "" && c.Generated == ""
}

func (d *differ) columns(fn, tn, fo, to *schema.Table) {
	fromN, toN := colByName(fn), colByName(tn)
	fromO, toO := colByName(fo), colByName(to)

	// Explicit renames: either the old or the new table name may prefix the key.
	renameTo := map[string]string{} // old column -> new column
	for _, tbl := range []string{to.Name, fo.Name} {
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
			renameTo[oldCol] = newCol
		}
	}
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
		if needsStaging(&c) && !candidate[c.Name] {
			if !d.opts.Stage {
				d.staged = append(d.staged, fmt.Sprintf("%s.%s", to.Name, c.Name))
			} else {
				staged := *col
				staged.Nullable = true
				d.emit(Change{Kind: AddColumn, Schema: to.Schema, Table: to.Name, Name: c.Name, After: &staged})
				d.hint(fmt.Sprintf("staged: %s.%s was added as nullable; backfill it, then run ALTER TABLE %s ALTER COLUMN %s SET NOT NULL (re-run migrate diff after the backfill)", to.Name, c.Name, to.Name, c.Name))
				continue
			}
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
	if a.Nullable && !b.Nullable && needsStaging(b) {
		if !d.opts.Stage {
			d.staged = append(d.staged, fmt.Sprintf("%s.%s", t.Name, b.Name))
		} else {
			after.Nullable = true
			nb.Nullable = true
			d.hint(fmt.Sprintf("staged: %s.%s stays nullable; backfill it, then run ALTER TABLE %s ALTER COLUMN %s SET NOT NULL (re-run migrate diff after the backfill)", t.Name, b.Name, t.Name, b.Name))
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
