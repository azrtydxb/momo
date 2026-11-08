// SPDX-License-Identifier: Apache-2.0

package mysql

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// tableOptions end every CREATE TABLE.
const tableOptions = " ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"

// Render turns an ordered plan into MySQL statements. The plan must already be
// dependency-ordered by migrate.OrderErr or migrate.OrderWithViews; Render
// never reorders. Every identifier is quoted, every literal comes through
// quoteLiteral, and the output depends only on the plan. The output is the
// same for MySQL and MariaDB; a construct only one of them accepts (a
// functional index part, a VECTOR column) is rejected by the server when the
// migration runs.
//
// The statements are not marked NoTx: every MySQL migration runs statement by
// statement already, because DDL commits implicitly.
func (Dialect) Render(plan []migrate.Change) ([]migrate.Statement, error) {
	var out []migrate.Statement
	for _, c := range plan {
		stmts, err := renderChange(c)
		if err != nil {
			return nil, fmt.Errorf("mysql render %s %s: %w", c.Kind, describe(c), err)
		}
		out = append(out, stmts...)
	}
	return out, nil
}

func describe(c migrate.Change) string {
	if c.Table != "" && c.Table != c.Name {
		return c.Table + "." + c.Name
	}
	return c.Name
}

func renderChange(c migrate.Change) ([]migrate.Statement, error) {
	t := tableRef(c.Schema, c.Table)
	switch c.Kind {
	case migrate.CreateTable:
		return renderCreateTable(c)
	case migrate.DropTable:
		return one("DROP TABLE " + tableRef(c.Schema, c.Table)), nil
	case migrate.RenameTable:
		return one("RENAME TABLE " + t + " TO " + tableRef(c.Schema, c.Name)), nil
	case migrate.AddColumn:
		col, err := get[schema.Column](c.After)
		if err != nil {
			return nil, err
		}
		def, err := columnDef(col)
		if err != nil {
			return nil, err
		}
		return one("ALTER TABLE " + t + " ADD COLUMN " + def), nil
	case migrate.DropColumn:
		return one("ALTER TABLE " + t + " DROP COLUMN " + ident(c.Name)), nil
	case migrate.RenameColumn:
		old, err := get[schema.Column](c.Before)
		if err != nil {
			return nil, err
		}
		return one("ALTER TABLE " + t + " RENAME COLUMN " + ident(old.Name) + " TO " + ident(c.Name)), nil
	case migrate.AlterColumn:
		return renderAlterColumn(c)
	case migrate.AddPrimaryKey:
		cols, err := get[[]string](c.After)
		if err != nil {
			return nil, err
		}
		return one("ALTER TABLE " + t + " ADD PRIMARY KEY (" + identList(cols) + ")"), nil
	case migrate.DropPrimaryKey:
		return one("ALTER TABLE " + t + " DROP PRIMARY KEY"), nil
	case migrate.AddUnique:
		u, err := get[schema.Unique](c.After)
		if err != nil {
			return nil, err
		}
		return one("ALTER TABLE " + t + " ADD " + uniqueDef(u)), nil
	case migrate.DropUnique:
		// A UNIQUE constraint is an index of the same name.
		return one("ALTER TABLE " + t + " DROP INDEX " + ident(c.Name)), nil
	case migrate.AddCheck:
		ck, err := get[schema.Check](c.After)
		if err != nil {
			return nil, err
		}
		return one("ALTER TABLE " + t + " ADD " + checkDef(ck)), nil
	case migrate.DropCheck:
		return one("ALTER TABLE " + t + " DROP CONSTRAINT " + ident(c.Name)), nil
	case migrate.AddFK:
		fk, err := get[schema.ForeignKey](c.After)
		if err != nil {
			return nil, err
		}
		def, err := fkDef(fk)
		if err != nil {
			return nil, err
		}
		return one("ALTER TABLE " + t + " ADD " + def), nil
	case migrate.DropFK:
		return renderDropFK(c)
	case migrate.CreateIndex:
		ix, err := get[schema.Index](c.After)
		if err != nil {
			return nil, err
		}
		s, err := createIndex(c.Schema, c.Table, ix)
		if err != nil {
			return nil, err
		}
		return []migrate.Statement{s}, nil
	case migrate.DropIndex:
		return one("DROP INDEX " + ident(c.Name) + " ON " + t), nil
	}
	return nil, fmt.Errorf("the change kind %q has no MySQL rendering", c.Kind)
}

