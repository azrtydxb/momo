// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// Every query below is a constant with bound parameters: no identifier or
// value is ever spliced into SQL text.
const (
	qCurrentSchema = `SELECT current_schema()`
	qNamespaces    = `SELECT nspname FROM pg_namespace WHERE nspname = ANY($1) ORDER BY 1`
	qExtensions    = `SELECT extname, extversion FROM pg_extension WHERE extname <> 'plpgsql' ORDER BY 1`
	qEnums         = `SELECT n.nspname, t.typname, e.enumlabel
FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace LEFT JOIN pg_enum e ON e.enumtypid = t.oid
WHERE t.typtype = 'e' AND n.nspname = ANY($1) ORDER BY 1,2,e.enumsortorder`
	// Partition children are included (they carry PartitionOf); the plan's
	// relispartition = false filter would hide them from the result.
	qTables = `SELECT c.oid, n.nspname, c.relname, c.relrowsecurity, c.relforcerowsecurity
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r','p') AND n.nspname = ANY($1) ORDER BY 2,3`
	qColumns = `SELECT a.attname, format_type(a.atttypid, a.atttypmod), NOT a.attnotnull,
  pg_get_expr(d.adbin, d.adrelid), a.attidentity::text, a.attgenerated::text, t.typtype::text, t.typname,
  tn.nspname, a.attnum,
  CASE WHEN a.attcollation <> t.typcollation THEN (SELECT collname::text FROM pg_collation WHERE oid = a.attcollation) END,
  col_description(a.attrelid, a.attnum)
FROM pg_attribute a
LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
JOIN pg_type t ON t.oid = a.atttypid
JOIN pg_namespace tn ON tn.oid = t.typnamespace
WHERE a.attrelid = $1 AND a.attnum > 0 AND NOT a.attisdropped ORDER BY a.attnum`
	qOwnedSeq = `SELECT EXISTS (SELECT 1 FROM pg_depend
WHERE classid = 'pg_class'::regclass AND objid = to_regclass($1)::oid
  AND refobjid = $2::oid AND refobjsubid = $3::int2 AND deptype = 'a')`
	// conparentid = 0 drops constraints a partition child merely inherits.
	qConstraints = `SELECT conname, contype::text,
  (SELECT array_agg(a.attname::text ORDER BY k.ord) FROM unnest(conkey) WITH ORDINALITY k(n, ord)
     JOIN pg_attribute a ON a.attrelid = conrelid AND a.attnum = k.n),
  (SELECT n.nspname::text FROM pg_class rc JOIN pg_namespace n ON n.oid = rc.relnamespace WHERE rc.oid = confrelid),
  (SELECT rc.relname::text FROM pg_class rc WHERE rc.oid = confrelid),
  (SELECT array_agg(a.attname::text ORDER BY k.ord) FROM unnest(confkey) WITH ORDINALITY k(n, ord)
     JOIN pg_attribute a ON a.attrelid = confrelid AND a.attnum = k.n),
  confupdtype::text, confdeltype::text, pg_get_constraintdef(oid)
FROM pg_constraint WHERE conrelid = $1 AND conparentid = 0 ORDER BY conname`
	qPartitioned = `SELECT pt.partstrat::text, pg_get_partkeydef(c.oid)
FROM pg_partitioned_table pt JOIN pg_class c ON c.oid = pt.partrelid WHERE c.oid = $1`
	qPartChild = `SELECT pg_get_expr(c.relpartbound, c.oid), p.relname, pn.nspname
FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid JOIN pg_class p ON p.oid = i.inhparent
JOIN pg_namespace pn ON pn.oid = p.relnamespace WHERE c.oid = $1`
)

var (
	serialDefault = regexp.MustCompile(`^nextval\('(.*)'::regclass\)$`)
	checkBody     = regexp.MustCompile(`(?s)^CHECK \((.*)\)(?: NO INHERIT)?(?: NOT VALID)?$`)
)

