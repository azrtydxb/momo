// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/schema"
)

// Constant, bound catalog queries. Names come from pg_namespace and pg_class
// columns and are spelled relative to the current schema by inspector.qualify,
// never through regclass::text, whose output depends on the search_path.
// Objects owned by an extension (deptype 'e') are not Momo's to manage.
const (
	qViews = `SELECT c.oid, n.nspname, c.relname, c.relkind::text, pg_get_viewdef(c.oid, true)
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('v','m') AND n.nspname = ANY($1)
  AND NOT EXISTS (SELECT 1 FROM pg_depend e WHERE e.classid = 'pg_class'::regclass AND e.objid = c.oid AND e.deptype = 'e')
ORDER BY 2,3`
	qViewDeps = `SELECT DISTINCT rn.nspname::text, rc.relname::text
FROM pg_depend d
JOIN pg_rewrite r ON r.oid = d.objid
JOIN pg_class v ON v.oid = r.ev_class
JOIN pg_class rc ON rc.oid = d.refobjid
JOIN pg_namespace rn ON rn.oid = rc.relnamespace
WHERE v.oid = $1 AND rc.oid <> v.oid AND d.classid = 'pg_rewrite'::regclass AND d.refclassid = 'pg_class'::regclass
ORDER BY 1,2`
	qFunctions = `SELECT n.nspname, p.proname, pg_get_function_arguments(p.oid), pg_get_function_result(p.oid),
  l.lanname, p.provolatile::text, p.prosrc, p.proisstrict, p.prosecdef
FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace JOIN pg_language l ON l.oid = p.prolang
WHERE p.prokind = 'f' AND n.nspname = ANY($1)
  AND NOT EXISTS (SELECT 1 FROM pg_depend e WHERE e.classid = 'pg_proc'::regclass AND e.objid = p.oid AND e.deptype = 'e')
ORDER BY 1,2,3`
	// pg_get_expr cannot deparse a WHEN condition (it references OLD and NEW),
	// so the condition is cut out of pg_get_triggerdef.
	// The trigger function is spelled name(argtypes), schema-qualified only
	// when it lives outside the current schema (the shape regprocedure gives
	// for a function that is not on the search_path), with identifiers quoted
	// as quote_ident does. tgparentid <> 0 marks a trigger a partition child
	// merely inherited; like inherited constraints it is hidden.
	qTriggers = `SELECT n.nspname, c.relname, t.tgname, t.tgtype::int4, CASE WHEN t.tgqual IS NOT NULL THEN pg_get_triggerdef(t.oid, true) END, ARRAY(SELECT a.attname::text FROM unnest(t.tgattr) WITH ORDINALITY k(n, ord) JOIN pg_attribute a ON a.attrelid = t.tgrelid AND a.attnum = k.n ORDER BY k.ord),
  pn.nspname, quote_ident(pn.nspname), quote_ident(p.proname), replace(oidvectortypes(p.proargtypes), ', ', ',')
FROM pg_trigger t
JOIN pg_class c ON c.oid = t.tgrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_proc p ON p.oid = t.tgfoid
JOIN pg_namespace pn ON pn.oid = p.pronamespace
WHERE NOT t.tgisinternal AND t.tgparentid = 0 AND n.nspname = ANY($1)
ORDER BY 1,2,3`
	qPolicies = `SELECT n.nspname, c.relname, p.polname, p.polcmd::text, p.polpermissive,
  ARRAY(SELECT CASE WHEN r.oid = 0 THEN 'PUBLIC' ELSE (SELECT rolname::text FROM pg_roles WHERE oid = r.oid) END
        FROM unnest(p.polroles) AS r(oid) ORDER BY 1),
  pg_get_expr(p.polqual, p.polrelid), pg_get_expr(p.polwithcheck, p.polrelid)
FROM pg_policy p JOIN pg_class c ON c.oid = p.polrelid JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = ANY($1)
ORDER BY 1,2,3`
)

