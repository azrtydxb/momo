// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// bodyTag is the dollar-quote tag every function body is wrapped in.
const bodyTag = "$momo$"

var pg = dialect.For(dialect.Postgres)

// Render turns an ordered plan into PostgreSQL statements. The plan must
// already be dependency-ordered by migrate.OrderErr or migrate.OrderWithViews,
// whose cycle errors the caller surfaces; Render never reorders. Every
// identifier is quoted, every literal comes through quoteLiteral, and the
// output depends only on the plan, never on map order.
func (Dialect) Render(plan []migrate.Change) ([]migrate.Statement, error) {
	var out []migrate.Statement
	for _, c := range plan {
		stmts, err := renderChange(c)
		if err != nil {
			return nil, fmt.Errorf("postgres render %s %s: %w", c.Kind, describe(c), err)
		}
		out = append(out, stmts...)
	}
	return out, nil
}

func describe(c migrate.Change) string {
	switch {
	case c.Table != "" && c.Table != c.Name:
		return c.Table + "." + c.Name
	default:
		return c.Name
	}
}

func renderChange(c migrate.Change) ([]migrate.Statement, error) {
	switch c.Kind {
	case migrate.CreateSchema:
		return one("CREATE SCHEMA " + ident(c.Name)), nil
	case migrate.CreateExtension:
		return renderCreateExtension(c)
	case migrate.CreateEnum:
		return renderCreateEnum(c)
	case migrate.AlterEnumAddValue:
		v, ok := c.After.(string)
		if !ok {
			return nil, fmt.Errorf("the After payload must be the new value, got %T", c.After)
		}
		lit, err := quoteLiteral(v)
		if err != nil {
			return nil, err
		}
		return []migrate.Statement{{SQL: "ALTER TYPE " + ident(c.Name) + " ADD VALUE " + lit, Hint: "enum-add"}}, nil
	case migrate.DropEnum:
		return nil, fmt.Errorf("enum %q cannot be dropped by a generated migration; removing an enum or its values requires a manual migration", c.Name)
	case migrate.CreateTable:
		return renderCreateTable(c)
	case migrate.DropTable:
		return one("DROP TABLE " + tableRef(c.Schema, c.Table)), nil
	case migrate.RenameTable:
		return one("ALTER TABLE " + tableRef(c.Schema, c.Table) + " RENAME TO " + ident(c.Name)), nil
	case migrate.AddColumn:
		return renderAddColumn(c)
	case migrate.DropColumn:
		return one("ALTER TABLE " + tableRef(c.Schema, c.Table) + " DROP COLUMN " + ident(c.Name)), nil
	case migrate.RenameColumn:
		return renderRenameColumn(c)
	case migrate.AlterColumn:
		return renderAlterColumn(c)
	case migrate.AddPrimaryKey:
		cols, err := get[[]string](c.After)
		if err != nil {
			return nil, err
		}
		return one("ALTER TABLE " + tableRef(c.Schema, c.Table) + " ADD CONSTRAINT " + ident(c.Name) + " PRIMARY KEY (" + identList(cols) + ")"), nil
	case migrate.DropPrimaryKey, migrate.DropUnique, migrate.DropCheck, migrate.DropFK:
		ifExists := ""
		if c.IfExists {
			ifExists = "IF EXISTS "
		}
		return one("ALTER TABLE " + tableRef(c.Schema, c.Table) + " DROP CONSTRAINT " + ifExists + ident(c.Name)), nil
	case migrate.AddUnique:
		u, err := get[schema.Unique](c.After)
		if err != nil {
			return nil, err
		}
		return renderAddUnique(c, u), nil
	case migrate.AddCheck:
		ck, err := get[schema.Check](c.After)
		if err != nil {
			return nil, err
		}
		return renderAddCheck(c, ck), nil
	case migrate.AddFK:
		fk, err := get[schema.ForeignKey](c.After)
		if err != nil {
			return nil, err
		}
		def, err := fkDef(fk)
		if err != nil {
			return nil, err
		}
		return one("ALTER TABLE " + tableRef(c.Schema, c.Table) + " ADD " + def), nil
	case migrate.CreateIndex:
		ix, err := get[schema.Index](c.After)
		if err != nil {
			return nil, err
		}
		if c.Partitioned {
			ix.Concurrently = false // not allowed on a partitioned table
		}
		s, err := createIndex(c.Schema, c.Table, ix)
		if err != nil {
			return nil, err
		}
		return []migrate.Statement{s}, nil
	case migrate.DropIndex:
		ix, err := get[schema.Index](c.Before)
		if err != nil {
			return nil, err
		}
		concurrently := ix.Concurrently && !c.Partitioned // not allowed on a partitioned table
		sql := "DROP INDEX "
		if concurrently {
			sql += "CONCURRENTLY "
		}
		if c.IfExists {
			sql += "IF EXISTS "
		}
		return []migrate.Statement{{SQL: sql + tableRef(c.Schema, c.Name), NoTx: concurrently}}, nil
	case migrate.SetRLS:
		return renderSetRLS(c)
	case migrate.CreatePolicy:
		return renderCreatePolicy(c)
	case migrate.DropPolicy:
		return one("DROP POLICY " + ident(c.Name) + " ON " + tableRef(c.Schema, c.Table)), nil
	case migrate.CreateView:
		return renderCreateView(c)
	case migrate.DropView:
		v, err := get[schema.View](c.Before)
		if err != nil {
			return nil, err
		}
		kw := "VIEW "
		if v.Materialized {
			kw = "MATERIALIZED VIEW "
		}
		return one("DROP " + kw + tableRef(c.Schema, c.Name)), nil
	case migrate.CreateFunction:
		f, err := get[schema.Function](c.After)
		if err != nil {
			return nil, err
		}
		s, err := createFunction(f, false)
		if err != nil {
			return nil, err
		}
		return one(s), nil
	case migrate.ReplaceFunction:
		return renderReplaceFunction(c)
	case migrate.DropFunction:
		f, err := get[schema.Function](c.Before)
		if err != nil {
			return nil, err
		}
		return one(dropFunction(f)), nil
	case migrate.CreateTrigger:
		return renderCreateTrigger(c)
	case migrate.DropTrigger:
		return one("DROP TRIGGER " + ident(c.Name) + " ON " + tableRef(c.Schema, c.Table)), nil
	}
	return nil, fmt.Errorf("unsupported change kind %q", c.Kind)
}

