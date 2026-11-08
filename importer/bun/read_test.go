// SPDX-License-Identifier: Apache-2.0

package bun

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/importer"
)

var modelsPosition = regexp.MustCompile(`^models\.go:\d+:\d+$`)

func TestReadReportsNullzeroWithPosition(t *testing.T) {
	_, rep, err := Read("../../internal/compat/bunfixture")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range rep.Changes {
		if c.Kind == importer.ZeroValueHandling && c.Field == "Balance" && strings.Contains(c.Source, "models.go:") {
			found = true
			if !modelsPosition.MatchString(c.Source) {
				t.Fatalf("source must be models.go:<line>:<col> relative to the directory, got %q", c.Source)
			}
		}
	}
	if !found {
		t.Fatalf("Balance nullzero change with a file position not reported: %s", rep)
	}
}

func TestReadFixtureIsDeterministicAndClean(t *testing.T) {
	_, first, err := Read("../../internal/compat/bunfixture")
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := Read("../../internal/compat/bunfixture")
	if err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() || !reflect.DeepEqual(first.Changes, second.Changes) {
		t.Fatalf("two reads report differently:\n%s\n%s", first, second)
	}
	if first.Has(importer.Unsupported) {
		t.Fatalf("the fixture must import cleanly: %s", first)
	}
	if first.Has(importer.ImplicitSave) {
		t.Fatalf("Bun saves no relation field, so no implicit-save may be reported: %s", first)
	}
}

