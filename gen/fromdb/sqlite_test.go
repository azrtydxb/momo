// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// generateSQLite runs Generate on a schema as the SQLite inspector reports it.
func generateSQLite(t *testing.T, s *schema.Schema, cfg Config) (*Result, map[string]string) {
	t.Helper()
	res, err := Generate(s, Options{Package: "models", Dialect: dialect.SQLite, Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range res.Files {
		files[f.Table] = string(f.Content)
	}
	return res, files
}

// sqliteSchema is what the SQLite inspector reports for a small database: a
// rowid table, an AUTOINCREMENT table, an enum, defaults, generated columns,
// every kind of index, a foreign key, a view and a trigger.
func sqliteSchema() *schema.Schema {
	return &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.SQLite,
		Enums:         []schema.Enum{{Name: "accounts_status", Values: []string{"new", "active", "closed"}}},
		Tables: []schema.Table{
			{
				Name: "accounts",
				Columns: []schema.Column{
					{Name: "id", Type: "INTEGER", Identity: "by_default"},
					{Name: "tenant_id", Type: "INTEGER"},
					{Name: "email", Type: "VARCHAR(200)"},
					{Name: "nickname", Type: "TEXT", Nullable: true},
					{Name: "active", Type: "BOOLEAN", Default: "1"},
					{Name: "score", Type: "REAL", Default: "0"},
					{Name: "created_at", Type: "DATETIME", Default: "CURRENT_TIMESTAMP"},
					{Name: "status", Type: "TEXT", Default: "'new'", Enum: "accounts_status"},
					{Name: "email_lower", Type: "TEXT", Nullable: true, Generated: "lower(email)"},
					{Name: "initial", Type: "TEXT", Nullable: true, Generated: "substr(email, 1, 1)", GeneratedVirtual: true},
					{Name: "avatar", Type: "BLOB", Nullable: true},
				},
				PrimaryKey: []string{"id"},
				Checks:     []schema.Check{{Name: "accounts_score_check", Expr: "score >= 0"}},
				ForeignKeys: []schema.ForeignKey{{
					Name: "accounts_tenant_id_fkey", Columns: []string{"tenant_id"}, RefTable: "tenants",
					RefColumns: []string{"id"}, OnDelete: "CASCADE",
				}},
				Indexes: []schema.Index{
					{Name: "accounts_email_key", Unique: true, Parts: []schema.IndexPart{{Column: "tenant_id"}, {Column: "email"}}},
					{Name: "accounts_nickname_idx", Parts: []schema.IndexPart{{Column: "nickname", Desc: true}}},
					{Name: "accounts_active_idx", Where: "active = 1", Parts: []schema.IndexPart{{Column: "tenant_id"}}},
					{Name: "accounts_email_lower_key", Unique: true, Parts: []schema.IndexPart{{Expr: "lower(email)"}}},
				},
			},
			{
				Name: "tenants", Autoincrement: true,
				Columns: []schema.Column{
					{Name: "id", Type: "INTEGER", Identity: "by_default"},
					{Name: "name", Type: "TEXT"},
				},
				PrimaryKey: []string{"id"},
				Uniques:    []schema.Unique{{Name: "tenants_name_key", Columns: []string{"name"}}},
			},
			{
				Name: "settings", WithoutRowID: true, Strict: true,
				Columns: []schema.Column{
					{Name: "k", Type: "TEXT"},
					{Name: "v", Type: "TEXT", Nullable: true},
				},
				PrimaryKey: []string{"k"},
			},
		},
		Views:    []schema.View{{Name: "active_accounts", Query: "SELECT id, email FROM accounts WHERE active = 1", Depends: []string{"accounts"}}},
		Triggers: []schema.Trigger{{Name: "accounts_ai", Table: "accounts", Definition: "CREATE TRIGGER accounts_ai AFTER INSERT ON accounts BEGIN SELECT 1; END"}},
	}
}

// Catches: the SQLite type mapping giving a declared type the wrong Go type, or
// leaving a common spelling (VARCHAR(n), BIGINT, DOUBLE PRECISION) unmapped, so
// a real database fails with "unmapped column type" or loses its precision.
func TestMapTypeSQLiteAffinity(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"INTEGER", "int64"}, {"int", "int64"}, {"BIGINT", "int64"}, {"UNSIGNED BIG INT", "int64"},
		{"TEXT", "string"}, {"VARCHAR(200)", "string"}, {"nvarchar(10)", "string"}, {"CLOB", "string"},
		{"BLOB", "[]byte"}, {"REAL", "float64"}, {"FLOAT", "float64"}, {"DOUBLE PRECISION", "float64"},
		{"BOOLEAN", "bool"}, {"DATETIME", "time.Time"}, {"TIMESTAMP", "time.Time"}, {"DATE", "types.Date"},
		{"NUMERIC", "float64"},
	} {
		got, err := MapType(dialect.SQLite, c.in)
		if err != nil {
			t.Errorf("MapType(sqlite, %q): %v", c.in, err)
			continue
		}
		if got.Expr != c.want || got.Size != 0 {
			t.Errorf("MapType(sqlite, %q) = %+v, want %s", c.in, got, c.want)
		}
	}
	if g, _ := MapType(dialect.SQLite, "BLOB"); !g.Nilable {
		t.Error("a BLOB column holds NULL as a nil slice, so it must be Nilable")
	}
}

