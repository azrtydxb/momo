// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a MySQL column type mapped to the wrong Go type, for example an
// unsigned key read as a signed int, tinyint(1) not a bool, or a decimal that
// is not types.Numeric.
func TestMapTypeMySQL(t *testing.T) {
	cases := []struct {
		in   string
		want string
		size int
	}{
		{"bigint unsigned", "uint64", 0},
		{"bigint(20) unsigned", "uint64", 0},
		{"int", "int32", 0},
		{"int(11)", "int32", 0},
		{"int unsigned", "uint32", 0},
		{"mediumint", "int32", 0},
		{"smallint", "int16", 0},
		{"tinyint(1)", "bool", 0},
		{"tinyint(4)", "int8", 0},
		{"tinyint unsigned", "uint8", 0},
		{"tinyint(1) unsigned", "uint8", 0},
		{"float", "float32", 0},
		{"double", "float64", 0},
		{"decimal(10,2)", "types.Numeric", 0},
		{"decimal(10,0)", "types.Numeric", 0},
		{"varchar(100)", "string", 100},
		{"char(3)", "string", 0},
		{"text", "string", 0},
		{"longtext", "string", 0},
		{"enum('a','b')", "string", 0},
		{"set('x','y')", "string", 0},
		{"enum('a)b','c')", "string", 0},
		{"set('x,y','z)')", "string", 0},
		{"enum('it''s','(')", "string", 0},
		{"blob", "[]byte", 0},
		{"varbinary(16)", "[]byte", 0},
		{"date", "types.Date", 0},
		{"datetime(6)", "time.Time", 0},
		{"timestamp", "time.Time", 0},
		{"time", "string", 0},
		{"year", "int16", 0},
		{"json", "types.JSON[json.RawMessage]", 0},
	}
	for _, c := range cases {
		got, err := MapType(dialect.MySQL, c.in)
		if err != nil {
			t.Errorf("MapType(mysql, %q): %v", c.in, err)
			continue
		}
		if got.Expr != c.want || got.Size != c.size {
			t.Errorf("MapType(mysql, %q) = %+v; want %s size %d", c.in, got, c.want, c.size)
		}
	}
	if g, _ := MapType(dialect.MySQL, "decimal(10,2)"); g.Import != importTypes {
		t.Errorf("decimal import = %q, want the types package", g.Import)
	}
	if g, _ := MapType(dialect.MySQL, "tinyint(1)"); !strings.Contains(g.Note, "introspect.types") || !strings.Contains(g.Note, "fails to scan") {
		t.Errorf("tinyint(1) note = %q, want the scan warning and the rule to add", g.Note)
	}
	if g, _ := MapType(dialect.MySQL, "tinyint"); g.Note != "" {
		t.Errorf("tinyint has no note, got %q", g.Note)
	}
	if g, _ := MapType(dialect.MySQL, "blob"); !g.Nilable {
		t.Error("blob must be nilable, so a NULL column gets the nullable tag")
	}
}

// Catches: a MySQL type with no Go mapping silently becoming a string.
func TestMapTypeMySQLUnmapped(t *testing.T) {
	for _, in := range []string{"geometry", "point", "vector(3)", "int unsigned foo"} {
		if _, err := MapType(dialect.MySQL, in); !errors.Is(err, ErrUnmapped) {
			t.Errorf("%q: want ErrUnmapped, got %v", in, err)
		}
	}
}

// Catches: a type tag value the tag grammar would split into two options, so
// the model no longer declares the column the database has.
func TestTypeTagSafe(t *testing.T) {
	for in, want := range map[string]bool{
		"decimal(12,2)":     true,
		"enum('a','b')":     true,
		"enum('a)b','c')":   true,
		"enum('(','b')":     true,
		"enum('it''s','b')": true,
		"enum('a,b')":       true,
		"bigint unsigned":   true,
		"enum('a','b'":      false,
		"decimal(1,2),x":    false,
		"enum('a\\b')":      false,
		"enum(\"a\",\"b\")": false,
		"":                  false,
	} {
		if got := typeTagSafe(in); got != want {
			t.Errorf("typeTagSafe(%q) = %v, want %v", in, got, want)
		}
	}
}