var (
	triggerWhen = regexp.MustCompile(`(?s) WHEN \((.*)\) EXECUTE (?:FUNCTION|PROCEDURE) `)
	volatility  = map[string]string{"i": "IMMUTABLE", "s": "STABLE", "v": "VOLATILE"}
	policyCmds  = map[string]string{"r": "SELECT", "a": "INSERT", "w": "UPDATE", "d": "DELETE", "*": "ALL"}
)

// pg_trigger.tgtype bits.
const (
	trigRow      = 1
	trigBefore   = 2
	trigInsert   = 4
	trigDelete   = 8
	trigUpdate   = 16
	trigTruncate = 32
	trigInstead  = 64
)

// inspectObjects fills Views, Functions, Triggers and Policies of s from the
// given schemas. Table-level RLS flags and partition information are read
// with the tables themselves (inspect.go); partition children are tables
// carrying PartitionOf.
func (Dialect) inspectObjects(ctx context.Context, c momo.PinnedConn, s *schema.Schema, schemas, exclude []string) error {
	var cur *string
	if err := queryOne(ctx, c, qCurrentSchema, nil, &cur); err != nil {
		return fmt.Errorf("current schema: %w", err)
	}
	if cur == nil {
		return fmt.Errorf("current_schema() is NULL: the search_path names no existing schema")
	}
	in := &inspector{c: c, current: *cur, managed: schemas, exclude: exclude}
	if err := in.views(ctx, s); err != nil {
		return fmt.Errorf("views: %w", err)
	}
	if err := in.functions(ctx, s); err != nil {
		return fmt.Errorf("functions: %w", err)
	}
	if err := in.triggers(ctx, s); err != nil {
		return fmt.Errorf("triggers: %w", err)
	}
	if err := in.policies(ctx, s); err != nil {
		return fmt.Errorf("policies: %w", err)
	}
	return nil
}

// excluded reports whether a Config.Exclude glob matches name or schema.name.
// An excluded object is skipped before it is read, so one the model cannot
// represent does not fail the whole inspection.
func (in *inspector) excluded(ns, name string) bool {
	ns = in.optSchema(ns)
	for _, g := range in.exclude {
		if ok, _ := path.Match(g, name); ok {
			return true
		}
		if ns == "" {
			continue
		}
		if ok, _ := path.Match(g, ns+"."+name); ok {
			return true
		}
	}
	return false
}

// outsideOnly reports whether Config.OnlyTables is set and no glob of it
// matches the table name or schema.name.
func (in *inspector) outsideOnly(ns, name string) bool {
	if len(in.only) == 0 {
		return false
	}
	ns = in.optSchema(ns)
	for _, g := range in.only {
		if ok, _ := path.Match(g, name); ok {
			return false
		}
		if ns == "" {
			continue
		}
		if ok, _ := path.Match(g, ns+"."+name); ok {
			return false
		}
	}
	return true
}

// optSchema is the Schema field value for a namespace: empty for the current one.
func (in *inspector) optSchema(ns string) string {
	if ns == in.current {
		return ""
	}
	return ns
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (in *inspector) views(ctx context.Context, s *schema.Schema) error {
	type rawView struct {
		oid          uint32
		ns, name     string
		kind         string
		query        *string
		materialized bool
	}
	var raws []rawView
	err := forEach(ctx, in.c, qViews, []any{in.managed}, func(r momo.Rows) error {
		var v rawView
		if err := r.Scan(&v.oid, &v.ns, &v.name, &v.kind, &v.query); err != nil {
			return err
		}
		if in.excluded(v.ns, v.name) {
			return nil
		}
		v.materialized = v.kind == "m"
		raws = append(raws, v)
		return nil
	})
	if err != nil {
		return err
	}
	for _, x := range raws {
		v := schema.View{
			Schema:       in.optSchema(x.ns),
			Name:         x.name,
			Materialized: x.materialized,
			Query:        strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(deref(x.query)), ";")),
		}
		err := forEach(ctx, in.c, qViewDeps, []any{x.oid}, func(r momo.Rows) error {
			var ns, name string
			if err := r.Scan(&ns, &name); err != nil {
				return err
			}
			v.Depends = append(v.Depends, in.qualify(ns, name))
			return nil
		})
		if err != nil {
			return fmt.Errorf("dependencies of %s.%s: %w", x.ns, x.name, err)
		}
		if x.materialized {
			ixs, _, err := Dialect{}.inspectIndexes(ctx, in.c, x.oid)
			if err != nil {
				return fmt.Errorf("indexes of %s.%s: %w", x.ns, x.name, err)
			}
			v.Indexes = ixs
		}
		s.Views = append(s.Views, v)
	}
	return nil
}

