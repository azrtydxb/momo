// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/schema"
)

// sampleSchema is what an inspector reports for a small shop database.
func sampleSchema() *schema.Schema {
	return &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Enums:         []schema.Enum{{Name: "user_role", Values: []string{"admin", "member"}}},
		Tables: []schema.Table{
			{
				Name: "orders",
				Columns: []schema.Column{
					{Name: "id", Type: "bigint", Identity: "serial"},
					{Name: "user_id", Type: "bigint"},
					{Name: "note", Type: "text", Default: "'a,b'::text"},
					{Name: "placed_at", Type: "timestamp without time zone"},
					{Name: "total", Type: "integer"},
				},
				PrimaryKey: []string{"id"},
				Indexes: []schema.Index{{
					Name:  "orders_user_placed_idx",
					Parts: []schema.IndexPart{{Column: "user_id"}, {Column: "placed_at", Desc: true}},
				}},
				ForeignKeys: []schema.ForeignKey{{
					Name: "orders_user_id_fkey", Columns: []string{"user_id"}, RefTable: "users",
					RefColumns: []string{"id"}, OnDelete: "CASCADE",
				}},
				Checks: []schema.Check{{Name: "orders_total_check", Expr: "total > 0"}},
			},
			{
				Name: "users",
				Columns: []schema.Column{
					{Name: "id", Type: "bigint", Identity: "by_default"},
					{Name: "email", Type: "character varying(200)"},
					{Name: "full_name", Type: "text", Nullable: true},
					{Name: "created_at", Type: "timestamp with time zone", Default: "now()"},
					{Name: "meta", Type: "jsonb", Nullable: true},
					{Name: "role", Type: "user_role", Enum: "user_role"},
					{Name: "tags", Type: "text[]", Nullable: true},
					{Name: "balance", Type: "numeric(10,2)"},
				},
				PrimaryKey: []string{"id"},
				Indexes: []schema.Index{{
					Name: "users_email_key", Unique: true, Parts: []schema.IndexPart{{Column: "email"}},
				}},
			},
		},
	}
}