func mysqlSchema() *schema.Schema {
	return &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.MySQL,
		Tables: []schema.Table{
			{
				Name: "users",
				Columns: []schema.Column{
					{Name: "id", Type: "bigint unsigned", Identity: "by_default"},
					{Name: "email", Type: "varchar(200)"},
					{Name: "balance", Type: "decimal(12,2)", Default: "0.00"},
					{Name: "active", Type: "tinyint(1)", Default: "1"},
					{Name: "level", Type: "int", Default: "5"},
					{Name: "score", Type: "int", Default: "0"},
					{Name: "status", Type: "enum('new','closed')", Default: "'new'"},
					{Name: "perms", Type: "set('r','w')", Nullable: true},
					{Name: "settings", Type: "json", Nullable: true},
					{Name: "stamp", Type: "datetime(6)", Default: "CURRENT_TIMESTAMP(6)"},
					{Name: "email_lower", Type: "varchar(200)", Nullable: true, Generated: "lower(email)"},
					{Name: "email_len", Type: "int", Nullable: true, Generated: "char_length(email)", GeneratedVirtual: true},
				},
				PrimaryKey: []string{"id"},
				Uniques:    []schema.Unique{{Name: "users_email_key", Columns: []string{"email"}}},
				Indexes:    []schema.Index{{Name: "users_level_idx", Parts: []schema.IndexPart{{Column: "level"}, {Column: "score", Desc: true}}}},
				Checks:     []schema.Check{{Name: "users_level_chk", Expr: "(level < 100)"}},
			},
			{
				Name: "posts",
				Columns: []schema.Column{
					{Name: "id", Type: "bigint", Identity: "by_default"},
					{Name: "user_id", Type: "bigint unsigned"},
				},
				PrimaryKey: []string{"id"},
				ForeignKeys: []schema.ForeignKey{{
					Name: "posts_ibfk_1", Columns: []string{"user_id"}, RefTable: "users", RefColumns: []string{"id"}, OnDelete: "CASCADE",
				}},
			},
		},
	}
}

func generateMySQL(t *testing.T, s *schema.Schema, cfg Config) (*Result, map[string]string) {
	t.Helper()
	return generateFlavor(t, s, cfg, dialect.FlavorMySQL, nil)
}