func TestParseTagMatchesBun(t *testing.T) {
	cases := []struct {
		in   string
		want tag
	}{
		{"", tag{}},
		{"id,pk,autoincrement", tag{name: "id", opts: map[string][]string{"pk": {""}, "autoincrement": {""}}}},
		{"table:users,alias:u", tag{opts: map[string][]string{"table": {"users"}, "alias": {"u"}}}},
		{",pk", tag{opts: map[string][]string{"pk": {""}}}},
		{"pk", tag{name: "pk"}},
		{"type:numeric(10,2),notnull", tag{opts: map[string][]string{"type": {"numeric(10,2)"}, "notnull": {""}}}},
		{`default:"a,b"`, tag{opts: map[string][]string{"default": {"a,b"}}}},
		{"unique:a,unique:b", tag{opts: map[string][]string{"unique": {"a", "b"}}}},
		{"rel:belongs-to,join:org_id=id", tag{opts: map[string][]string{"rel": {"belongs-to"}, "join": {"org_id=id"}}}},
	}
	for _, c := range cases {
		if got := parseTag(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseTag(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

const header = `package m

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/uptrace/bun"
)

var (
	_ = time.Now
	_ sql.NullString
	_ json.RawMessage
)

`

func readSource(t *testing.T, src string) ([]importer.Model, *importer.Report) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "models.go"), []byte(header+src), 0o600); err != nil {
		t.Fatal(err)
	}
	// A test file in the project is never parsed.
	if err := os.WriteFile(filepath.Join(dir, "models_test.go"), []byte("this is not Go"), 0o600); err != nil {
		t.Fatal(err)
	}
	models, rep, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	return models, rep
}

func modelOf(t *testing.T, models []importer.Model, name string) importer.Model {
	t.Helper()
	for _, m := range models {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("model %s not read", name)
	return importer.Model{}
}

func fieldOf(t *testing.T, m importer.Model, name string) importer.Field {
	t.Helper()
	for _, f := range m.Fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("field %s.%s not read; got %+v", m.Name, name, m.Fields)
	return importer.Field{}
}

func hasChange(rep *importer.Report, kind importer.Kind, model, field string) bool {
	for _, c := range rep.Of(kind) {
		if c.Model == model && c.Field == field {
			return true
		}
	}
	return false
}

func TestReadMapsGoTypesLikePgdialect(t *testing.T) {
	models, rep := readSource(t, `
type Thing struct {
	bun.BaseModel `+"`bun:\"table:things,alias:t\"`"+`
	ID     int64     `+"`bun:\"id,pk,autoincrement\"`"+`
	Rank   int32     `+"`bun:\"rank,notnull\"`"+`
	Small  uint16
	Name   string    `+"`bun:\"name,notnull\"`"+`
	Note   *string
	Flag   bool      `+"`bun:\"flag,default:true\"`"+`
	Score  float64
	Real   float32
	Data   []byte
	At     time.Time `+"`bun:\"at,nullzero,notnull,default:current_timestamp\"`"+`
	Opt    sql.NullString
	Raw    json.RawMessage
	Sized  string    `+"`bun:\"sized,type:varchar(80)\"`"+`
	Hidden string    `+"`bun:\"-\"`"+`
	secret string
}
`)
	m := modelOf(t, models, "Thing")
	if m.Table != "things" {
		t.Fatalf("table = %q", m.Table)
	}
	want := map[string][3]string{ // name -> column, Go type, SQL type
		"ID":    {"id", "int64", "bigint"},
		"Rank":  {"rank", "int32", "integer"},
		"Small": {"small", "uint16", "smallint"},
		"Name":  {"name", "string", "varchar"},
		"Note":  {"note", "*string", "varchar"},
		"Flag":  {"flag", "bool", "boolean"},
		"Score": {"score", "float64", "double precision"},
		"Real":  {"real", "float32", "real"},
		"Data":  {"data", "[]byte", "bytea"},
		"At":    {"at", "time.Time", "timestamptz"},
		"Opt":   {"opt", "sql.NullString", "varchar"},
		"Raw":   {"raw", "json.RawMessage", "jsonb"},
		"Sized": {"sized", "string", "varchar(80)"},
	}
	if len(m.Fields) != len(want) {
		t.Fatalf("read %d fields, want %d: %+v", len(m.Fields), len(want), m.Fields)
	}
	for name, w := range want {
		f := fieldOf(t, m, name)
		if got := [3]string{f.Column, f.GoType, f.SQLType}; got != w {
			t.Errorf("%s = %v, want %v", name, got, w)
		}
	}
	// Bun makes a column nullable unless notnull, pk or autoincrement says otherwise.
	for name, nullable := range map[string]bool{"ID": false, "Rank": false, "Small": true, "Note": true, "Flag": true, "At": false} {
		if got := fieldOf(t, m, name).Nullable; got != nullable {
			t.Errorf("%s nullable = %v, want %v", name, got, nullable)
		}
	}
	if f := fieldOf(t, m, "At"); f.Default != "current_timestamp" || f.DefaultKind != "db" {
		t.Errorf("At default = %q (%s)", f.Default, f.DefaultKind)
	}
	if !hasChange(rep, importer.DroppedField, "Thing", "Hidden") {
		t.Errorf(`bun:"-" not reported: %s`, rep)
	}
	if !hasChange(rep, importer.ZeroValueHandling, "Thing", "At") {
		t.Errorf("nullzero on a value type not reported: %s", rep)
	}
	if hasChange(rep, importer.ZeroValueHandling, "Thing", "Note") || rep.Has(importer.Unsupported) {
		t.Errorf("unexpected report: %s", rep)
	}
}

func TestReadSerialFollowsPgdialectCaseRules(t *testing.T) {
	models, rep := readSource(t, `
type Seq struct {
	bun.BaseModel `+"`bun:\"table:seqs\"`"+`
	ID    int32  `+"`bun:\"id,pk,autoincrement\"`"+`
	Lower int64  `+"`bun:\"lower,autoincrement,type:bigint\"`"+`
	Upper int64  `+"`bun:\"upper,autoincrement,type:BIGINT\"`"+`
	Text  string `+"`bun:\"text,autoincrement\"`"+`
}
`)
	m := modelOf(t, models, "Seq")
	if f := fieldOf(t, m, "ID"); !f.Serial || f.SQLType != "integer" || !f.PK {
		t.Errorf("ID = %+v", f)
	}
	if f := fieldOf(t, m, "Upper"); !f.Serial || f.SQLType != "bigint" {
		t.Errorf("Upper = %+v", f)
	}
	for _, name := range []string{"Lower", "Text"} {
		if f := fieldOf(t, m, name); f.Serial {
			t.Errorf("%s must not be serial under pgdialect v1.3.0: %+v", name, f)
		}
		if !hasChange(rep, importer.Naming, "Seq", name) {
			t.Errorf("%s: the ineffective autoincrement is not reported: %s", name, rep)
		}
	}
}

func TestReadDefaultTableNames(t *testing.T) {
	models, _ := readSource(t, `
type UserAccount struct {
	bun.BaseModel
	ID int64 `+"`bun:\",pk\"`"+`
}

type Person struct {
	bun.BaseModel `+"`bun:\"people_tbl,alias:p\"`"+`
	ID int64 `+"`bun:\",pk\"`"+`
}

type Plain struct {
	ID int64 `+"`bun:\",pk\"`"+`
}
`)
	if len(models) != 2 {
		t.Fatalf("a struct without bun.BaseModel is not a model; got %d models", len(models))
	}
	if got := modelOf(t, models, "UserAccount").Table; got != "user_accounts" {
		t.Errorf("UserAccount table = %q", got)
	}
	if got := modelOf(t, models, "Person").Table; got != "people_tbl" {
		t.Errorf("Person table = %q (a bare BaseModel tag name is the table)", got)
	}
	if f := fieldOf(t, modelOf(t, models, "UserAccount"), "ID"); f.Column != "id" || f.Serial {
		t.Errorf("a plain pk is not serial in Bun: %+v", f)
	}
}

func TestReadUniqueConstraints(t *testing.T) {
	models, rep := readSource(t, `
type U struct {
	bun.BaseModel `+"`bun:\"table:us\"`"+`
	ID    int64  `+"`bun:\"id,pk\"`"+`
	Email string `+"`bun:\"email,notnull,unique\"`"+`
	Slug  string `+"`bun:\"slug,unique:uq_slug\"`"+`
	A     string `+"`bun:\"a,unique:grp\"`"+`
	B     string `+"`bun:\"b,unique:grp\"`"+`
	Twice string `+"`bun:\"twice,unique,unique:other\"`"+`
}
`)
	m := modelOf(t, models, "U")
	if f := fieldOf(t, m, "Email"); !f.Unique || f.UniqueName != "us_email_key" {
		t.Errorf("Email = %+v", f)
	}
	if f := fieldOf(t, m, "Slug"); !f.Unique || f.UniqueName != "uq_slug" {
		t.Errorf("Slug = %+v", f)
	}
	for _, name := range []string{"A", "B"} {
		if fieldOf(t, m, name).Unique {
			t.Errorf("%s is in a composite unique constraint and must not get a single-column one", name)
		}
	}
	if !hasChange(rep, importer.DroppedIndex, "U", "A") {
		t.Errorf("composite unique constraint not reported: %s", rep)
	}
	if f := fieldOf(t, m, "Twice"); !f.Unique {
		t.Errorf("Twice = %+v", f)
	}
	if !hasChange(rep, importer.DroppedIndex, "U", "Twice") {
		t.Errorf("second unique constraint on one field not reported: %s", rep)
	}
}

func TestReadEmbeddedStructs(t *testing.T) {
	models, rep := readSource(t, `
type Audit struct {
	CreatedBy string `+"`bun:\"created_by\"`"+`
}

type Stamp struct {
	Stamped time.Time
	Name    string
}

type E struct {
	bun.BaseModel `+"`bun:\"table:es\"`"+`
	ID    int64 `+"`bun:\"id,pk\"`"+`
	Audit Audit `+"`bun:\"embed:audit_\"`"+`
	Stamp
	Name string
}
`)
	m := modelOf(t, models, "E")
	var cols []string
	for _, f := range m.Fields {
		cols = append(cols, f.Column)
	}
	// The top-level Name wins over the embedded one; the embedded one is reported.
	if want := "id,audit_created_by,stamped,name"; strings.Join(cols, ",") != want {
		t.Errorf("columns = %v, want %s", cols, want)
	}
	if !hasChange(rep, importer.DroppedField, "E", "Name") {
		t.Errorf("the shadowed embedded field is not reported: %s", rep)
	}
	if rep.Has(importer.Unsupported) {
		t.Errorf("unexpected: %s", rep)
	}
}

func TestReadBelongsTo(t *testing.T) {
	models, rep := readSource(t, `
type Org struct {
	bun.BaseModel `+"`bun:\"table:orgs\"`"+`
	ID       int64      `+"`bun:\"id,pk,autoincrement\"`"+`
	Accounts []*Account `+"`bun:\"rel:has-many,join:id=org_id\"`"+`
	Tags     []*Tag     `+"`bun:\"m2m:org_tags,join:Org=Tag\"`"+`
}

type Account struct {
	bun.BaseModel `+"`bun:\"table:accounts\"`"+`
	ID    int64 `+"`bun:\"id,pk,autoincrement\"`"+`
	OrgID int64 `+"`bun:\"org_id,notnull\"`"+`
	Org   *Org  `+"`bun:\"rel:belongs-to,join:org_id=id,on_delete:cascade,on_update:set null\"`"+`
	Boss  *Org  `+"`bun:\"rel:belongs-to\"`"+`
	BossID int64 `+"`bun:\"boss_id\"`"+`
}

type Tag struct {
	bun.BaseModel `+"`bun:\"table:tags\"`"+`
	ID int64 `+"`bun:\"id,pk\"`"+`
}
`)
	acc := modelOf(t, models, "Account")
	if len(acc.ForeignKeys) != 2 {
		t.Fatalf("foreign keys = %+v\n%s", acc.ForeignKeys, rep)
	}
	byField := map[string]importer.ForeignKey{}
	for _, fk := range acc.ForeignKeys {
		byField[fk.Field] = fk
	}
	org := byField["Org"]
	if org.RefModel != "Org" || org.FKField != "OrgID" || org.OnDelete != "CASCADE" || org.OnUpdate != "SET NULL" ||
		org.RefTable != "orgs" || !reflect.DeepEqual(org.Columns, []string{"org_id"}) || !reflect.DeepEqual(org.RefColumns, []string{"id"}) {
		t.Errorf("Org key = %+v", org)
	}
	// Without join: Bun looks for <field>_<pk> (boss_id), found here.
	if boss := byField["Boss"]; boss.FKField != "BossID" || boss.OnDelete != "" {
		t.Errorf("Boss key = %+v", boss)
	}
	if !hasChange(rep, importer.Naming, "Account", "Org") {
		t.Errorf("the WithForeignKeys caveat is not reported: %s", rep)
	}
	for _, f := range []string{"Accounts", "Tags"} {
		if !hasChange(rep, importer.DroppedField, "Org", f) {
			t.Errorf("relation field %s is not reported as not imported: %s", f, rep)
		}
	}
	if rep.Has(importer.ImplicitSave) || rep.Has(importer.Unsupported) {
		t.Errorf("unexpected report: %s", rep)
	}
}

func TestReadRejectsWhatItCannotConvert(t *testing.T) {
	cases := []struct {
		name, src, model, field string
	}{
		{"array option", "type X struct {\n\tbun.BaseModel\n\tID int64 `bun:\",pk\"`\n\tTags []string `bun:\",array\"`\n}\n", "X", "Tags"},
		{"unknown option", "type X struct {\n\tbun.BaseModel\n\tID int64 `bun:\",pk\"`\n\tA string `bun:\"a,nullzro\"`\n}\n", "X", "A"},
		{"pointer notnull", "type X struct {\n\tbun.BaseModel\n\tID int64 `bun:\",pk\"`\n\tA *string `bun:\"a,notnull\"`\n}\n", "X", "A"},
		{"struct column", "type Meta struct{ K string }\ntype X struct {\n\tbun.BaseModel\n\tID int64 `bun:\",pk\"`\n\tM Meta\n}\n", "X", "M"},
		{"slice column", "type X struct {\n\tbun.BaseModel\n\tID int64 `bun:\",pk\"`\n\tM []string\n}\n", "X", "M"},
		{"foreign package", "type X struct {\n\tbun.BaseModel\n\tID int64 `bun:\",pk\"`\n\tU uuid.UUID\n}\n", "X", "U"},
		{"nullint32", "type X struct {\n\tbun.BaseModel\n\tID int64 `bun:\",pk\"`\n\tN sql.NullInt32\n}\n", "X", "N"},
		{"custom Value", "type Money int64\nfunc (m Money) Value() (any, error) { return int64(m), nil }\ntype X struct {\n\tbun.BaseModel\n\tID int64 `bun:\",pk\"`\n\tP Money\n}\n", "X", "P"},
		{"comma in default", "type X struct {\n\tbun.BaseModel\n\tID int64 `bun:\",pk\"`\n\tP int64 `bun:\"p,default:(coalesce(1,2))\"`\n}\n", "X", "P"},
		{"unknown join", "type Y struct {\n\tbun.BaseModel\n\tID int64 `bun:\",pk\"`\n}\ntype X struct {\n\tbun.BaseModel\n\tID int64 `bun:\",pk\"`\n\tY *Y `bun:\"rel:belongs-to,join:nope=id\"`\n}\n", "X", "Y"},
		{"bad action", "type Y struct {\n\tbun.BaseModel\n\tID int64 `bun:\",pk\"`\n}\ntype X struct {\n\tbun.BaseModel\n\tID int64 `bun:\",pk\"`\n\tYID int64 `bun:\"y_id\"`\n\tY *Y `bun:\"rel:belongs-to,on_delete:explode\"`\n}\n", "X", "Y"},
		{"missing target", "type X struct {\n\tbun.BaseModel\n\tID int64 `bun:\",pk\"`\n\tYID int64\n\tY *Gone `bun:\"rel:belongs-to\"`\n}\n", "X", "Y"},
		{"unknown relation", "type X struct {\n\tbun.BaseModel\n\tID int64 `bun:\",pk\"`\n\tY *X `bun:\"rel:has-some\"`\n}\n", "X", "Y"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, rep := readSource(t, c.src)
			if !hasChange(rep, importer.Unsupported, c.model, c.field) {
				t.Fatalf("expected an unsupported change for %s.%s: %s", c.model, c.field, rep)
			}
		})
	}
}

func TestReadReportsHooksAndOptions(t *testing.T) {
	_, rep := readSource(t, `
type H struct {
	bun.BaseModel `+"`bun:\"table:hs\"`"+`
	ID   int64     `+"`bun:\"id,pk\"`"+`
	Del  time.Time `+"`bun:\",soft_delete,nullzero\"`"+`
	Ro   string    `+"`bun:\"ro,scanonly\"`"+`
	Skip string    `+"`bun:\"skip,skipupdate\"`"+`
	Kind Kind
	Kptr *Kind
	Null bun.NullTime
}

type Kind string

func (h *H) BeforeAppendModel() error { return nil }
func (h *H) AfterCreateTable() error  { return nil }
`)
	if !hasChange(rep, importer.GoDefault, "H", "") {
		t.Errorf("BeforeAppendModel not reported: %s", rep)
	}
	if !hasChange(rep, importer.Unsupported, "H", "") {
		t.Errorf("AfterCreateTable not reported: %s", rep)
	}
	for _, f := range []string{"Del", "Kind", "Kptr", "Null"} {
		if !hasChange(rep, importer.RenamedType, "H", f) {
			t.Errorf("renamed-type for %s not reported: %s", f, rep)
		}
	}
	for _, f := range []string{"Ro", "Skip"} {
		if !hasChange(rep, importer.DroppedField, "H", f) {
			t.Errorf("dropped-field for %s not reported: %s", f, rep)
		}
	}
}

func TestReadNamedTypesAndAliases(t *testing.T) {
	dir := t.TempDir()
	src := `package m

import (
	stdsql "database/sql"
	orm "github.com/uptrace/bun"
	"time"
)

type Status string

type N struct {
	orm.BaseModel ` + "`bun:\"table:ns\"`" + `
	ID     int64 ` + "`bun:\",pk\"`" + `
	State  Status
	Maybe  stdsql.NullInt64
	When   *time.Time
}
`
	if err := os.WriteFile(filepath.Join(dir, "n.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	models, rep, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := modelOf(t, models, "N")
	if f := fieldOf(t, m, "State"); f.GoType != "string" || f.SQLType != "varchar" {
		t.Errorf("State = %+v", f)
	}
	if f := fieldOf(t, m, "Maybe"); f.GoType != "sql.NullInt64" || f.SQLType != "bigint" {
		t.Errorf("Maybe = %+v (a renamed import must resolve to the canonical name)", f)
	}
	if f := fieldOf(t, m, "When"); f.GoType != "*time.Time" || f.SQLType != "timestamptz" {
		t.Errorf("When = %+v", f)
	}
	if rep.Has(importer.Unsupported) {
		t.Errorf("unexpected: %s", rep)
	}
}

func TestReadDetectsTableNameInflector(t *testing.T) {
	_, rep := readSource(t, `
func init() { schema.SetTableNameInflector(func(s string) string { return s }) }
`)
	if !rep.Has(importer.Naming) {
		t.Fatalf("SetTableNameInflector not reported: %s", rep)
	}
}

func TestReadMissingDirectory(t *testing.T) {
	if _, _, err := Read(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("a missing directory must be an error")
	}
}
