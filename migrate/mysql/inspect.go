// SPDX-License-Identifier: Apache-2.0

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// Every query is a constant that binds the schema name: no identifier or value
// is spliced into SQL text. The history table is left out.
const (
	qTables = `SELECT TABLE_NAME FROM information_schema.TABLES
WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE' AND TABLE_NAME <> 'momo_migrations' ORDER BY TABLE_NAME`
	qColumns = `SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT, EXTRA, GENERATION_EXPRESSION, COLUMN_COMMENT
FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? ORDER BY TABLE_NAME, ORDINAL_POSITION`
	// MySQL 8.0.13 and later report the expression of a functional key part
	// in EXPRESSION; MariaDB has no such column.
	qIndexesMySQL = `SELECT TABLE_NAME, INDEX_NAME, NON_UNIQUE, COLUMN_NAME, SUB_PART, COLLATION, INDEX_TYPE, EXPRESSION
FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = ? ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`
	qIndexesMariaDB = `SELECT TABLE_NAME, INDEX_NAME, NON_UNIQUE, COLUMN_NAME, SUB_PART, COLLATION, INDEX_TYPE, NULL
FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = ? ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`
	qForeignKeys = `SELECT k.TABLE_NAME, k.CONSTRAINT_NAME, k.COLUMN_NAME, k.REFERENCED_TABLE_NAME, k.REFERENCED_COLUMN_NAME, r.UPDATE_RULE, r.DELETE_RULE
FROM information_schema.KEY_COLUMN_USAGE k
JOIN information_schema.REFERENTIAL_CONSTRAINTS r
  ON r.CONSTRAINT_SCHEMA = k.CONSTRAINT_SCHEMA AND r.CONSTRAINT_NAME = k.CONSTRAINT_NAME AND r.TABLE_NAME = k.TABLE_NAME
WHERE k.TABLE_SCHEMA = ? AND k.REFERENCED_TABLE_NAME IS NOT NULL
ORDER BY k.TABLE_NAME, k.CONSTRAINT_NAME, k.ORDINAL_POSITION`
	// MySQL's CHECK_CONSTRAINTS has no table column and names its checks
	// uniquely per schema, so the table comes from TABLE_CONSTRAINTS. MariaDB
	// names checks per table, so it reports the table itself.
	qChecksMySQL = `SELECT t.TABLE_NAME, c.CONSTRAINT_NAME, c.CHECK_CLAUSE
FROM information_schema.CHECK_CONSTRAINTS c
JOIN information_schema.TABLE_CONSTRAINTS t
  ON t.CONSTRAINT_SCHEMA = c.CONSTRAINT_SCHEMA AND t.CONSTRAINT_NAME = c.CONSTRAINT_NAME AND t.CONSTRAINT_TYPE = 'CHECK'
WHERE c.CONSTRAINT_SCHEMA = ? ORDER BY t.TABLE_NAME, c.CONSTRAINT_NAME`
	qChecksMariaDB = `SELECT TABLE_NAME, CONSTRAINT_NAME, CHECK_CLAUSE
FROM information_schema.CHECK_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = ? ORDER BY TABLE_NAME, CONSTRAINT_NAME`
)