func (in *inspector) functions(ctx context.Context, s *schema.Schema) error {
	return forEach(ctx, in.c, qFunctions, []any{in.managed}, func(r momo.Rows) error {
		var ns, vol string
		var f schema.Function
		var body *string
		if err := r.Scan(&ns, &f.Name, &f.Args, &f.Returns, &f.Language, &vol, &body, &f.Strict, &f.SecurityDefiner); err != nil {
			return err
		}
		if in.excluded(ns, f.Name) {
			return nil
		}
		f.Schema = in.optSchema(ns)
		f.Volatility = volatility[vol]
		f.Body = deref(body)
		s.Functions = append(s.Functions, f)
		return nil
	})
}

func (in *inspector) triggers(ctx context.Context, s *schema.Schema) error {
	return forEach(ctx, in.c, qTriggers, []any{in.managed}, func(r momo.Rows) error {
		var (
			ns, table, fnNS, fnNSQ, fnName, fnArgs string
			t                                      schema.Trigger
			tgtype                                 int32
			when                                   *string
			attrs                                  []string
		)
		if err := r.Scan(&ns, &table, &t.Name, &tgtype, &when, &attrs, &fnNS, &fnNSQ, &fnName, &fnArgs); err != nil {
			return err
		}
		if in.excluded(ns, table) || in.excluded(ns, t.Name) || in.outsideOnly(ns, table) {
			return nil
		}
		if len(attrs) > 0 {
			t.UpdateOf = attrs
		}
		t.Schema = in.optSchema(ns)
		t.Table = table
		if m := triggerWhen.FindStringSubmatch(deref(when)); m != nil {
			t.When = m[1]
		}
		t.ForEachRow = tgtype&trigRow != 0
		switch {
		case tgtype&trigInstead != 0:
			t.Timing = "INSTEAD OF"
		case tgtype&trigBefore != 0:
			t.Timing = "BEFORE"
		default:
			t.Timing = "AFTER"
		}
		for _, e := range []struct {
			bit  int32
			name string
		}{{trigInsert, "INSERT"}, {trigDelete, "DELETE"}, {trigUpdate, "UPDATE"}, {trigTruncate, "TRUNCATE"}} {
			if tgtype&e.bit != 0 {
				t.Events = append(t.Events, e.name)
			}
		}
		fn := fnName + "(" + fnArgs + ")"
		if fnNS != in.current {
			fn = fnNSQ + "." + fn
		}
		t.Function = fn
		s.Triggers = append(s.Triggers, t)
		return nil
	})
}

func (in *inspector) policies(ctx context.Context, s *schema.Schema) error {
	return forEach(ctx, in.c, qPolicies, []any{in.managed}, func(r momo.Rows) error {
		var ns, cmd string
		var p schema.Policy
		var using, check *string
		if err := r.Scan(&ns, &p.Table, &p.Name, &cmd, &p.Permissive, &p.Roles, &using, &check); err != nil {
			return err
		}
		if in.outsideOnly(ns, p.Table) {
			return nil
		}
		p.Schema = in.optSchema(ns)
		p.Command = policyCmds[cmd]
		p.Using = deref(using)
		p.Check = deref(check)
		// PUBLIC alone is the model's empty list; PUBLIC mixed with named
		// roles is kept so that no grant is lost.
		if len(p.Roles) == 1 && p.Roles[0] == "PUBLIC" {
			p.Roles = nil
		}
		s.Policies = append(s.Policies, p)
		return nil
	})
}