var fkActions = map[string]string{"a": "", "r": "RESTRICT", "c": "CASCADE", "n": "SET NULL", "d": "SET DEFAULT"}

var partStrategies = map[string]string{"r": "range", "l": "list", "h": "hash"}

// Inspect reads the objects in opts.Schemas (default: the connection's
// current schema) into a schema. A table in the current schema has an empty
// Table.Schema; Schema.Schemas lists only the other managed namespaces.
// opts.Exclude is a drift-time filter and is not applied here.
func (d Dialect) Inspect(ctx context.Context, c momo.PinnedConn, opts migrate.InspectOptions) (*schema.Schema, error) {
	s, _, err := d.InspectWithInvalid(ctx, c, opts)
	return s, err
}

// InspectWithInvalid is Inspect plus the qualified names (relative to the
// current schema: bare, or "schema.index" elsewhere) of the indexes PostgreSQL marks invalid,
// which a failed CREATE INDEX CONCURRENTLY leaves behind. The schema model
// has no slot for them and they stay in Table.Indexes, so a plain Inspect
// hides the failure. Drift detection (momo-04 Task 18) must call this and
// report each name as Changed with Detail "invalid index (failed CONCURRENTLY
// build)". Names follow table order, then index name.
func (Dialect) InspectWithInvalid(ctx context.Context, c momo.PinnedConn, opts migrate.InspectOptions) (*schema.Schema, []string, error) {
	var cur *string
	if err := queryOne(ctx, c, qCurrentSchema, nil, &cur); err != nil {
		return nil, nil, fmt.Errorf("postgres inspect: current schema: %w", err)
	}
	if cur == nil {
		return nil, nil, errors.New("postgres inspect: current_schema() is NULL: the search_path names no existing schema")
	}
	current := *cur
	managed := opts.Schemas
	if len(managed) == 0 {
		managed = []string{current}
	}
	in := &inspector{c: c, current: current, managed: managed}
	out := &schema.Schema{FormatVersion: schema.FormatVersion, Dialect: dialect.Postgres, Tables: []schema.Table{}}
	if err := in.namespaces(ctx, out); err != nil {
		return nil, nil, fmt.Errorf("postgres inspect: schemas: %w", err)
	}
	if err := in.extensions(ctx, out); err != nil {
		return nil, nil, fmt.Errorf("postgres inspect: extensions: %w", err)
	}
	if err := in.enums(ctx, out); err != nil {
		return nil, nil, fmt.Errorf("postgres inspect: enums: %w", err)
	}
	if err := in.tables(ctx, out); err != nil {
		return nil, nil, err
	}
	if err := (Dialect{}).inspectObjects(ctx, c, out, managed); err != nil {
		return nil, nil, fmt.Errorf("postgres inspect: %w", err)
	}
	out.Sort()
	return out, in.invalid, nil
}

type inspector struct {
	c       momo.PinnedConn
	current string
	managed []string
	// invalid names the indexes left invalid by a failed CONCURRENTLY build.
	// The schema model has no field for them; InspectWithInvalid returns them.
	invalid []string
}

// qualify spells an object name relative to the current schema.
func (in *inspector) qualify(ns, name string) string {
	if ns == in.current {
		return name
	}
	return ns + "." + name
}

func (in *inspector) namespaces(ctx context.Context, out *schema.Schema) error {
	return forEach(ctx, in.c, qNamespaces, []any{in.managed}, func(r momo.Rows) error {
		var n string
		if err := r.Scan(&n); err != nil {
			return err
		}
		if n != in.current {
			out.Schemas = append(out.Schemas, schema.SchemaNamespace{Name: n})
		}
		return nil
	})
}

func (in *inspector) extensions(ctx context.Context, out *schema.Schema) error {
	return forEach(ctx, in.c, qExtensions, nil, func(r momo.Rows) error {
		var e schema.Extension
		if err := r.Scan(&e.Name, &e.Version); err != nil {
			return err
		}
		out.Extensions = append(out.Extensions, e)
		return nil
	})
}