// Inspect reads the tables of the connection's current database into a schema.
// Table.Schema stays empty: a MySQL "schema" is a database, and the engine
// manages the one the connection works in. opts.Schemas, when set, must name
// that database. opts.Exclude is a drift-time filter and is not applied here.
//
// What the server cannot tell apart is reported in one form: a UNIQUE
// constraint and a unique index are the same object, reported as a Unique when
// every key part is a plain ascending column and as a unique Index otherwise;
// the index MySQL creates for a foreign key (named after the constraint) is
// left out; NO ACTION foreign key actions are reported empty, and on MariaDB so
// is RESTRICT, which MariaDB reports for a clause that was never written.
// MariaDB stores JSON as LONGTEXT with a json_valid check: that pair is
// reported as type json. A prefix index part is reported as the column name
// followed by its length in parentheses ("name(10)").
func (Dialect) Inspect(ctx context.Context, c momo.PinnedConn, opts migrate.InspectOptions) (*schema.Schema, error) {
	db, err := currentDatabase(ctx, c)
	if err != nil {
		return nil, err
	}
	if db == "" {
		return nil, errors.New("mysql inspect: the connection has no default database; name one in the DSN")
	}
	if len(opts.Schemas) > 1 || len(opts.Schemas) == 1 && opts.Schemas[0] != db {
		return nil, fmt.Errorf("mysql inspect: only the connection's database %q can be inspected, not %v: a MySQL schema is a database, and the engine works in one", db, opts.Schemas)
	}
	fl, err := serverFlavor(ctx, c)
	if err != nil {
		return nil, err
	}
	in := &inspector{c: c, db: db, flavor: fl, byName: map[string]int{}}
	out := &schema.Schema{FormatVersion: schema.FormatVersion, Dialect: dialect.MySQL, Tables: []schema.Table{}}
	if err := in.tables(ctx, out); err != nil {
		return nil, fmt.Errorf("mysql inspect: tables: %w", err)
	}
	if err := in.columns(ctx, out); err != nil {
		return nil, fmt.Errorf("mysql inspect: columns: %w", err)
	}
	pk, idx, err := in.indexes(ctx)
	if err != nil {
		return nil, fmt.Errorf("mysql inspect: indexes: %w", err)
	}
	if err := in.foreignKeys(ctx, out); err != nil {
		return nil, fmt.Errorf("mysql inspect: foreign keys: %w", err)
	}
	if err := in.checks(ctx, out); err != nil {
		return nil, fmt.Errorf("mysql inspect: checks: %w", err)
	}
	in.applyIndexes(out, pk, idx)
	in.jsonColumns(out)
	out.Sort()
	return out, nil
}

type inspector struct {
	c      momo.PinnedConn
	db     string
	flavor dialect.Flavor
	byName map[string]int // table name -> index into Schema.Tables
}

func (in *inspector) table(s *schema.Schema, name string) *schema.Table {
	if i, ok := in.byName[name]; ok {
		return &s.Tables[i]
	}
	return nil
}

func (in *inspector) tables(ctx context.Context, out *schema.Schema) error {
	return forEach(ctx, in.c, qTables, []any{in.db}, func(r momo.Rows) error {
		var name string
		if err := r.Scan(&name); err != nil {
			return err
		}
		in.byName[name] = len(out.Tables)
		out.Tables = append(out.Tables, schema.Table{Name: name})
		return nil
	})
}

func (in *inspector) columns(ctx context.Context, out *schema.Schema) error {
	return forEach(ctx, in.c, qColumns, []any{in.db}, func(r momo.Rows) error {
		var table, name, ctype, nullable, extra sql.NullString
		var def, gen, comment sql.NullString
		if err := r.Scan(&table, &name, &ctype, &nullable, &def, &extra, &gen, &comment); err != nil {
			return err
		}
		t := in.table(out, table.String)
		if t == nil {
			return nil // a view
		}
		col := schema.Column{
			Name:     name.String,
			Type:     ctype.String,
			Nullable: strings.EqualFold(nullable.String, "YES"),
			Comment:  comment.String,
		}
		if strings.Contains(strings.ToLower(extra.String), "auto_increment") {
			col.Identity = "by_default"
		}
		if g := strings.TrimSpace(gen.String); g != "" {
			col.Generated = stripQuotes(g)
		} else {
			col.Default = in.columnDefault(def, extra.String, nullable.String, ctype.String)
		}
		t.Columns = append(t.Columns, col)
		return nil
	})
}

// columnDefault turns COLUMN_DEFAULT into the SQL expression a DEFAULT clause
// would carry. The servers disagree: MySQL reports a literal without quotes and
// marks an expression default with DEFAULT_GENERATED in EXTRA; MariaDB reports
// literals quoted, expressions as they are, and the string NULL for a nullable
// column without a default.
func (in *inspector) columnDefault(def sql.NullString, extra, nullable, ctype string) string {
	if !def.Valid {
		return ""
	}
	v := def.String
	switch in.flavor {
	case dialect.FlavorMariaDB:
		if strings.EqualFold(v, "NULL") && strings.EqualFold(nullable, "YES") {
			return ""
		}
		return stripQuotes(v)
	default:
		if strings.Contains(strings.ToUpper(extra), "DEFAULT_GENERATED") {
			return stripQuotes(v)
		}
		if numericType(ctype) {
			return v
		}
		return quoteString(v)
	}
}

var numericTypes = map[string]bool{
	"tinyint": true, "smallint": true, "mediumint": true, "int": true, "bigint": true,
	"decimal": true, "float": true, "double": true, "bit": true,
}