// ------------------------------------------------------------------ helpers

func one(sql string) []migrate.Statement { return []migrate.Statement{{SQL: sql}} }

func ident(s string) string { return pg.QuoteIdent(s) }

func identList(xs []string) string {
	q := make([]string, len(xs))
	for i, x := range xs {
		q[i] = ident(x)
	}
	return strings.Join(q, ", ")
}

// tableRef is "schema"."name", or just "name" when schema is empty, in which
// case the connection's search_path resolves it.
func tableRef(schemaName, name string) string {
	if schemaName == "" {
		return ident(name)
	}
	return ident(schemaName) + "." + ident(name)
}

// qualifiedName splits a name the inspector spelled relative to the current
// schema ("name" or "schema.name") and quotes each part.
func qualifiedName(s string) string {
	if ns, name, ok := strings.Cut(s, "."); ok {
		return ident(ns) + "." + ident(name)
	}
	return ident(s)
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
	return zero, fmt.Errorf("unexpected change payload %T, want %T", v, zero)
}

// quoteLiteral renders s as a SQL string literal. Single quotes are doubled.
// A string with a backslash uses the E” form with backslashes doubled, which
// reads the same whatever standard_conforming_strings is set to.
func quoteLiteral(s string) (string, error) {
	if strings.ContainsRune(s, 0) {
		return "", errors.New("a string literal cannot contain a NUL byte")
	}
	q := strings.ReplaceAll(s, "'", "''")
	if strings.Contains(q, `\`) {
		return "E'" + strings.ReplaceAll(q, `\`, `\\`) + "'", nil
	}
	return "'" + q + "'", nil
}

var fkActionSet = map[string]bool{"CASCADE": true, "SET NULL": true, "SET DEFAULT": true, "RESTRICT": true, "NO ACTION": true}

func fkAction(a string) (string, error) {
	n := strings.ToUpper(strings.Join(strings.Fields(a), " "))
	if n != "" && !fkActionSet[n] {
		return "", fmt.Errorf("unknown foreign key action %q", a)
	}
	return n, nil
}

// ------------------------------------------------------- extensions, enums

func renderCreateExtension(c migrate.Change) ([]migrate.Statement, error) {
	e, err := get[schema.Extension](c.After)
	if err != nil {
		return nil, err
	}
	sql := "CREATE EXTENSION IF NOT EXISTS " + ident(e.Name)
	if e.Version != "" {
		v, err := quoteLiteral(e.Version)
		if err != nil {
			return nil, err
		}
		sql += " VERSION " + v
	}
	return one(sql), nil
}

func renderCreateEnum(c migrate.Change) ([]migrate.Statement, error) {
	e, err := get[schema.Enum](c.After)
	if err != nil {
		return nil, err
	}
	vals := make([]string, len(e.Values))
	for i, v := range e.Values {
		if vals[i], err = quoteLiteral(v); err != nil {
			return nil, err
		}
	}
	return one("CREATE TYPE " + ident(e.Name) + " AS ENUM (" + strings.Join(vals, ", ") + ")"), nil
}

// ------------------------------------------------------------------ tables

// columnType is the type text of a column. An enum column's type is the enum
// name and is quoted like any identifier; every other type is the model's SQL
// type text.
func columnType(col schema.Column) string {
	if col.Enum != "" && col.Type == col.Enum {
		return ident(col.Type)
	}
	return col.Type
}

func columnDef(col schema.Column) string {
	var b strings.Builder
	b.WriteString(ident(col.Name))
	b.WriteByte(' ')
	b.WriteString(columnType(col))
	if col.Collate != "" {
		b.WriteString(" COLLATE " + ident(col.Collate))
	}
	switch col.Identity {
	case "always":
		b.WriteString(" GENERATED ALWAYS AS IDENTITY")
	case "by_default":
		b.WriteString(" GENERATED BY DEFAULT AS IDENTITY")
	}
	if col.Generated != "" {
		b.WriteString(" GENERATED ALWAYS AS (" + col.Generated + ") STORED")
	} else if col.Default != "" {
		b.WriteString(" DEFAULT " + col.Default)
	}
	if !col.Nullable {
		b.WriteString(" NOT NULL")
	}
	return b.String()
}

func uniqueDef(u schema.Unique) string {
	return "CONSTRAINT " + ident(u.Name) + " UNIQUE (" + identList(u.Columns) + ")"
}

// renderAddUnique adds a unique constraint to an existing table without
// holding a long lock: the unique index is built CONCURRENTLY, then adopted
// as the constraint with USING INDEX, which only takes a brief lock. Both
// statements run outside a transaction (Diff puts them in the trailing
// no-transaction migration). A new table's uniques stay inline in CREATE TABLE.
func renderAddUnique(c migrate.Change, u schema.Unique) []migrate.Statement {
	t := tableRef(c.Schema, c.Table)
	if c.Partitioned {
		// CONCURRENTLY is not allowed on a partitioned table.
		return one("ALTER TABLE " + t + " ADD " + uniqueDef(u))
	}
	return []migrate.Statement{
		{SQL: "CREATE UNIQUE INDEX CONCURRENTLY " + ident(u.Name) + " ON " + t + " (" + identList(u.Columns) + ")", NoTx: true},
		{SQL: "ALTER TABLE " + t + " ADD CONSTRAINT " + ident(u.Name) + " UNIQUE USING INDEX " + ident(u.Name), NoTx: true},
	}
}

// renderAddCheck adds a check constraint to an existing table as NOT VALID,
// which skips the full-table scan, then validates it in a separate statement
// that only takes SHARE UPDATE EXCLUSIVE. The validation is NoTx so Diff
// places it in the trailing migration: inside the transaction of the ADD it
// would run under the ADD's ACCESS EXCLUSIVE lock and gain nothing.
func renderAddCheck(c migrate.Change, ck schema.Check) []migrate.Statement {
	t := tableRef(c.Schema, c.Table)
	return []migrate.Statement{
		{SQL: "ALTER TABLE " + t + " ADD " + checkDef(ck) + " NOT VALID"},
		{SQL: "ALTER TABLE " + t + " VALIDATE CONSTRAINT " + ident(ck.Name), NoTx: true},
	}
}

func checkDef(c schema.Check) string {
	return "CONSTRAINT " + ident(c.Name) + " CHECK (" + c.Expr + ")"
}

func fkDef(fk schema.ForeignKey) (string, error) {
	del, err := fkAction(fk.OnDelete)
	if err != nil {
		return "", err
	}
	upd, err := fkAction(fk.OnUpdate)
	if err != nil {
		return "", err
	}
	s := "CONSTRAINT " + ident(fk.Name) + " FOREIGN KEY (" + identList(fk.Columns) + ") REFERENCES " +
		qualifiedName(fk.RefTable) + " (" + identList(fk.RefColumns) + ")"
	if del != "" {
		s += " ON DELETE " + del
	}
	if upd != "" {
		s += " ON UPDATE " + upd
	}
	return s, nil
}

func commentStmt(schemaName, table, column, comment string) (migrate.Statement, error) {
	lit := "NULL"
	if comment != "" {
		var err error
		if lit, err = quoteLiteral(comment); err != nil {
			return migrate.Statement{}, err
		}
	}
	return migrate.Statement{SQL: "COMMENT ON COLUMN " + tableRef(schemaName, table) + "." + ident(column) + " IS " + lit}, nil
}

func renderCreateTable(c migrate.Change) ([]migrate.Statement, error) {
	t, err := get[schema.Table](c.After)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("CREATE TABLE " + tableRef(t.Schema, t.Name))
	if t.PartitionOf != nil {
		// A partition takes its columns from the parent.
		b.WriteString(" PARTITION OF " + qualifiedName(t.PartitionOf.Parent) + " " + t.PartitionOf.Bound)
		return one(b.String()), nil
	}
	var parts []string
	for _, col := range t.Columns {
		parts = append(parts, columnDef(col))
	}
	if len(t.PrimaryKey) > 0 {
		parts = append(parts, "CONSTRAINT "+ident(t.Name+"_pkey")+" PRIMARY KEY ("+identList(t.PrimaryKey)+")")
	}
	for _, u := range t.Uniques {
		parts = append(parts, uniqueDef(u))
	}
	for _, ck := range t.Checks {
		parts = append(parts, checkDef(ck))
	}
	for _, fk := range t.ForeignKeys {
		def, err := fkDef(fk)
		if err != nil {
			return nil, err
		}
		parts = append(parts, def)
	}
	b.WriteString(" (" + strings.Join(parts, ", ") + ")")
	if t.Partition != nil {
		b.WriteString(" PARTITION BY " + t.Partition.Key)
	}
	out := one(b.String())
	for _, col := range t.Columns {
		if col.Comment != "" {
			s, err := commentStmt(t.Schema, t.Name, col.Name, col.Comment)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
	}
	return out, nil
}

func renderAddColumn(c migrate.Change) ([]migrate.Statement, error) {
	col, err := get[schema.Column](c.After)
	if err != nil {
		return nil, err
	}
	out := one("ALTER TABLE " + tableRef(c.Schema, c.Table) + " ADD COLUMN " + columnDef(col))
	if col.Comment != "" {
		s, err := commentStmt(c.Schema, c.Table, col.Name, col.Comment)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func renderRenameColumn(c migrate.Change) ([]migrate.Statement, error) {
	old, err := get[schema.Column](c.Before)
	if err != nil {
		return nil, err
	}
	return one("ALTER TABLE " + tableRef(c.Schema, c.Table) + " RENAME COLUMN " + ident(old.Name) + " TO " + ident(c.Name)), nil
}

func renderAlterColumn(c migrate.Change) ([]migrate.Statement, error) {
	before, err := get[schema.Column](c.Before)
	if err != nil {
		return nil, err
	}
	after, err := get[schema.Column](c.After)
	if err != nil {
		return nil, err
	}
	pre := "ALTER TABLE " + tableRef(c.Schema, c.Table) + " ALTER COLUMN " + ident(after.Name) + " "
	nt := func(x string) string { return schema.NormalizeType(dialect.Postgres, x) }
	ne := func(x string) string { return schema.NormalizeExpr(dialect.Postgres, x) }

	typeChanged := nt(before.Type) != nt(after.Type) || before.Enum != after.Enum
	collateChanged := before.Collate != after.Collate
	defaultChanged := ne(before.Default) != ne(after.Default)
	var out []migrate.Statement
	add := func(s string) { out = append(out, migrate.Statement{SQL: s}) }

	if ne(before.Generated) != ne(after.Generated) {
		return nil, fmt.Errorf("column %q: a generated expression cannot be altered in place; write a custom migration", after.Name)
	}
	if (typeChanged || collateChanged) && before.Default != "" {
		// The old default may not cast to the new type.
		add(pre + "DROP DEFAULT")
		defaultChanged = after.Default != ""
	}
	if typeChanged || collateChanged {
		s := pre + "TYPE " + columnType(after)
		if after.Collate != "" {
			s += " COLLATE " + ident(after.Collate)
		}
		cast := ident(after.Name) + "::" + columnType(after)
		if after.Enum != "" || before.Enum != "" {
			cast = ident(after.Name) + "::text::" + columnType(after)
		}
		add(s + " USING " + cast)
	}
	if defaultChanged {
		if after.Default == "" {
			if before.Default != "" && !typeChanged && !collateChanged {
				add(pre + "DROP DEFAULT")
			}
		} else {
			add(pre + "SET DEFAULT " + after.Default)
		}
	}
	if before.Identity != after.Identity {
		switch after.Identity {
		case "":
			add(pre + "DROP IDENTITY IF EXISTS")
		case "always", "by_default":
			mode := map[string]string{"always": "ALWAYS", "by_default": "BY DEFAULT"}[after.Identity]
			if before.Identity == "" {
				add(pre + "ADD GENERATED " + mode + " AS IDENTITY")
			} else {
				add(pre + "SET GENERATED " + mode)
			}
		default:
			return nil, fmt.Errorf("column %q: unknown identity %q", after.Name, after.Identity)
		}
	}
	if before.Nullable != after.Nullable {
		if after.Nullable {
			add(pre + "DROP NOT NULL")
		} else {
			add(pre + "SET NOT NULL")
		}
	}
	if before.Comment != after.Comment {
		s, err := commentStmt(c.Schema, c.Table, after.Name, after.Comment)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// ----------------------------------------------------------------- indexes

var safeValue = regexp.MustCompile(`^[A-Za-z0-9_.+-]+$`)

var nullsOrder = map[string]string{"first": "NULLS FIRST", "last": "NULLS LAST"}

func createIndex(schemaName, table string, ix schema.Index) (migrate.Statement, error) {
	var b strings.Builder
	b.WriteString("CREATE ")
	if ix.Unique {
		b.WriteString("UNIQUE ")
	}
	b.WriteString("INDEX ")
	if ix.Concurrently {
		b.WriteString("CONCURRENTLY ")
	}
	b.WriteString(ident(ix.Name) + " ON " + tableRef(schemaName, table))
	if ix.Method != "" {
		b.WriteString(" USING " + ident(ix.Method))
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
			return migrate.Statement{}, fmt.Errorf("index %q part %d has neither a column nor an expression", ix.Name, i)
		}
		if p.OpClass != "" {
			s += " " + qualifiedName(p.OpClass)
		}
		if p.Desc {
			s += " DESC"
		}
		if p.Nulls != "" {
			n, ok := nullsOrder[strings.ToLower(p.Nulls)]
			if !ok {
				return migrate.Statement{}, fmt.Errorf("index %q: unknown nulls ordering %q", ix.Name, p.Nulls)
			}
			s += " " + n
		}
		parts[i] = s
	}
	b.WriteString(" (" + strings.Join(parts, ", ") + ")")
	if len(ix.With) > 0 {
		kv := make([]string, len(ix.With))
		for i, p := range ix.With {
			v := p.Value
			if !safeValue.MatchString(v) {
				var err error
				if v, err = quoteLiteral(v); err != nil {
					return migrate.Statement{}, err
				}
			}
			kv[i] = ident(p.Key) + " = " + v
		}
		b.WriteString(" WITH (" + strings.Join(kv, ", ") + ")")
	}
	if ix.Where != "" {
		b.WriteString(" WHERE " + ix.Where)
	}
	return migrate.Statement{SQL: b.String(), NoTx: ix.Concurrently}, nil
}

// ------------------------------------------------- RLS, policies, views

func renderSetRLS(c migrate.Change) ([]migrate.Statement, error) {
	after, err := get[schema.RLS](c.After)
	if err != nil {
		return nil, err
	}
	var before schema.RLS
	if c.Before != nil {
		if before, err = get[schema.RLS](c.Before); err != nil {
			return nil, err
		}
	}
	pre := "ALTER TABLE " + tableRef(c.Schema, c.Table) + " "
	var out []migrate.Statement
	if before.Enabled != after.Enabled {
		if after.Enabled {
			out = append(out, migrate.Statement{SQL: pre + "ENABLE ROW LEVEL SECURITY"})
		} else {
			out = append(out, migrate.Statement{SQL: pre + "DISABLE ROW LEVEL SECURITY"})
		}
	}
	if before.Forced != after.Forced {
		if after.Forced {
			out = append(out, migrate.Statement{SQL: pre + "FORCE ROW LEVEL SECURITY"})
		} else {
			out = append(out, migrate.Statement{SQL: pre + "NO FORCE ROW LEVEL SECURITY"})
		}
	}
	return out, nil
}

var policyCommands = map[string]bool{"ALL": true, "SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true}

var roleKeywords = map[string]bool{"PUBLIC": true, "CURRENT_USER": true, "SESSION_USER": true, "CURRENT_ROLE": true}

func renderCreatePolicy(c migrate.Change) ([]migrate.Statement, error) {
	p, err := get[schema.Policy](c.After)
	if err != nil {
		return nil, err
	}
	cmd := strings.ToUpper(p.Command)
	if cmd == "" {
		cmd = "ALL"
	}
	if !policyCommands[cmd] {
		return nil, fmt.Errorf("policy %q: unknown command %q", p.Name, p.Command)
	}
	kind := "RESTRICTIVE"
	if p.Permissive {
		kind = "PERMISSIVE"
	}
	s := "CREATE POLICY " + ident(p.Name) + " ON " + tableRef(c.Schema, c.Table) + " AS " + kind + " FOR " + cmd
	if len(p.Roles) > 0 {
		roles := make([]string, len(p.Roles))
		for i, r := range p.Roles {
			if roleKeywords[strings.ToUpper(r)] {
				roles[i] = strings.ToUpper(r)
			} else {
				roles[i] = ident(r)
			}
		}
		s += " TO " + strings.Join(roles, ", ")
	}
	if p.Using != "" {
		s += " USING (" + p.Using + ")"
	}
	if p.Check != "" {
		s += " WITH CHECK (" + p.Check + ")"
	}
	return one(s), nil
}

func renderCreateView(c migrate.Change) ([]migrate.Statement, error) {
	v, err := get[schema.View](c.After)
	if err != nil {
		return nil, err
	}
	q := strings.TrimRight(strings.TrimSpace(v.Query), "; \t\r\n")
	kw := "VIEW "
	if v.Materialized {
		kw = "MATERIALIZED VIEW "
	}
	out := one("CREATE " + kw + tableRef(c.Schema, c.Name) + " AS " + q)
	if v.Materialized {
		for _, ix := range v.Indexes {
			s, err := createIndex(c.Schema, c.Name, ix)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
	}
	return out, nil
}

// --------------------------------------------------- functions, triggers

var triggerFunc = regexp.MustCompile(`^(?:[A-Za-z_][A-Za-z0-9_$]*|"(?:[^"]|"")+")(?:\.(?:[A-Za-z_][A-Za-z0-9_$]*|"(?:[^"]|"")+"))?\(\)$`)

var (
	volatilities = map[string]bool{"VOLATILE": true, "STABLE": true, "IMMUTABLE": true}
	timings      = map[string]bool{"BEFORE": true, "AFTER": true, "INSTEAD OF": true}
	events       = map[string]bool{"INSERT": true, "UPDATE": true, "DELETE": true, "TRUNCATE": true}
)

func createFunction(f schema.Function, replace bool) (string, error) {
	if strings.Contains(f.Body+"$", bodyTag) {
		return "", fmt.Errorf("function %q: the body contains the reserved dollar-quote tag %s", f.Name, bodyTag)
	}
	vol := strings.ToUpper(f.Volatility)
	if vol == "" {
		vol = "VOLATILE"
	}
	if !volatilities[vol] {
		return "", fmt.Errorf("function %q: unknown volatility %q", f.Name, f.Volatility)
	}
	if f.Language == "" {
		return "", fmt.Errorf("function %q has no language", f.Name)
	}
	head := "CREATE FUNCTION "
	if replace {
		head = "CREATE OR REPLACE FUNCTION "
	}
	attrs := vol
	if f.Strict {
		attrs += " STRICT"
	}
	if f.SecurityDefiner {
		attrs += " SECURITY DEFINER"
	}
	return head + tableRef(f.Schema, f.Name) + "(" + f.Args + ") RETURNS " + f.Returns +
		" LANGUAGE " + ident(f.Language) + " " + attrs + " AS " + bodyTag + f.Body + bodyTag, nil
}

func dropFunction(f schema.Function) string {
	return "DROP FUNCTION " + tableRef(f.Schema, f.Name) + "(" + f.Args + ")"
}

func renderReplaceFunction(c migrate.Change) ([]migrate.Statement, error) {
	before, err := get[schema.Function](c.Before)
	if err != nil {
		return nil, err
	}
	after, err := get[schema.Function](c.After)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(before.Args) != strings.TrimSpace(after.Args) ||
		!strings.EqualFold(strings.TrimSpace(before.Returns), strings.TrimSpace(after.Returns)) {
		// CREATE OR REPLACE cannot change the argument list or the return type.
		create, err := createFunction(after, false)
		if err != nil {
			return nil, err
		}
		return []migrate.Statement{{SQL: dropFunction(before)}, {SQL: create}}, nil
	}
	s, err := createFunction(after, true)
	if err != nil {
		return nil, err
	}
	return one(s), nil
}

func renderCreateTrigger(c migrate.Change) ([]migrate.Statement, error) {
	t, err := get[schema.Trigger](c.After)
	if err != nil {
		return nil, err
	}
	timing := strings.ToUpper(t.Timing)
	if !timings[timing] {
		return nil, fmt.Errorf("trigger %q: unknown timing %q", t.Name, t.Timing)
	}
	if len(t.Events) == 0 {
		return nil, fmt.Errorf("trigger %q has no events", t.Name)
	}
	evs := make([]string, len(t.Events))
	for i, e := range t.Events {
		evs[i] = strings.ToUpper(e)
		if !events[evs[i]] {
			return nil, fmt.Errorf("trigger %q: unknown event %q", t.Name, e)
		}
	}
	if t.Function == "" {
		return nil, fmt.Errorf("trigger %q has no function", t.Name)
	}
	// Function is the regprocedure text read by inspection, for example
	// "touch()". The trigger arguments replace the empty parentheses.
	if !triggerFunc.MatchString(t.Function) {
		return nil, fmt.Errorf("trigger %q: function %q must be a (qualified) identifier followed by ()", t.Name, t.Function)
	}
	fn := strings.TrimSuffix(t.Function, "()")
	args := make([]string, len(t.Args))
	for i, a := range t.Args {
		if args[i], err = quoteLiteral(a); err != nil {
			return nil, err
		}
	}
	each := "STATEMENT"
	if t.ForEachRow {
		each = "ROW"
	}
	s := "CREATE TRIGGER " + ident(t.Name) + " " + timing + " " + strings.Join(evs, " OR ") +
		" ON " + tableRef(c.Schema, c.Table) + " FOR EACH " + each
	if t.When != "" {
		s += " WHEN (" + t.When + ")"
	}
	return one(s + " EXECUTE FUNCTION " + fn + "(" + strings.Join(args, ", ") + ")"), nil
}