func (in *inspector) enums(ctx context.Context, out *schema.Schema) error {
	idx := map[string]int{}
	return forEach(ctx, in.c, qEnums, []any{in.managed}, func(r momo.Rows) error {
		var ns, name string
		var label *string
		if err := r.Scan(&ns, &name, &label); err != nil {
			return err
		}
		q := in.qualify(ns, name)
		i, ok := idx[q]
		if !ok {
			i = len(out.Enums)
			idx[q] = i
			out.Enums = append(out.Enums, schema.Enum{Name: q, Values: []string{}})
		}
		if label != nil {
			out.Enums[i].Values = append(out.Enums[i].Values, *label)
		}
		return nil
	})
}

type tableRow struct {
	oid         uint32
	ns, name    string
	rls, forced bool
}

func (in *inspector) tables(ctx context.Context, out *schema.Schema) error {
	var rows []tableRow
	err := forEach(ctx, in.c, qTables, []any{in.managed}, func(r momo.Rows) error {
		var t tableRow
		if err := r.Scan(&t.oid, &t.ns, &t.name, &t.rls, &t.forced); err != nil {
			return err
		}
		rows = append(rows, t)
		return nil
	})
	if err != nil {
		return fmt.Errorf("postgres inspect: tables: %w", err)
	}
	for _, tr := range rows {
		t := schema.Table{Name: tr.name}
		if tr.ns != in.current {
			t.Schema = tr.ns
		}
		if tr.rls || tr.forced {
			t.RLS = &schema.RLS{Enabled: tr.rls, Forced: tr.forced}
		}
		if err := in.columns(ctx, tr, &t); err != nil {
			return fmt.Errorf("postgres inspect: columns of %s.%s: %w", tr.ns, tr.name, err)
		}
		if err := in.constraints(ctx, tr, &t); err != nil {
			return fmt.Errorf("postgres inspect: constraints of %s.%s: %w", tr.ns, tr.name, err)
		}
		if err := in.partitions(ctx, tr, &t); err != nil {
			return fmt.Errorf("postgres inspect: partitions of %s.%s: %w", tr.ns, tr.name, err)
		}
		ixs, invalid, err := Dialect{}.inspectIndexes(ctx, in.c, tr.oid)
		if err != nil {
			return fmt.Errorf("postgres inspect: indexes of %s.%s: %w", tr.ns, tr.name, err)
		}
		t.Indexes = ixs
		for _, name := range invalid {
			in.invalid = append(in.invalid, in.qualify(tr.ns, name))
		}
		out.Tables = append(out.Tables, t)
	}
	return nil
}

type rawColumn struct {
	name, formatted, identity, gen, typtype, typname, typns string
	nullable                                                bool
	attnum                                                  int16
	def, coll, cmt                                          *string
}

func (in *inspector) columns(ctx context.Context, tr tableRow, t *schema.Table) error {
	var cols []rawColumn
	err := forEach(ctx, in.c, qColumns, []any{tr.oid}, func(r momo.Rows) error {
		var x rawColumn
		if err := r.Scan(&x.name, &x.formatted, &x.nullable, &x.def, &x.identity, &x.gen,
			&x.typtype, &x.typname, &x.typns, &x.attnum, &x.coll, &x.cmt); err != nil {
			return err
		}
		cols = append(cols, x)
		return nil
	})
	if err != nil {
		return err
	}
	for _, x := range cols {
		col, err := in.column(ctx, tr.oid, x)
		if err != nil {
			return err
		}
		t.Columns = append(t.Columns, col)
	}
	return nil
}