func generateFlavor(t *testing.T, s *schema.Schema, cfg Config, flavor dialect.Flavor, extra []Finding) (*Result, map[string]string) {
	t.Helper()
	res, err := Generate(s, Options{Package: "models", Dialect: dialect.MySQL, Flavor: flavor, Extra: extra, Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range res.Files {
		files[f.Table] = string(f.Content)
	}
	return res, files
}

func hasFinding(res *Result, sev Severity, contains string) bool {
	for _, f := range res.Findings {
		if f.Severity == sev && strings.Contains(f.String(), contains) {
			return true
		}
	}
	return false
}

// Catches: a MySQL model that does not declare the column the database has:
// the type tag, unsigned keys, enum and decimal types, generated columns
// (STORED and VIRTUAL), checks, a unique key, the has_many inverse and the
// foreign key; and, on MySQL, a dbdefault tag that would make an insert with a
// nil field fail for lack of RETURNING.
func TestGenerateFromDBMySQL(t *testing.T) {
	res, files := generateMySQL(t, mysqlSchema(), Config{Inverse: true})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	mustMatch(t, "users", files["users"],
		"ID\\s+uint64\\s+`momo:\"pk,identity,type:bigint unsigned\"`",
		`Email\s+string\s+`+"`momo:\"size:200",
		`Balance\s+types\.Numeric\s+`+"`momo:\"type:decimal\\(12,2\\),default:0.00\"`",
		`Active\s+bool\s+`+"`momo:\"type:tinyint\\(1\\),default:1\"`",
		`Level\s+int32\s+`+"`momo:\"type:int,default:5\"`",
		`Score\s+int32\s+`+"`momo:\"type:int,default:0\"`",
		`Status\s+string\s+`+"`momo:\"type:enum\\('new','closed'\\),default:'new'\"`",
		`Perms\s+\*string\s+`+"`momo:\"type:set\\('r','w'\\)\"`",
		`Settings\s+\*types\.JSON\[json\.RawMessage\]`,
		`Stamp\s+time\.Time\s+`+"`momo:\"type:datetime\\(6\\),default:CURRENT_TIMESTAMP\\(6\\)\"`",
		`EmailLower\s+\*string\s+`+"`momo:\"size:200,read_only,generated:lower\\(email\\)\"`",
		`EmailLen\s+\*int32\s+`+"`momo:\"type:int,read_only,generated:char_length\\(email\\),virtual\"`",
		`c\.Check\("users_level_chk", "\(level < 100\)"\)`,
		`c\.Unique\("users_email_key", &m\.Email\)`,
		`has_many`,
	)
	if strings.Contains(files["users"], "dbdefault") {
		t.Errorf("MySQL has no RETURNING, so no field may be tagged dbdefault:\n%s", files["users"])
	}
	mustMatch(t, "posts", files["posts"],
		"UserID\\s+uint64\\s+`momo:\"type:bigint unsigned\"`",
		`belongs_to,fk:UserID,fk_name:posts_ibfk_1,on_delete:cascade`,
	)
	for _, want := range []string{"users.active", "users.level"} {
		if !hasFinding(res, SevNote, want+": the default") || !hasFinding(res, SevNote, "a zero value is written instead of the column default") {
			t.Errorf("no note that %s keeps a plain field: %v", want, res.Findings)
		}
	}
	if hasFinding(res, SevNote, "users.score: the default") {
		t.Errorf("a zero default needs no note: %v", res.Findings)
	}
	if !hasFinding(res, SevNote, "users.active: tinyint(1) is a Go bool") {
		t.Errorf("no tinyint(1) note: %v", res.Findings)
	}
}

// Catches: MariaDB, which has INSERT ... RETURNING, not getting the dbdefault
// pointer for a non-zero constant default (an explicit false or 0 would be
// replaced by the default), or a zero or computed default getting it.
func TestGenerateFromDBMariaDBDBDefault(t *testing.T) {
	res, files := generateFlavor(t, mysqlSchema(), Config{}, dialect.FlavorMariaDB, nil)
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	mustMatch(t, "users", files["users"],
		`Active\s+\*bool\s+`+"`momo:\"type:tinyint\\(1\\),default:1,dbdefault\"`",
		`Level\s+\*int32\s+`+"`momo:\"type:int,default:5,dbdefault\"`",
		`Score\s+int32\s+`+"`momo:\"type:int,default:0\"`",
		`Stamp\s+time\.Time\s+`+"`momo:\"type:datetime\\(6\\),default:CURRENT_TIMESTAMP\\(6\\)\"`",
	)
	if hasFinding(res, SevNote, "a zero value is written") {
		t.Errorf("MariaDB needs no dbdefault note: %v", res.Findings)
	}
}

// Catches: a foreign key to a non-primary unique column being dropped, so a
// database rebuilt from the models lacks it.
func TestGenerateFromDBMySQLForeignKeyToUniqueColumn(t *testing.T) {
	s := mysqlSchema()
	s.Tables[1].Columns = append(s.Tables[1].Columns, schema.Column{Name: "owner_email", Type: "varchar(200)", Nullable: true})
	s.Tables[1].ForeignKeys = append(s.Tables[1].ForeignKeys, schema.ForeignKey{
		Name: "posts_owner_fk", Columns: []string{"owner_email"}, RefTable: "users", RefColumns: []string{"email"},
	})
	res, files := generateMySQL(t, s, Config{})
	if res.Failed() || hasFinding(res, SevWarn, "posts_owner_fk") {
		t.Fatalf("findings: %v", res.Findings)
	}
	mustContain(t, "posts", files["posts"], `c.ForeignKey("posts_owner_fk", []any{&m.OwnerEmail}, "users", []string{"email"})`)
}

// Catches: the findings the caller read from the server (ON UPDATE, a prefix
// primary key) being lost, or an error among them not stopping the run.
func TestGenerateFromDBMySQLExtraFindings(t *testing.T) {
	warn := Finding{Severity: SevWarn, Object: "users.stamp", Msg: "ON UPDATE is not modeled"}
	res, _ := generateFlavor(t, mysqlSchema(), Config{}, dialect.FlavorMySQL, []Finding{warn})
	if res.Failed() || !hasFinding(res, SevWarn, "users.stamp: ON UPDATE is not modeled") {
		t.Fatalf("a warning must be reported and not stop the run: %v", res.Findings)
	}
	bad := Finding{Severity: SevError, Object: "users.email", Msg: "the primary key uses a column prefix"}
	if res, _ = generateFlavor(t, mysqlSchema(), Config{}, dialect.FlavorMySQL, []Finding{bad}); !res.Failed() {
		t.Fatal("an error finding must stop the run")
	}
}

// Catches: an unmappable MySQL type falling back to a guess instead of an
// error that names the introspect.types rule.
func TestGenerateFromDBMySQLUnmapped(t *testing.T) {
	s := &schema.Schema{Dialect: dialect.MySQL, Tables: []schema.Table{{
		Name:       "places",
		Columns:    []schema.Column{{Name: "id", Type: "int"}, {Name: "spot", Type: "geometry"}},
		PrimaryKey: []string{"id"},
	}}}
	res, _ := generateMySQL(t, s, Config{})
	if !res.Failed() {
		t.Fatal("a geometry column must be an error")
	}
	if !strings.Contains(res.Findings[0].Msg, "introspect.types") {
		t.Errorf("finding %q does not name the rule to add", res.Findings[0].Msg)
	}
}

// Catches: dbdefault on a flavor without RETURNING, on a nullable, identity,
// zero or computed default, or the MariaDB quoted form of a constant default
// ('5', as MariaDB reports a literal) not being read as a non-zero constant.
func TestMySQLDBDefaultRules(t *testing.T) {
	gt := GoType{Expr: "int32"}
	cases := []struct {
		name   string
		col    schema.Column
		flavor dialect.Flavor
		want   bool
	}{
		{"mariadb constant", schema.Column{Default: "5"}, dialect.FlavorMariaDB, true},
		{"mariadb quoted constant", schema.Column{Default: "'5'"}, dialect.FlavorMariaDB, true},
		{"mariadb quoted zero", schema.Column{Default: "'0'"}, dialect.FlavorMariaDB, false},
		{"mariadb zero", schema.Column{Default: "0"}, dialect.FlavorMariaDB, false},
		{"mariadb nullable", schema.Column{Default: "5", Nullable: true}, dialect.FlavorMariaDB, false},
		{"mariadb identity", schema.Column{Default: "5", Identity: "by_default"}, dialect.FlavorMariaDB, false},
		{"mariadb none", schema.Column{}, dialect.FlavorMariaDB, false},
		{"mysql constant", schema.Column{Default: "5"}, dialect.FlavorMySQL, false},
		{"no flavor", schema.Column{Default: "5"}, "", false},
	}
	for _, c := range cases {
		if got := mysqlDBDefault(dialect.MySQL, c.flavor, &c.col, gt, nil); got != c.want {
			t.Errorf("%s: mysqlDBDefault = %v, want %v", c.name, got, c.want)
		}
	}
	if !mysqlNonZeroDefault(dialect.MySQL, &schema.Column{Default: "'5'"}, gt, nil) {
		t.Error("a quoted non-zero constant is a non-zero default")
	}
	if mysqlDBDefault(dialect.Postgres, dialect.FlavorMariaDB, &schema.Column{Default: "5"}, gt, nil) {
		t.Error("postgres has its own rule")
	}
	if mysqlDBDefault(dialect.MySQL, dialect.FlavorMariaDB, &schema.Column{Default: "CURRENT_TIMESTAMP"}, GoType{Expr: "time.Time"}, nil) {
		t.Error("a computed default must not get dbdefault")
	}
}