func generate(t *testing.T, s *schema.Schema, cfg Config) (*Result, map[string]string) {
	t.Helper()
	res, err := Generate(s, Options{Package: "models", Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range res.Files {
		files[f.Table] = string(f.Content)
	}
	return res, files
}

func mustContain(t *testing.T, what, src string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(src, w) {
			t.Errorf("%s lacks %q:\n%s", what, w, src)
		}
	}
}

// mustMatch checks regular expressions, so the assertions do not depend on how
// gofmt aligns the columns of a struct.
func mustMatch(t *testing.T, what, src string, patterns ...string) {
	t.Helper()
	for _, p := range patterns {
		if !regexp.MustCompile(p).MatchString(src) {
			t.Errorf("%s does not match %q:\n%s", what, p, src)
		}
	}
}

func TestGenerateFromDB(t *testing.T) {
	res, files := generate(t, sampleSchema(), Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	if len(res.Files) != 2 || res.Files[0].Name != "orders_model.go" || res.Files[1].Name != "users_model.go" {
		t.Fatalf("files = %+v", res.Files)
	}
	users := files["users"]
	mustContain(t, "users", users,
		"package models",
		"type User struct",
		`momo:"pk,identity"`,
		`momo:"size:200,unique"`,
		"time.Time",
		`momo:"default:now(),dbdefault"`,
		`momo:"enum:user_role"`,
		"nullable",
		`momo:"precision:10,scale:2"`,
	)
	mustMatch(t, "users", users,
		`ID\s+int64`, `FullName\s+\*string`, `Meta\s+\*types\.JSON\[json\.RawMessage\]`, `Tags\s+\[\]string`,
	)
	orders := files["orders"]
	mustMatch(t, "orders", orders, `User\s+\*User`)
	mustContain(t, "orders", orders,
		"type Order struct",
		`momo:"pk,serial"`,
		"UserID",
		`momo:"belongs_to,fk:UserID,on_delete:cascade"`,
		`type:timestamp without time zone`,
		`c.Default(&m.Note, "'a,b'::text")`,
		`c.Index("orders_user_placed_idx", &m.UserID, &m.PlacedAt).Desc(&m.PlacedAt)`,
		`c.Check("orders_total_check", "total > 0")`,
		`c.Enum("user_role", "admin", "member")`,
	)
	for _, f := range res.Files {
		body, hash, ok := SplitHeader(f.Content)
		if !ok || hash == "" || len(body) == 0 {
			t.Errorf("%s: no header", f.Name)
		}
		if edited, gen := Edited(f.Content); edited || !gen {
			t.Errorf("%s: fresh file reads as edited=%v generated=%v", f.Name, edited, gen)
		}
	}
	var role *schema.Column
	for i := range res.Snapshot.Tables {
		if res.Snapshot.Tables[i].Name == "users" {
			role, _ = res.Snapshot.Tables[i].Column("role")
		}
	}
	if role == nil || role.Type != "user_role" {
		t.Errorf("snapshot enum column = %+v", role)
	}
}

func TestGenerateFromDBUnrepresentable(t *testing.T) {
	s := sampleSchema()
	orders := &s.Tables[0]
	orders.ForeignKeys = append(orders.ForeignKeys, schema.ForeignKey{
		Name: "legacy_fk", Columns: []string{"id", "user_id"}, RefTable: "ghost", RefColumns: []string{"id", "id"},
	})
	orders.Indexes = append(orders.Indexes, schema.Index{
		Name: "orders_lower_idx", Unique: true, Parts: []schema.IndexPart{{Expr: "lower(note)", Desc: true}},
	})
	res, files := generate(t, s, Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	var fk, ix bool
	for _, f := range res.Findings {
		fk = fk || (f.Severity == SevWarn && strings.Contains(f.Object, "legacy_fk"))
		ix = ix || (f.Severity == SevWarn && strings.Contains(f.Object, "orders_lower_idx"))
	}
	if !fk || !ix {
		t.Errorf("want warnings for the foreign key to an unmodeled table and the ordered expression key: %v", res.Findings)
	}
	for _, tb := range res.Snapshot.Tables {
		if tb.Name != "orders" {
			continue
		}
		if len(tb.ForeignKeys) != 1 || len(tb.Indexes) != 1 {
			t.Errorf("snapshot keeps what the models cannot declare: fks %d indexes %d", len(tb.ForeignKeys), len(tb.Indexes))
		}
	}
	if strings.Contains(files["orders"], "legacy_fk") {
		t.Error("the model declares the foreign key it cannot represent")
	}
}

// TestGenerateFromDBCompositeForeignKey covers issue 54: a multi-column key
// into a unique constraint is written as a c.ForeignKey statement and stays in
// the snapshot, so the baseline sees no difference.
func TestGenerateFromDBCompositeForeignKey(t *testing.T) {
	s := &schema.Schema{FormatVersion: schema.FormatVersion, Dialect: dialect.Postgres, Tables: []schema.Table{
		{
			Name:       "document",
			Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "company_id", Type: "bigint"}},
			PrimaryKey: []string{"id"},
			Uniques:    []schema.Unique{{Name: "document_company_id_id_key", Columns: []string{"company_id", "id"}}},
		},
		{
			Name: "note",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint"}, {Name: "company_id", Type: "bigint"}, {Name: "document_id", Type: "bigint"},
			},
			PrimaryKey: []string{"id"},
			ForeignKeys: []schema.ForeignKey{{
				Name: "note_company_doc_fkey", Columns: []string{"company_id", "document_id"},
				RefTable: "document", RefColumns: []string{"company_id", "id"}, OnDelete: "CASCADE",
			}},
		},
	}}
	res, files := generate(t, s, Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	for _, f := range res.Findings {
		if f.Severity == SevWarn {
			t.Errorf("unexpected warning: %v", f)
		}
	}
	mustContain(t, "note", files["note"],
		`c.ForeignKey("note_company_doc_fkey", []any{&m.CompanyID, &m.DocumentID}, "document", []string{"company_id", "id"}).OnDelete("CASCADE")`)
	for _, tb := range res.Snapshot.Tables {
		if tb.Name == "note" && len(tb.ForeignKeys) != 1 {
			t.Errorf("snapshot lost the composite key: %+v", tb.ForeignKeys)
		}
	}
}

// TestGenerateFromDBUniqueExpressionIndex covers issue 55: unique expression
// indexes, alone, mixed with a column and partial, are written as IndexExpr
// statements and stay in the snapshot.
func TestGenerateFromDBUniqueExpressionIndex(t *testing.T) {
	s := sampleSchema()
	users := &s.Tables[1]
	users.Indexes = append(users.Indexes,
		schema.Index{Name: "users_email_lower_key", Unique: true, Parts: []schema.IndexPart{{Expr: "lower((email)::text)"}}},
		schema.Index{Name: "users_role_name_key", Unique: true, Where: "(role IS NOT NULL)",
			Parts: []schema.IndexPart{{Column: "role"}, {Expr: "lower(full_name)"}}},
		schema.Index{Name: "users_name_lower_idx", Parts: []schema.IndexPart{{Expr: "lower(full_name)"}}},
	)
	res, files := generate(t, s, Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	for _, f := range res.Findings {
		if f.Severity == SevWarn {
			t.Errorf("unexpected warning: %v", f)
		}
	}
	mustContain(t, "users", files["users"],
		`c.IndexExpr("users_email_lower_key", "lower((email)::text)").Unique()`,
		`c.IndexExpr("users_role_name_key", "role", "lower(full_name)").Unique().Where("(role IS NOT NULL)")`,
		`c.IndexExpr("users_name_lower_idx", "lower(full_name)")`,
	)
	for _, tb := range res.Snapshot.Tables {
		if tb.Name == "users" && len(tb.Indexes) != 4 {
			t.Errorf("snapshot indexes = %d, want 4", len(tb.Indexes))
		}
	}
}

// TestGenerateFromDBIdentityAlways covers issue 56: a GENERATED ALWAYS column
// is declared identity:always, without a warning, and stays always in the
// snapshot.
func TestGenerateFromDBIdentityAlways(t *testing.T) {
	s := sampleSchema()
	s.Tables[1].Columns[0].Identity = "always"
	res, files := generate(t, s, Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	for _, f := range res.Findings {
		if f.Severity == SevWarn {
			t.Errorf("unexpected warning: %v", f)
		}
	}
	mustContain(t, "users", files["users"], `momo:"pk,identity:always"`)
	for _, tb := range res.Snapshot.Tables {
		if tb.Name == "users" && tb.Columns[0].Identity != "always" {
			t.Errorf("snapshot identity = %q, want always", tb.Columns[0].Identity)
		}
	}
}

func TestGenerateFromDBUnmappedColumn(t *testing.T) {
	s := sampleSchema()
	s.Tables[1].Columns = append(s.Tables[1].Columns, schema.Column{Name: "ip", Type: "inet"})
	res, _ := generate(t, s, Config{})
	if !res.Failed() {
		t.Fatal("an unmapped column type must fail the run")
	}
	cfg := Config{Types: map[string]TypeRule{"inet": {Go: "string"}}}
	res, files := generate(t, s, cfg)
	if res.Failed() {
		t.Fatalf("a types rule must resolve it: %v", res.Findings)
	}
	mustContain(t, "users", files["users"], "IP", `type:inet`)
}

func TestGenerateFromDBRules(t *testing.T) {
	cfg := Config{
		Skip: []string{"orders"},
		Columns: []ColumnRule{
			{Match: "*.created_at", Tags: []string{"created_at", "read_only"}},
			{Match: "users.meta", Go: "types.JSON[Settings]", Name: "Settings"},
		},
		Tables: map[string]TableRule{"users": {Name: "Account", Tags: []string{"soft_delete:timestamp"}}},
	}
	res, files := generate(t, sampleSchema(), cfg)
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	if len(res.Files) != 1 || len(res.Snapshot.Tables) != 1 {
		t.Fatalf("skip: files %d tables %d", len(res.Files), len(res.Snapshot.Tables))
	}
	mustContain(t, "users", files["users"],
		"type Account struct",
		`momo:"table:users,soft_delete:timestamp"`,
		`default:now(),created_at,read_only`,
		"Settings",
		"*types.JSON[Settings]",
		`c.Enum("user_role"`,
	)
}

func TestGenerateFromDBNames(t *testing.T) {
	for _, c := range []struct{ table, want string }{
		{"users", "User"}, {"categories", "Category"}, {"statuses", "Status"},
		{"order_items", "OrderItem"}, {"people", "Person"}, {"Account", "Account"},
	} {
		g := &generator{o: Options{Namer: naming.Momo{}}}
		if got := g.structName(c.table); got != c.want {
			t.Errorf("structName(%q) = %q, want %q", c.table, got, c.want)
		}
	}
	if got := pascal(splitWords("userId"), "X"); got != "UserID" {
		t.Errorf("userId = %q", got)
	}
	if got := pascal(splitWords("HTTPServer_url"), "X"); got != "HTTPServerURL" {
		t.Errorf("HTTPServer_url = %q", got)
	}
	// A name that starts with a letter that has no upper case is not exported
	// by Go, so it gets an X in front.
	for in, want := range map[string]string{"\u65e5\u672c": "X\u65e5\u672c", "\u00df": "X\u00df"} {
		if got := pascal(splitWords(in), "F"); got != want {
			t.Errorf("pascal(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	bad := []Config{
		{Package: "not valid"},
		{Skip: []string{"["}},
		{Types: map[string]TypeRule{"inet": {}}},
		{Columns: []ColumnRule{{Match: "nodot"}}},
		{Columns: []ColumnRule{{Match: "a.b", Go: "*int"}}},
		{Columns: []ColumnRule{{Match: "a.b", Tags: []string{"bogus"}}}},
		{Tables: map[string]TableRule{"t": {Name: "lower"}}},
	}
	for i, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d: want an error", i)
		}
	}
	good := Config{Package: "models", Skip: []string{"_prisma_*"}, Columns: []ColumnRule{{Match: "*.updated_at", Tags: []string{"updated_at"}}}}
	if err := good.Validate(); err != nil {
		t.Errorf("good config: %v", err)
	}
}

func TestPlanApplyKeepsHandEdits(t *testing.T) {
	dir := t.TempDir()
	res, _ := generate(t, sampleSchema(), Config{})
	actions, err := Plan(dir, res.Files, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range actions {
		if a.Kind != ActionCreate {
			t.Fatalf("first run: %+v", a)
		}
	}
	if _, err := Apply(dir, actions, writeFile); err != nil {
		t.Fatal(err)
	}
	if actions, _ = Plan(dir, res.Files, false); len(actions) != 2 || actions[0].Kind != ActionUnchanged || actions[1].Kind != ActionUnchanged {
		t.Fatalf("second run must be unchanged: %+v", actions)
	}

	// Hand edit users, then change both tables in the database.
	usersPath := filepath.Join(dir, "users_model.go")
	b, err := os.ReadFile(usersPath) //nolint:gosec // test path
	if err != nil {
		t.Fatal(err)
	}
	edited := append(b, []byte("\n// hand written\n")...)
	if err := os.WriteFile(usersPath, edited, 0o600); err != nil {
		t.Fatal(err)
	}
	// Hand-edited, but the database did not change: a re-run stays quiet.
	if quiet, err := Plan(dir, res.Files, false); err != nil || kindOf(quiet, "users_model.go") != ActionUnchanged {
		t.Fatalf("an edited file with an unchanged database must not conflict: %+v %v", quiet, err)
	}
	// --overwrite does not change that: it replaces a file the database
	// changed, never a hand edit the database did not call for.
	if quiet, err := Plan(dir, res.Files, true); err != nil || kindOf(quiet, "users_model.go") != ActionUnchanged {
		t.Fatalf("--overwrite must keep an edited file whose table did not change: %+v %v", quiet, err)
	}
	s := sampleSchema()
	s.Tables[0].Columns = append(s.Tables[0].Columns, schema.Column{Name: "shipped_at", Type: "timestamp with time zone", Nullable: true})
	s.Tables[1].Columns = append(s.Tables[1].Columns, schema.Column{Name: "nickname", Type: "text", Nullable: true})
	res2, _ := generate(t, s, Config{})
	actions, err = Plan(dir, res2.Files, false)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, a := range actions {
		kinds[a.File] = a.Kind
	}
	if kinds["orders_model.go"] != ActionUpdate || kinds["users_model.go"] != ActionConflict {
		t.Fatalf("kinds = %v", kinds)
	}
	written, err := Apply(dir, actions, writeFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 2 {
		t.Fatalf("written = %v", written)
	}
	after, _ := os.ReadFile(usersPath) //nolint:gosec // test path
	if string(after) != string(edited) {
		t.Fatal("a file with hand edits was overwritten")
	}
	side, err := os.ReadFile(usersPath + ".new") //nolint:gosec // test path
	if err != nil || !strings.Contains(string(side), "Nickname") {
		t.Fatalf("the regenerated version is missing: %v", err)
	}
	if actions, _ = Plan(dir, res2.Files, true); kindOf(actions, "users_model.go") != ActionUpdate {
		t.Fatalf("--overwrite must replace the edited file: %+v", actions)
	}

	// A table that is gone leaves an orphan, never a deletion.
	res3, _ := generate(t, sampleSchema(), Config{Skip: []string{"orders"}})
	actions, _ = Plan(dir, res3.Files, false)
	if kindOf(actions, "orders_model.go") != ActionOrphan {
		t.Fatalf("want an orphan: %+v", actions)
	}
}

func kindOf(actions []Action, file string) string {
	for _, a := range actions {
		if a.File == file {
			return a.Kind
		}
	}
	return ""
}

func TestPlanRefusesHandWrittenFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "users_model.go"), []byte("package models\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, _ := generate(t, sampleSchema(), Config{})
	actions, err := Plan(dir, res.Files, false)
	if err != nil {
		t.Fatal(err)
	}
	if kindOf(actions, "users_model.go") != ActionConflict {
		t.Fatalf("a file this command did not write must conflict: %+v", actions)
	}
}

func writeFile(path string, content []byte) error {
	return os.WriteFile(path, content, 0o600)
}

func TestGenerateFromDBHostileNames(t *testing.T) {
	s := &schema.Schema{Dialect: dialect.Postgres, Tables: []schema.Table{
		{Name: "User", Columns: []schema.Column{{Name: "id", Type: "bigint"}}, PrimaryKey: []string{"id"}},
		{Name: "user", Columns: []schema.Column{{Name: "id", Type: "bigint"}}, PrimaryKey: []string{"id"}},
		{Name: "select", Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "type", Type: "text"},
			{Name: "1st", Type: "text"},
			{Name: "\u65e5\u672c", Type: "text"},
			{Name: "\u00df", Type: "text"},
			{Name: "model", Type: "text"},
			{Name: "momo_config", Type: "text"},
			{Name: "A", Type: "text"},
			{Name: "a", Type: "text"},
			{Name: "quoted", Type: "text", Default: "'it''s \"x\"'::text", Comment: "line one\nline `two`"},
		}, PrimaryKey: []string{"id"}},
	}}
	res, err := Generate(s, Options{Package: "models"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	if len(res.Files) != 3 {
		t.Fatalf("files: %d", len(res.Files))
	}
	names := map[string]bool{}
	for _, f := range res.Files {
		if names[f.Name] {
			t.Errorf("duplicate file name %s", f.Name)
		}
		names[f.Name] = true
	}
	var sel string
	for _, f := range res.Files {
		if f.Table == "select" {
			sel = string(f.Content)
		}
	}
	mustContain(t, "select", sel, "type Select struct", "Type ", "X1st", "Model2", "MomoConfig2", "A2", `c.Comment(&m.Quoted,`)
	if !strings.Contains(sel, `c.Default(&m.Quoted,`) {
		t.Errorf("a default with quotes must go through the companion:\n%s", sel)
	}

	// Each unrepresentable name gets a schema of its own, so one failing name
	// cannot hide that the other is accepted.
	for name, c := range map[string]struct {
		col  string
		fail bool
	}{"comma": {"a,b", true}, "invalid utf-8": {"bad\xffname", false}} {
		bad := &schema.Schema{Dialect: dialect.Postgres, Tables: []schema.Table{
			{Name: "t", Columns: []schema.Column{{Name: c.col, Type: "text"}}},
		}}
		res, err = Generate(bad, Options{Package: "models"})
		if err != nil || res.Failed() != c.fail {
			t.Errorf("%s in a column name: failed = %v, want %v (%v %v)", name, res.Failed(), c.fail, err, res.Findings)
		}
	}
}

// Catches: dbdefault written for a constant default (a needless RETURNING on
// every insert), for a nullable column (nil would stop meaning NULL) or next
// to a tag that already owns the column.
func TestDBDefaultFits(t *testing.T) {
	cases := []struct {
		name string
		col  schema.Column
		opts []string
		want bool
	}{
		{"function default", schema.Column{Default: "gen_random_uuid()"}, nil, true},
		{"now", schema.Column{Default: "now()"}, nil, true},
		{"current timestamp", schema.Column{Default: "CURRENT_TIMESTAMP"}, nil, true},
		{"expression", schema.Column{Default: "(now() AT TIME ZONE 'utc'::text)"}, nil, true},
		{"literal", schema.Column{Default: "'draft'::text"}, nil, false},
		{"number", schema.Column{Default: "0"}, nil, false},
		{"bool", schema.Column{Default: "false"}, nil, false},
		{"no default", schema.Column{}, nil, false},
		{"nullable", schema.Column{Default: "now()", Nullable: true}, nil, false},
		{"identity", schema.Column{Default: "now()", Identity: "serial"}, nil, false},
		{"read only rule", schema.Column{Default: "now()"}, []string{"default:now()", "read_only"}, false},
		{"sequence default without identity", schema.Column{Default: "nextval('seq_n'::regclass)"}, nil, true},
		{"created_at rule", schema.Column{Default: "now()"}, []string{"created_at"}, false},
	}
	for _, c := range cases {
		if got := dbDefaultFits(dialect.Postgres, &c.col, c.opts); got != c.want {
			t.Errorf("%s: dbDefaultFits = %v, want %v", c.name, got, c.want)
		}
	}
}

// Catches: --from-db tagging a MySQL, MariaDB or SQLite default, where a zero
// value would then fail (MySQL has no RETURNING) or be read back differently.
func TestDBDefaultFitsOnlyOnPostgres(t *testing.T) {
	col := schema.Column{Default: "CURRENT_TIMESTAMP"}
	for _, d := range []dialect.Name{dialect.MySQL, dialect.SQLite} {
		if dbDefaultFits(d, &col, nil) {
			t.Errorf("%s: dbdefault must not be emitted", d)
		}
	}
	if !dbDefaultFits(dialect.Postgres, &col, nil) {
		t.Error("postgres: dbdefault expected")
	}
}