func numericType(ctype string) bool {
	base := strings.ToLower(ctype)
	if i := strings.IndexAny(base, "( "); i >= 0 {
		base = base[:i]
	}
	return numericTypes[base]
}

// quoteString renders v as a MySQL string literal.
func quoteString(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

// stripQuotes removes the identifier quotes of a stored expression and the
// character set introducers MySQL adds to its string literals.
func stripQuotes(s string) string {
	var sb strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			sb.WriteByte(c)
			switch {
			case c == '\\' && i+1 < len(s):
				i++
				sb.WriteByte(s[i])
			case c == quote:
				quote = 0
			}
			continue
		}
		switch {
		case c == '`':
			continue
		case c == '\'' || c == '"':
			quote = c
		case c == '_' && (i == 0 || !isWordByte(s[i-1])):
			if n := charsetIntroducer(s[i:]); n > 0 {
				i += n - 1
				continue
			}
		}
		sb.WriteByte(c)
	}
	return strings.TrimSpace(sb.String())
}

func isWordByte(c byte) bool {
	return c == '_' || c == '$' || c >= 0x80 || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// charsetIntroducer returns the length of a character set introducer such as
// _utf8mb4 at the start of s when a string literal follows it, otherwise 0.
func charsetIntroducer(s string) int {
	for _, cs := range []string{"_utf8mb4", "_utf8mb3", "_utf8", "_latin1", "_binary"} {
		if strings.HasPrefix(s, cs) && len(s) > len(cs) && s[len(cs)] == '\'' {
			return len(cs)
		}
	}
	return 0
}

type idxRow struct {
	unique  bool
	column  sql.NullString
	expr    sql.NullString
	subPart sql.NullInt64
	desc    bool
	itype   string
}

type idxGroup struct {
	table string
	name  string
	rows  []idxRow
}

// indexes reads information_schema.STATISTICS, grouped by table and index in
// the order the query returns them.
func (in *inspector) indexes(ctx context.Context) (pk map[string][]string, groups []idxGroup, err error) {
	q := qIndexesMySQL
	if in.flavor == dialect.FlavorMariaDB {
		q = qIndexesMariaDB
	}
	pk = map[string][]string{}
	err = forEach(ctx, in.c, q, []any{in.db}, func(r momo.Rows) error {
		var table, name, column, collation, itype, expr sql.NullString
		var nonUnique sql.NullInt64
		var sub sql.NullInt64
		if err := r.Scan(&table, &name, &nonUnique, &column, &sub, &collation, &itype, &expr); err != nil {
			return err
		}
		if name.String == "PRIMARY" {
			pk[table.String] = append(pk[table.String], column.String)
			return nil
		}
		row := idxRow{
			unique:  nonUnique.Int64 == 0,
			column:  column,
			subPart: sub,
			desc:    strings.EqualFold(collation.String, "D"),
			itype:   strings.ToUpper(itype.String),
		}
		if expr.Valid && expr.String != "" {
			row.expr = sql.NullString{String: stripQuotes(expr.String), Valid: true}
		}
		if n := len(groups); n > 0 && groups[n-1].table == table.String && groups[n-1].name == name.String {
			groups[n-1].rows = append(groups[n-1].rows, row)
		} else {
			groups = append(groups, idxGroup{table: table.String, name: name.String, rows: []idxRow{row}})
		}
		return nil
	})
	return pk, groups, err
}

// applyIndexes sets the primary keys, then files each index as a Unique or an
// Index. Foreign keys must be read first: the index the server creates for one
// is dropped here.
func (in *inspector) applyIndexes(out *schema.Schema, pk map[string][]string, groups []idxGroup) {
	for name, cols := range pk {
		if t := in.table(out, name); t != nil {
			t.PrimaryKey = cols
		}
	}
	for _, g := range groups {
		t := in.table(out, g.table)
		if t == nil {
			continue
		}
		cols := make([]string, len(g.rows))
		plain := true
		for i, r := range g.rows {
			if !r.column.Valid || r.desc || r.subPart.Valid {
				plain = false
			}
			cols[i] = r.column.String
		}
		first := g.rows[0]
		method := ""
		switch first.itype {
		case "BTREE", "":
		default:
			method = strings.ToLower(first.itype)
		}
		if first.unique && plain && method == "" {
			t.Uniques = append(t.Uniques, schema.Unique{Name: g.name, Columns: cols})
			continue
		}
		if !first.unique && plain && method == "" && implicitFKIndex(t, g.name, cols) {
			continue
		}
		ix := schema.Index{Name: g.name, Unique: first.unique, Method: method}
		for _, r := range g.rows {
			part := schema.IndexPart{Desc: r.desc}
			switch {
			case r.expr.Valid:
				part.Expr = r.expr.String
			case r.subPart.Valid:
				part.Column = r.column.String + "(" + strconv.FormatInt(r.subPart.Int64, 10) + ")"
			default:
				part.Column = r.column.String
			}
			ix.Parts = append(ix.Parts, part)
		}
		t.Indexes = append(t.Indexes, ix)
	}
}

// implicitFKIndex reports whether an index is the one the server created for a
// foreign key: it carries the constraint's name and exactly its columns.
func implicitFKIndex(t *schema.Table, name string, cols []string) bool {
	for _, fk := range t.ForeignKeys {
		if fk.Name == name && strings.Join(fk.Columns, "\x00") == strings.Join(cols, "\x00") {
			return true
		}
	}
	return false
}

func (in *inspector) foreignKeys(ctx context.Context, out *schema.Schema) error {
	return forEach(ctx, in.c, qForeignKeys, []any{in.db}, func(r momo.Rows) error {
		var table, name, col, refTable, refCol, upd, del sql.NullString
		if err := r.Scan(&table, &name, &col, &refTable, &refCol, &upd, &del); err != nil {
			return err
		}
		t := in.table(out, table.String)
		if t == nil {
			return nil
		}
		if n := len(t.ForeignKeys); n > 0 && t.ForeignKeys[n-1].Name == name.String {
			fk := &t.ForeignKeys[n-1]
			fk.Columns = append(fk.Columns, col.String)
			fk.RefColumns = append(fk.RefColumns, refCol.String)
			return nil
		}
		t.ForeignKeys = append(t.ForeignKeys, schema.ForeignKey{
			Name: name.String, Columns: []string{col.String}, RefTable: refTable.String, RefColumns: []string{refCol.String},
			OnDelete: in.fkAction(del.String), OnUpdate: in.fkAction(upd.String),
		})
		return nil
	})
}

// fkAction maps a referential rule to the schema spelling, where empty means
// the default. NO ACTION is the default; MariaDB also reports its default as
// RESTRICT, so there RESTRICT cannot be told from an unwritten clause.
func (in *inspector) fkAction(rule string) string {
	switch r := strings.ToUpper(strings.TrimSpace(rule)); r {
	case "NO ACTION":
		return ""
	case "RESTRICT":
		if in.flavor == dialect.FlavorMariaDB {
			return ""
		}
		return r
	default:
		return r
	}
}

func (in *inspector) checks(ctx context.Context, out *schema.Schema) error {
	q := qChecksMySQL
	if in.flavor == dialect.FlavorMariaDB {
		q = qChecksMariaDB
	}
	return forEach(ctx, in.c, q, []any{in.db}, func(r momo.Rows) error {
		var table, name, clause sql.NullString
		if err := r.Scan(&table, &name, &clause); err != nil {
			return err
		}
		t := in.table(out, table.String)
		if t == nil {
			return nil
		}
		t.Checks = append(t.Checks, schema.Check{Name: name.String, Expr: stripQuotes(clause.String)})
		return nil
	})
}

var jsonValid = regexp.MustCompile(`(?i)^json_valid\(\s*([^)\s]+)\s*\)$`)

// jsonColumns folds MariaDB's JSON alias back into the type it was declared
// as. MariaDB creates "col json" as LONGTEXT with a check named after the
// column, json_valid(col); reporting both would show a type change and an
// extra check that no model declares.
func (in *inspector) jsonColumns(out *schema.Schema) {
	if in.flavor != dialect.FlavorMariaDB {
		return
	}
	for i := range out.Tables {
		t := &out.Tables[i]
		for j := range t.Columns {
			col := &t.Columns[j]
			if !strings.EqualFold(col.Type, "longtext") {
				continue
			}
			for k, ck := range t.Checks {
				m := jsonValid.FindStringSubmatch(strings.TrimSpace(ck.Expr))
				if ck.Name == col.Name && m != nil && m[1] == col.Name {
					col.Type = "json"
					t.Checks = append(t.Checks[:k], t.Checks[k+1:]...)
					break
				}
			}
		}
	}
}