// ------------------------------------------------------------------ helpers

func one(sql string) []migrate.Statement { return []migrate.Statement{{SQL: sql}} }

func ident(s string) string { return quoteIdent(s) }

func identList(xs []string) string {
	q := make([]string, len(xs))
	for i, x := range xs {
		q[i] = ident(x)
	}
	return strings.Join(q, ", ")
}

func tableRef(schemaName, name string) string {
	if schemaName == "" {
		return ident(name)
	}
	return ident(schemaName) + "." + ident(name)
}

// qualifiedName splits "name" or "database.name" and quotes each part.
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

// quoteLiteral renders s as a MySQL string literal: quotes are doubled and
// backslashes escaped, which reads the same unless NO_BACKSLASH_ESCAPES is on.
func quoteLiteral(s string) (string, error) {
	if strings.ContainsRune(s, 0) {
		return "", errors.New("a string literal cannot contain a NUL byte")
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	return "'" + strings.ReplaceAll(s, "'", "''") + "'", nil
}

var (
	fkActionSet = map[string]bool{"CASCADE": true, "SET NULL": true, "SET DEFAULT": true, "RESTRICT": true, "NO ACTION": true}
	collation   = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	// literalDefault matches the defaults MySQL takes without parentheses; any
	// other expression must be wrapped.
	literalDefault = regexp.MustCompile(`(?is)^(?:null|true|false|-?\d+(?:\.\d+)?(?:e[+-]?\d+)?|'(?:[^'\\]|''|\\.)*'|x'[0-9a-f]*'|b'[01]*'|` +
		`current_timestamp(?:\(\d*\))?|now\(\d*\)|localtimestamp(?:\(\d*\))?|localtime(?:\(\d*\))?)$`)
	prefixPart = regexp.MustCompile(`^(.+)\((\d+)\)$`)
)

func fkAction(a string) (string, error) {
	n := strings.ToUpper(strings.Join(strings.Fields(a), " "))
	if n != "" && !fkActionSet[n] {
		return "", fmt.Errorf("unknown foreign key action %q", a)
	}
	return n, nil
}

// defaultSQL renders a column default. A literal and CURRENT_TIMESTAMP are
// written as they are; any other expression needs the parentheses MySQL
// 8.0.13 and MariaDB 10.2.1 require for an expression default.
func defaultSQL(expr string) string {
	e := strings.TrimSpace(expr)
	if literalDefault.MatchString(e) || wrapped(e) {
		return e
	}
	return "(" + e + ")"
}

// wrapped reports whether the opening parenthesis of e closes at its end.
func wrapped(e string) bool {
	if len(e) < 2 || e[0] != '(' {
		return false
	}
	depth := 0
	var quote byte
	for i := 0; i < len(e); i++ {
		c := e[i]
		if quote != 0 {
			switch {
			case c == '\\' && i+1 < len(e):
				i++
			case c == quote:
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i == len(e)-1
			}
		}
	}
	return false
}

// ------------------------------------------------------------------ tables

func columnDef(col schema.Column) (string, error) {
	if col.Enum != "" {
		return "", fmt.Errorf("column %q: enum types are not supported on MySQL; declare the column as ENUM(...) text or a varchar with a check", col.Name)
	}
	var b strings.Builder
	b.WriteString(ident(col.Name) + " " + col.Type)
	if col.Collate != "" {
		if !collation.MatchString(col.Collate) {
			return "", fmt.Errorf("column %q: %q is not a collation name", col.Name, col.Collate)
		}
		b.WriteString(" COLLATE " + col.Collate)
	}
	if col.Generated != "" {
		if col.Default != "" || col.Identity != "" {
			return "", fmt.Errorf("column %q: a generated column cannot have a default or an identity", col.Name)
		}
		b.WriteString(" GENERATED ALWAYS AS (" + col.Generated + ") STORED")
	}
	switch {
	case !col.Nullable:
		b.WriteString(" NOT NULL")
	case col.Generated == "":
		b.WriteString(" NULL")
		// A nullable generated column is written without the keyword: that is
		// the one spelling both servers take.
	}
	switch col.Identity {
	case "":
	case "by_default":
		b.WriteString(" AUTO_INCREMENT")
	case "always":
		return "", fmt.Errorf("column %q: GENERATED ALWAYS identity does not exist on MySQL; use by_default (AUTO_INCREMENT)", col.Name)
	default:
		return "", fmt.Errorf("column %q: identity %q is not supported on MySQL; use by_default (AUTO_INCREMENT)", col.Name, col.Identity)
	}
	if col.Default != "" {
		b.WriteString(" DEFAULT " + defaultSQL(col.Default))
	}
	if col.Comment != "" {
		lit, err := quoteLiteral(col.Comment)
		if err != nil {
			return "", err
		}
		b.WriteString(" COMMENT " + lit)
	}
	return b.String(), nil
}

func uniqueDef(u schema.Unique) string {
	return "CONSTRAINT " + ident(u.Name) + " UNIQUE (" + identList(u.Columns) + ")"
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

func renderCreateTable(c migrate.Change) ([]migrate.Statement, error) {
	t, err := get[schema.Table](c.After)
	if err != nil {
		return nil, err
	}
	if t.Partition != nil || t.PartitionOf != nil {
		return nil, errors.New("partitioned tables are not supported on MySQL")
	}
	var parts []string
	for _, col := range t.Columns {
		def, err := columnDef(col)
		if err != nil {
			return nil, err
		}
		parts = append(parts, def)
	}
	if len(t.PrimaryKey) > 0 {
		parts = append(parts, "PRIMARY KEY ("+identList(t.PrimaryKey)+")")
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
	return one("CREATE TABLE " + tableRef(t.Schema, t.Name) + " (" + strings.Join(parts, ", ") + ")" + tableOptions), nil
}

// renderDropFK drops a foreign key and the index MySQL built for it. That
// index carries the constraint's name, is not part of the schema model and
// would stay behind, so the next diff would show it. It is dropped only when
// it exists, is not unique, and no other foreign key of the table starts with
// the column the index starts with (MySQL then built no index of its own for
// that key, and DROP INDEX would fail with error 1553 after the key was
// already gone). A prepared statement decides, because DROP INDEX has no IF
// EXISTS on MySQL. The index stays when another key needs it, and then shows
// up as an index named like the dropped key: declare it, or drop it once
// nothing needs it. A model cannot declare an index under the name of a
// foreign key (ValidateSchema), so a declared index is never dropped here.
// The prepared statement is deallocated by the last statement; when EXECUTE
// fails the run stops with a dirty row first, which leaves the statement
// prepared on a session that ends with the run. That is harmless: a retry
// prepares the same name again, which replaces it.
func renderDropFK(c migrate.Change) ([]migrate.Statement, error) {
	t := tableRef(c.Schema, c.Table)
	drop, err := quoteLiteral("ALTER TABLE " + t + " DROP INDEX " + ident(c.Name))
	if err != nil {
		return nil, err
	}
	table, err := quoteLiteral(c.Table)
	if err != nil {
		return nil, err
	}
	name, err := quoteLiteral(c.Name)
	if err != nil {
		return nil, err
	}
	db := "DATABASE()"
	if c.Schema != "" {
		if db, err = quoteLiteral(c.Schema); err != nil {
			return nil, err
		}
	}
	return []migrate.Statement{
		{SQL: "ALTER TABLE " + t + " DROP FOREIGN KEY " + ident(c.Name)},
		{SQL: "SET @momo_drop_fk_index = (SELECT IF(COUNT(*) > 0 AND NOT EXISTS (SELECT 1 FROM information_schema.KEY_COLUMN_USAGE k WHERE k.TABLE_SCHEMA = " + db + " AND k.TABLE_NAME = " + table + " AND k.REFERENCED_TABLE_NAME IS NOT NULL AND k.CONSTRAINT_NAME <> " + name + " AND k.ORDINAL_POSITION = 1 AND k.COLUMN_NAME IN (SELECT s2.COLUMN_NAME FROM information_schema.STATISTICS s2 WHERE s2.TABLE_SCHEMA = " + db + " AND s2.TABLE_NAME = " + table + " AND s2.INDEX_NAME = " + name + " AND s2.SEQ_IN_INDEX = 1)), " + drop + ", 'DO 0') FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = " + db + " AND TABLE_NAME = " + table + " AND INDEX_NAME = " + name + " AND NON_UNIQUE = 1)"},
		{SQL: "PREPARE momo_drop_fk_index FROM @momo_drop_fk_index"},
		{SQL: "EXECUTE momo_drop_fk_index"},
		{SQL: "DEALLOCATE PREPARE momo_drop_fk_index"},
	}, nil
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
	t := tableRef(c.Schema, c.Table)
	nt := func(x string) string { return schema.NormalizeType(dialect.MySQL, x) }
	ne := func(x string) string { return schema.NormalizeExpr(dialect.MySQL, x) }
	onlyDefault := nt(before.Type) == nt(after.Type) && before.Nullable == after.Nullable && before.Identity == after.Identity &&
		ne(before.Generated) == ne(after.Generated) && before.Collate == after.Collate && before.Comment == after.Comment &&
		before.Enum == after.Enum && after.Generated == ""
	if onlyDefault {
		// A default change touches no data: SET DEFAULT is a metadata change.
		if after.Default == "" {
			return one("ALTER TABLE " + t + " ALTER COLUMN " + ident(after.Name) + " DROP DEFAULT"), nil
		}
		return one("ALTER TABLE " + t + " ALTER COLUMN " + ident(after.Name) + " SET DEFAULT " + defaultSQL(after.Default)), nil
	}
	// MODIFY COLUMN restates the whole column from the schema model, which has
	// no ON UPDATE CURRENT_TIMESTAMP, character set or visibility: a change
	// to such a column loses them. Write those migrations by hand.
	def, err := columnDef(after)
	if err != nil {
		return nil, err
	}
	return one("ALTER TABLE " + t + " MODIFY COLUMN " + def), nil
}

// ----------------------------------------------------------------- indexes

func createIndex(schemaName, table string, ix schema.Index) (migrate.Statement, error) {
	if ix.Where != "" {
		return migrate.Statement{}, fmt.Errorf("index %q: partial indexes do not exist on MySQL", ix.Name)
	}
	if len(ix.With) > 0 {
		return migrate.Statement{}, fmt.Errorf("index %q: storage parameters do not exist on MySQL", ix.Name)
	}
	var b strings.Builder
	b.WriteString("CREATE ")
	switch strings.ToLower(ix.Method) {
	case "", "btree":
	case "hash":
		return migrate.Statement{}, fmt.Errorf("index %q: InnoDB builds a HASH index as BTREE, so it would differ from the declaration forever; leave the method out", ix.Name)
	case "fulltext":
		if ix.Unique {
			return migrate.Statement{}, fmt.Errorf("index %q: a FULLTEXT index cannot be unique", ix.Name)
		}
		b.WriteString("FULLTEXT ")
	case "spatial":
		if ix.Unique {
			return migrate.Statement{}, fmt.Errorf("index %q: a SPATIAL index cannot be unique", ix.Name)
		}
		b.WriteString("SPATIAL ")
	default:
		return migrate.Statement{}, fmt.Errorf("index %q: the method %q is not supported on MySQL", ix.Name, ix.Method)
	}
	if ix.Unique {
		b.WriteString("UNIQUE ")
	}
	b.WriteString("INDEX " + ident(ix.Name) + " ON " + tableRef(schemaName, table))
	parts := make([]string, len(ix.Parts))
	for i, p := range ix.Parts {
		var s string
		switch {
		case p.Expr != "":
			s = "(" + p.Expr + ")"
		case p.Column != "":
			if m := prefixPart.FindStringSubmatch(p.Column); m != nil {
				s = ident(m[1]) + "(" + m[2] + ")"
			} else {
				s = ident(p.Column)
			}
		default:
			return migrate.Statement{}, fmt.Errorf("index %q part %d has neither a column nor an expression", ix.Name, i)
		}
		if p.OpClass != "" {
			return migrate.Statement{}, fmt.Errorf("index %q: operator classes do not exist on MySQL", ix.Name)
		}
		if p.Nulls != "" {
			return migrate.Statement{}, fmt.Errorf("index %q: NULLS %s ordering does not exist on MySQL", ix.Name, strings.ToUpper(p.Nulls))
		}
		if p.Desc {
			s += " DESC"
		}
		parts[i] = s
	}
	b.WriteString(" (" + strings.Join(parts, ", ") + ")")
	return migrate.Statement{SQL: b.String()}, nil
}
