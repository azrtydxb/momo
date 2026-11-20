// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// The catalog queries of domains and composite types. Objects an extension owns
// (deptype 'e') are not Momo's to manage. A composite type is a pg_type whose
// pg_class entry has relkind 'c': the row type of a table is a composite type
// too, but its relation is a table.
const (
	qDomains = `SELECT t.oid, n.nspname, t.typname, format_type(t.typbasetype, t.typtypmod), t.typnotnull,
  pg_get_expr(t.typdefaultbin, 0)
FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
WHERE t.typtype = 'd' AND n.nspname = ANY($1)
  AND NOT EXISTS (SELECT 1 FROM pg_depend e WHERE e.classid = 'pg_type'::regclass AND e.objid = t.oid AND e.deptype = 'e')
ORDER BY 2,3`
	// $1 is the set of domain oids, passed as int8[]. Only CHECK constraints: a
	// domain NOT NULL is read from pg_type.typnotnull.
	qDomainChecks = `SELECT c.contypid, c.conname, pg_get_constraintdef(c.oid)
FROM pg_constraint c WHERE c.contypid = ANY($1::int8[]::oid[]) AND c.contype = 'c' ORDER BY 1,2`
	// One row per attribute, and one row with NULL attribute columns for a type
	// without any. The last two columns say whether the attribute type is a
	// composite type, or an array of one, which momo refuses.
	qComposites = `SELECT n.nspname, t.typname, a.attname, format_type(a.atttypid, a.atttypmod), at.typtype::text,
  (SELECT et.typtype::text FROM pg_type et WHERE et.oid = at.typelem AND at.typcategory = 'A')
FROM pg_type t
JOIN pg_namespace n ON n.oid = t.typnamespace
JOIN pg_class c ON c.oid = t.typrelid AND c.relkind = 'c'
LEFT JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
LEFT JOIN pg_type at ON at.oid = a.atttypid
WHERE t.typtype = 'c' AND n.nspname = ANY($1)
  AND NOT EXISTS (SELECT 1 FROM pg_depend e WHERE e.classid = 'pg_type'::regclass AND e.objid = t.oid AND e.deptype = 'e')
ORDER BY 1,2,a.attnum`
)

// unmodelled is the error for a domain or composite type momo cannot model:
// it names the object and says how to proceed, and wraps momo.ErrUnsupported.
func unmodelled(what, name, why string) error {
	return &momo.Error{Kind: momo.ErrUnsupported, Op: "migrate/postgres", Err: fmt.Errorf("%s %q is not modelled (%s): declare it outside momo and hide it from inspection (migrate.exclude) to proceed", what, name, why)}
}

// domains reads the domains of the managed schemas.
func (in *inspector) domains(ctx context.Context, out *schema.Schema) error {
	type rawDomain struct {
		oid uint32
		ns  string
		d   schema.Domain
	}
	var raws []rawDomain
	err := forEach(ctx, in.c, qDomains, []any{in.managed}, func(r momo.Rows) error {
		var x rawDomain
		var def *string
		if err := r.Scan(&x.oid, &x.ns, &x.d.Name, &x.d.Type, &x.d.NotNull, &def); err != nil {
			return err
		}
		if in.excluded(x.ns, x.d.Name) {
			return nil
		}
		x.d.Schema = in.optSchema(x.ns)
		x.d.Type = schema.NormalizeType(dialect.Postgres, x.d.Type)
		x.d.Default = deref(def)
		raws = append(raws, x)
		return nil
	})
	if err != nil {
		return err
	}
	if len(raws) == 0 {
		return nil
	}
	ids := make([]int64, len(raws))
	for i, x := range raws {
		ids[i] = int64(x.oid)
	}
	checks := map[uint32][]schema.DomainCheck{}
	err = forEach(ctx, in.c, qDomainChecks, []any{ids}, func(r momo.Rows) error {
		var oid uint32
		var name, def string
		if err := r.Scan(&oid, &name, &def); err != nil {
			return err
		}
		m := checkBody.FindStringSubmatch(def)
		if m == nil {
			return fmt.Errorf("constraint %q of a domain has the definition %q, which is not a CHECK (...)", name, def)
		}
		checks[oid] = append(checks[oid], schema.DomainCheck{Name: name, Expr: m[1]})
		return nil
	})
	if err != nil {
		return fmt.Errorf("checks: %w", err)
	}
	for _, x := range raws {
		x.d.Checks = checks[x.oid]
		if err := schema.ValidateDomain(x.d); err != nil {
			return unmodelled("domain", x.d.Ref(), err.Error())
		}
		out.Domains = append(out.Domains, x.d)
	}
	return nil
}

// composites reads the composite types of the managed schemas. A type with an
// attribute that is a composite type or an array of one is refused with an
// error that wraps momo.ErrUnsupported.
func (in *inspector) composites(ctx context.Context, out *schema.Schema) error {
	idx := map[string]int{}
	return forEach(ctx, in.c, qComposites, []any{in.managed}, func(r momo.Rows) error {
		var ns, name string
		var attr, typ, kind, elemKind *string
		if err := r.Scan(&ns, &name, &attr, &typ, &kind, &elemKind); err != nil {
			return err
		}
		if in.excluded(ns, name) {
			return nil
		}
		key := ns + "." + name
		i, ok := idx[key]
		if !ok {
			i = len(out.Composites)
			idx[key] = i
			out.Composites = append(out.Composites, schema.Composite{Schema: in.optSchema(ns), Name: name})
		}
		if attr == nil {
			return nil
		}
		c := &out.Composites[i]
		if deref(kind) == "c" || deref(elemKind) == "c" {
			return unmodelled("composite type", c.Ref(), fmt.Sprintf("its attribute %q is a composite type or an array of one; nested composite types are not supported yet", *attr))
		}
		c.Attributes = append(c.Attributes, schema.CompositeAttr{Name: *attr, Type: schema.NormalizeType(dialect.Postgres, deref(typ))})
		return nil
	})
}

// checkComposites validates what composites read, after the last row, with the
// same rules a declaration has.
func checkComposites(out *schema.Schema) error {
	for _, c := range out.Composites {
		if len(c.Attributes) == 0 {
			return unmodelled("composite type", c.Ref(), "it has no attributes")
		}
		if err := schema.ValidateComposite(c); err != nil {
			return unmodelled("composite type", c.Ref(), err.Error())
		}
	}
	return nil
}