func (in *inspector) column(ctx context.Context, rel uint32, x rawColumn) (schema.Column, error) {
	col := schema.Column{
		Name:     x.name,
		Type:     schema.NormalizeType(dialect.Postgres, x.formatted),
		Nullable: x.nullable,
	}
	def := ""
	if x.def != nil {
		def = *x.def
	}
	switch x.identity {
	case "a":
		col.Identity = "always"
	case "d":
		col.Identity = "by_default"
	}
	switch x.gen {
	case "s":
		col.Generated, def = def, ""
	case "":
	default:
		return col, fmt.Errorf("column %s: generated column kind %q (virtual) is not supported by the schema model", x.name, x.gen)
	}
	if def != "" && col.Identity == "" {
		serial, err := in.ownedSerial(ctx, def, rel, x.attnum)
		if err != nil {
			return col, err
		}
		if serial {
			col.Identity, def = "serial", ""
		}
	}
	col.Default = def
	if x.typtype == "e" {
		col.Enum = in.qualify(x.typns, x.typname)
	}
	if x.coll != nil {
		col.Collate = *x.coll
	}
	if x.cmt != nil {
		col.Comment = *x.cmt
	}
	return col, nil
}

// ownedSerial reports whether def is nextval() of a sequence owned by the
// column, which is how serial columns are declared.
func (in *inspector) ownedSerial(ctx context.Context, def string, rel uint32, attnum int16) (bool, error) {
	m := serialDefault.FindStringSubmatch(def)
	if m == nil {
		return false, nil
	}
	seq := strings.ReplaceAll(m[1], "''", "'")
	var owned bool
	if err := queryOne(ctx, in.c, qOwnedSeq, []any{seq, rel, attnum}, &owned); err != nil {
		return false, err
	}
	return owned, nil
}

func (in *inspector) constraints(ctx context.Context, tr tableRow, t *schema.Table) error {
	return forEach(ctx, in.c, qConstraints, []any{tr.oid}, func(r momo.Rows) error {
		var (
			name, typ, upd, del, def string
			refNS, refName           *string
			cols, refCols            []string
		)
		if err := r.Scan(&name, &typ, &cols, &refNS, &refName, &refCols, &upd, &del, &def); err != nil {
			return err
		}
		switch typ {
		case "p":
			t.PrimaryKey = cols
		case "u":
			t.Uniques = append(t.Uniques, schema.Unique{Name: name, Columns: cols})
		case "c":
			expr := def
			if m := checkBody.FindStringSubmatch(def); m != nil {
				expr = m[1]
			}
			t.Checks = append(t.Checks, schema.Check{Name: name, Expr: expr})
		case "f":
			ref := ""
			if refNS != nil && refName != nil {
				ref = in.qualify(*refNS, *refName)
			}
			t.ForeignKeys = append(t.ForeignKeys, schema.ForeignKey{
				Name: name, Columns: cols, RefTable: ref, RefColumns: refCols,
				OnUpdate: fkActions[upd], OnDelete: fkActions[del],
			})
		}
		return nil
	})
}

func (in *inspector) partitions(ctx context.Context, tr tableRow, t *schema.Table) error {
	var strat, key string
	err := forEach(ctx, in.c, qPartitioned, []any{tr.oid}, func(r momo.Rows) error {
		return r.Scan(&strat, &key)
	})
	if err != nil {
		return err
	}
	if key != "" {
		t.Partition = &schema.Partition{Strategy: partStrategies[strat], Key: key}
	}
	return forEach(ctx, in.c, qPartChild, []any{tr.oid}, func(r momo.Rows) error {
		var bound, parent, ns string
		if err := r.Scan(&bound, &parent, &ns); err != nil {
			return err
		}
		t.PartitionOf = &schema.PartitionOf{Parent: in.qualify(ns, parent), Bound: bound}
		return nil
	})
}

// forEach runs a query and calls fn for each row, always closing the rows.
func forEach(ctx context.Context, c momo.PinnedConn, sql string, args []any, fn func(momo.Rows) error) (err error) {
	rows, err := c.Query(ctx, sql, args)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := rows.Close(); err == nil {
			err = cerr
		}
	}()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func queryOne(ctx context.Context, c momo.PinnedConn, sql string, args []any, dest ...any) error {
	found := false
	err := forEach(ctx, c, sql, args, func(r momo.Rows) error {
		found = true
		return r.Scan(dest...)
	})
	if err == nil && !found {
		return errors.New("query returned no row")
	}
	return err
}