// Catches: a type with NUMERIC affinity (DECIMAL(10,2), JSON) or no declared
// type being given a guessed Go type, which would store decimals as float64
// without anyone choosing it.
func TestMapTypeSQLiteUnmapped(t *testing.T) {
	for _, in := range []string{"DECIMAL(10,2)", "JSON", "GEOMETRY", "", "ANY"} {
		if _, err := MapType(dialect.SQLite, in); !errors.Is(err, ErrUnmapped) {
			t.Errorf("MapType(sqlite, %q): want ErrUnmapped, got %v", in, err)
		}
	}
	res, _ := generateSQLite(t, &schema.Schema{Dialect: dialect.SQLite, Tables: []schema.Table{
		{Name: "t", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "price", Type: "DECIMAL(10,2)"}}, PrimaryKey: []string{"id"}},
	}}, Config{})
	if !res.Failed() {
		t.Fatalf("a DECIMAL column must fail the run: %v", res.Findings)
	}
}

// Catches: the inspected SQLite schema not turning into the models its
// features need: the rowid alias, the table options, the type tags that keep
// the declared type, defaults, enum, generated columns, indexes, foreign key,
// view and trigger.
func TestGenerateSQLiteModels(t *testing.T) {
	res, files := generateSQLite(t, sqliteSchema(), Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	acct := files["accounts"]
	mustMatch(t, "accounts", acct,
		"ID\\s+int64\\s+`momo:\"pk,identity\"`",
		"TenantID\\s+int64\\s*\n",
		"Email\\s+string\\s+`momo:\"type:VARCHAR\\(200\\)\"`",
		"Nickname\\s+\\*string\\s+`momo:\"index,sort:desc\"`",
		"Active\\s+bool\\s+`momo:\"type:BOOLEAN,default:1\"`",
		"Score\\s+float64\\s+`momo:\"default:0\"`",
		"CreatedAt\\s+time\\.Time\\s+`momo:\"type:DATETIME,default:CURRENT_TIMESTAMP,dbdefault\"`",
		"Status\\s+string\\s+`momo:\"enum:accounts_status,default:'new'\"`",
		"EmailLower\\s+\\*string\\s+`momo:\"read_only,generated:lower\\(email\\)\"`",
		"Initial\\s+\\*string\\s+`momo:\"read_only\"`",
		"Avatar\\s+\\[\\]byte\\s+`momo:\"nullable\"`",
		"Tenant\\s+\\*Tenant\\s+`momo:\"belongs_to,fk:TenantID,on_delete:cascade\"`",
	)
	mustContain(t, "accounts", acct,
		`c.GeneratedVirtual(&m.Initial, "substr(email, 1, 1)")`,
		`c.Index("accounts_email_key", &m.TenantID, &m.Email).Unique()`,
		`c.Index("accounts_active_idx", &m.TenantID).Where("active = 1")`,
		`c.IndexExpr("accounts_email_lower_key", "lower(email)").Unique()`,
		`c.Check("accounts_score_check", "score >= 0")`,
		`c.Enum("accounts_status", "new", "active", "closed")`,
		`c.View(schema.View{Name: "active_accounts"`,
		`c.Trigger(schema.Trigger{Table: "accounts", Name: "accounts_ai"`,
	)
	if strings.Contains(acct, `Method("btree")`) {
		t.Errorf("a SQLite unique index must not carry the btree method, which inspection never reports:\n%s", acct)
	}
	mustMatch(t, "tenants", files["tenants"], "momo\\.Model\\s+`momo:\"autoincrement\"`")
	mustContain(t, "tenants", files["tenants"], `c.Unique("tenants_name_key", &m.Name)`)
	mustMatch(t, "settings", files["settings"], "momo\\.Model\\s+`momo:\"without_rowid,strict\"`")
	for _, tbl := range res.Snapshot.Tables {
		if tbl.Name != "accounts" {
			continue
		}
		if col, _ := tbl.Column("status"); col.Type != "TEXT" {
			t.Errorf("an enum column stays TEXT on SQLite (a CHECK, not a type): %+v", col)
		}
	}
}

// Catches: introspect.dbdefault: all stopping at PostgreSQL, so a SQLite
// column with a constant default keeps a plain field and an explicit false or
// 0 is replaced by the default.
func TestGenerateSQLiteDBDefaultAll(t *testing.T) {
	res, files := generateSQLite(t, sqliteSchema(), Config{DBDefault: "all"})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	mustMatch(t, "accounts", files["accounts"],
		"Active\\s+\\*bool\\s+`momo:\"type:BOOLEAN,default:1,dbdefault\"`",
		"Score\\s+float64\\s+`momo:\"default:0,dbdefault\"`",
	)
}

// Catches: dbdefault being written for MySQL after SQLite joined PostgreSQL,
// where a zero value has no RETURNING to read the stored default back.
func TestDBDefaultFitsSQLite(t *testing.T) {
	col := schema.Column{Default: "CURRENT_TIMESTAMP"}
	if !dbDefaultFits(dialect.SQLite, &col, nil) {
		t.Error("sqlite: a computed default on a NOT NULL column takes dbdefault")
	}
	if dbDefaultFits(dialect.MySQL, &col, nil) {
		t.Error("mysql: dbdefault must not be emitted")
	}
	nullable := schema.Column{Default: "CURRENT_TIMESTAMP", Nullable: true}
	if dbDefaultFits(dialect.SQLite, &nullable, nil) {
		t.Error("sqlite: a nullable column never takes dbdefault")
	}
}

// Catches: a SQLite primary key column that allows NULL (TEXT PRIMARY KEY with
// no NOT NULL) becoming a nullable key field, which the generator refuses, or
// the snapshot keeping a nullability the models cannot declare.
func TestGenerateSQLiteNullablePrimaryKey(t *testing.T) {
	s := &schema.Schema{Dialect: dialect.SQLite, Tables: []schema.Table{{
		Name:       "codes",
		Columns:    []schema.Column{{Name: "code", Type: "TEXT", Nullable: true}, {Name: "label", Type: "TEXT", Nullable: true}},
		PrimaryKey: []string{"code"},
	}}}
	res, files := generateSQLite(t, s, Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	mustMatch(t, "codes", files["codes"], "Code\\s+string\\s+`momo:\"pk\"`", "Label\\s+\\*string")
	if col, _ := res.Snapshot.Tables[0].Column("code"); col.Nullable {
		t.Error("the snapshot must hold the NOT NULL the model declares")
	}
	warned := false
	for _, f := range res.Findings {
		warned = warned || (f.Severity == SevWarn && f.Object == "codes.code")
	}
	if !warned {
		t.Errorf("a changed nullability must be reported: %v", res.Findings)
	}
}

// Catches: has_one and has_many inverses depending on PostgreSQL details, so
// introspect.inverse writes nothing, or the wrong side, for a SQLite database.
func TestGenerateSQLiteInverse(t *testing.T) {
	s := &schema.Schema{Dialect: dialect.SQLite, Tables: []schema.Table{
		{
			Name:       "parents",
			Columns:    []schema.Column{{Name: "id", Type: "INTEGER", Identity: "by_default"}},
			PrimaryKey: []string{"id"},
		},
		{
			Name: "kids",
			Columns: []schema.Column{
				{Name: "id", Type: "INTEGER", Identity: "by_default"},
				{Name: "parent_id", Type: "INTEGER"},
			},
			PrimaryKey:  []string{"id"},
			ForeignKeys: []schema.ForeignKey{{Name: "kids_parent_id_fkey", Columns: []string{"parent_id"}, RefTable: "parents", RefColumns: []string{"id"}}},
		},
		{
			Name: "notes",
			Columns: []schema.Column{
				{Name: "id", Type: "INTEGER", Identity: "by_default"},
				{Name: "parent_id", Type: "INTEGER"},
			},
			PrimaryKey:  []string{"id"},
			Uniques:     []schema.Unique{{Name: "notes_parent_id_key", Columns: []string{"parent_id"}}},
			ForeignKeys: []schema.ForeignKey{{Name: "notes_parent_id_fkey", Columns: []string{"parent_id"}, RefTable: "parents", RefColumns: []string{"id"}}},
		},
	}}
	res, files := generateSQLite(t, s, Config{Inverse: true})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	mustMatch(t, "parents", files["parents"],
		"Kids\\s+\\[\\]Kid\\s+`momo:\"has_many,fk:ParentID\"`",
		"Note\\s+\\*Note\\s+`momo:\"has_one,fk:ParentID\"`",
	)
}

// Catches: the dialect gate rejecting SQLite or MySQL, or accepting a dialect
// with no inspector.
func TestGenerateDialectGate(t *testing.T) {
	if _, err := Generate(&schema.Schema{}, Options{Package: "models", Dialect: dialect.SQLite}); err != nil {
		t.Errorf("sqlite: %v", err)
	}
	if _, err := Generate(&schema.Schema{}, Options{Package: "models", Dialect: dialect.MySQL}); err != nil {
		t.Errorf("mysql: %v", err)
	}
	if _, err := Generate(&schema.Schema{}, Options{Package: "models", Dialect: dialect.Name("oracle")}); err == nil {
		t.Error("a dialect with no inspector must be refused")
	}
}
