// SPDX-License-Identifier: Apache-2.0

package gorm

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/importer"
)

var modelsPosition = regexp.MustCompile(`^models\.go:\d+:\d+$`)

func TestReadReportsImplicitSaveWithPosition(t *testing.T) {
	_, rep, err := Read("../../internal/compat/gormfixture")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range rep.Changes {
		if c.Kind != "implicit-save" || c.Model != "User" || c.Field != "Org" {
			continue
		}
		found = true
		if !modelsPosition.MatchString(c.Source) {
			t.Fatalf("source must be models.go:<line>:<col> relative to the directory, got %q", c.Source)
		}
	}
	if !found {
		t.Fatalf("implicit-save for User.Org not reported: %s", rep)
	}
}

func readSource(t *testing.T, src string) ([]importer.Model, *importer.Report) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "models.go"), []byte(src), 0o600); err != nil {
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

func field(t *testing.T, m importer.Model, name string) importer.Field {
	t.Helper()
	for _, f := range m.Fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("model %s has no field %s: %+v", m.Name, name, m.Fields)
	return importer.Field{}
}

func TestReadFixtureModels(t *testing.T) {
	models, rep, err := Read("../../internal/compat/gormfixture")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, m := range models {
		names = append(names, m.Name)
	}
	if strings.Join(names, " ") != "Org User Post Comment Sized Folder File" {
		t.Fatalf("want the fixture models in source order, got %v", names)
	}
	if rep.Has(importer.Unsupported) {
		t.Fatalf("the fixture must import cleanly: %s", rep)
	}
	org, user, post, comment, sized, file := models[0], models[1], models[2], models[3], models[4], models[6]
	if f := field(t, org, "Slug"); !f.Unique || f.UniqueIndex || f.UniqueName != "uni_orgs_slug" {
		t.Fatalf("a plain unique stays a constraint: %+v", f)
	}
	if f := field(t, user, "Score"); f.SQLType != "bigint" {
		t.Fatalf("Score: %+v", f)
	}
	for name, want := range map[string]string{"Rank": "integer", "Level": "integer", "Flag": "smallint", "Alt": "integer"} {
		if f := field(t, sized, name); f.SQLType != want {
			t.Fatalf("%s is %s, gorm.io/driver/postgres makes it %s", name, f.SQLType, want)
		}
	}
	if len(file.ForeignKeys) != 1 || file.ForeignKeys[0].OnDelete != "SET NULL" || file.ForeignKeys[0].Name != "fk_folders_files" {
		t.Fatalf("constraint:OnDelete:SET NULL: %+v", file.ForeignKeys)
	}
	if f := field(t, post, "CreatedBy"); f.Column != "audit_created_by" {
		t.Fatalf("embedded with a prefix on a named field: %+v", f)
	}
	if len(comment.ForeignKeys) != 1 || comment.ForeignKeys[0].Name != "fk_posts_comments" || comment.ForeignKeys[0].Field != "Post" || comment.ForeignKeys[0].FKField != "PostID" {
		t.Fatalf("has-many puts the key on the child: %+v", comment.ForeignKeys)
	}
	if f := field(t, comment, "Kind"); f.Index || f.IndexName != "" {
		t.Fatalf("a composite index is not emitted on its fields: %+v", f)
	}
	if got := rep.Of(importer.DroppedIndex); len(got) != 1 {
		t.Fatalf("one composite index: %s", rep)
	}
	if org.Table != "orgs" || user.Table != "app_users" {
		t.Fatalf("tables: %q, %q", org.Table, user.Table)
	}
	if id := field(t, user, "ID"); !id.PK || !id.Serial || id.SQLType != "bigint" || id.Nullable {
		t.Fatalf("gorm.Model ID: %+v", id)
	}
	if e := field(t, user, "Email"); e.SQLType != "varchar(255)" || e.Nullable || !e.UniqueIndex || e.UniqueName != "idx_user_email" {
		t.Fatalf("Email: %+v", e)
	}
	if a := field(t, user, "Active"); a.Default != "true" || a.DefaultKind != "db" || !a.Nullable {
		t.Fatalf("Active: %+v", a)
	}
	if n := field(t, user, "Note"); n.GoType != "*string" || !n.Nullable || n.SQLType != "text" {
		t.Fatalf("Note: %+v", n)
	}
	if len(user.ForeignKeys) != 1 {
		t.Fatalf("want one foreign key, got %+v", user.ForeignKeys)
	}
	fk := user.ForeignKeys[0]
	if fk.Name != "fk_app_users_org" || fk.RefTable != "orgs" || fk.OnDelete != "CASCADE" || fk.Columns[0] != "org_id" {
		t.Fatalf("foreign key: %+v", fk)
	}
	if got := rep.Of(importer.ZeroValueHandling); len(got) != 2 {
		t.Fatalf("Active and Score have a default and a non-pointer type: %s", rep)
	}
}

func TestReadReportsWhatItCannotImport(t *testing.T) {
	_, rep := readSource(t, `package m

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Tag struct{ ID uint }

type Post struct {
	ID    uint
	Key   uuid.UUID
	Tags  []Tag `+"`gorm:\"many2many:post_tags\"`"+`
	Rank  int   `+"`gorm:\"index:idx_rank;uniqueIndex:idx_rank\"`"+`
	Stamp int `+"`gorm:\"default:now\"`"+`
	Hidden string `+"`gorm:\"-\"`"+`
	_ gorm.Model
}
`)
	if len(rep.Of(importer.Unsupported)) < 2 {
		t.Fatalf("uuid type and many2many must be reported unsupported: %s", rep)
	}
	if !rep.Has(importer.GoDefault) {
		t.Fatalf("an identifier default is a Go-side default: %s", rep)
	}
}

func TestReadFlattensEmbeddedStructsAndKeepsOverrides(t *testing.T) {
	models, _ := readSource(t, `package m

type Audit struct {
	CreatedBy string `+"`gorm:\"column:made_by\"`"+`
}

type Doc struct {
	ID    uint
	Audit `+"`gorm:\"embeddedPrefix:a_\"`"+`
	Title string `+"`gorm:\"column:heading;not null\"`"+`
}
`)
	if len(models) != 1 {
		t.Fatalf("Audit is embedded, not a model: %+v", models)
	}
	if f := field(t, models[0], "CreatedBy"); f.Column != "a_made_by" {
		t.Fatalf("embedded prefix: %+v", f)
	}
	if f := field(t, models[0], "Title"); f.Column != "heading" || f.Nullable {
		t.Fatalf("column override: %+v", f)
	}
}

func TestReadDecidesKeysBeforeColumns(t *testing.T) {
	models, _ := readSource(t, `package m

type A struct {
	ID   uint
	Code string `+"`gorm:\"primaryKey\"`"+`
}

type B struct {
	TenantID uint `+"`gorm:\"primaryKey\"`"+`
	Seq      uint `+"`gorm:\"primaryKey;autoIncrement:true\"`"+`
	Plain    uint `+"`gorm:\"primaryKey\"`"+`
}
`)
	a := models[0]
	if field(t, a, "ID").PK || !field(t, a, "Code").PK || field(t, a, "Code").Serial {
		t.Fatalf("an explicit primaryKey replaces the implicit ID key: %+v", a.Fields)
	}
	b := models[1]
	if field(t, b, "TenantID").Serial || field(t, b, "Plain").Serial || !field(t, b, "Seq").Serial {
		t.Fatalf("a composite key only auto-increments when asked to: %+v", b.Fields)
	}
}

func TestReadReportsIgnoredTagsAndBadNumbers(t *testing.T) {
	_, rep := readSource(t, `package m

type P struct {
	ID     uint
	Skip   string `+"`gorm:\"-\"`"+`
	Off    string `+"`gorm:\"-:migration\"`"+`
	RO     string `+"`gorm:\"<-:false\"`"+`
	Ser    string `+"`gorm:\"serializer:json\"`"+`
	Size   string `+"`gorm:\"size:abc\"`"+`
	Many   string `+"`gorm:\"index:a;index:b\"`"+`
}
`)
	if len(rep.Of(importer.DroppedField)) != 3 {
		t.Fatalf("-, -:migration and <- are each reported: %s", rep)
	}
	if len(rep.Of(importer.Unsupported)) != 2 {
		t.Fatalf("serializer and the bad size are unsupported: %s", rep)
	}
	if len(rep.Of(importer.DroppedIndex)) != 1 {
		t.Fatalf("the second index tag on a field is reported: %s", rep)
	}
}

func TestReadHonoursAnImportAlias(t *testing.T) {
	models, rep := readSource(t, `package m

import g "gorm.io/gorm"

type D struct {
	g.Model
	Gone g.DeletedAt
}
`)
	if got := field(t, models[0], "Gone").GoType; got != "*time.Time" || !rep.Has(importer.RenamedType) {
		t.Fatalf("gorm.DeletedAt behind an alias: %q, %s", got, rep)
	}
}

func TestReadSizesFollowTheDialector(t *testing.T) {
	models, _ := readSource(t, `package m

import "database/sql"

type S struct {
	ID    uint8 `+"`gorm:\"primaryKey\"`"+`
	A     int8
	B     int16
	C     int32
	D     int64
	E     uint16
	F     uint32
	G     uint
	H     sql.NullInt16
	I     sql.NullInt64
	Big   string `+"`gorm:\"size:2000000\"`"+`
	Huge  string `+"`gorm:\"size:20000000\"`"+`
}
`)
	want := map[string]string{"ID": "smallint", "A": "smallint", "B": "smallint", "C": "integer", "D": "bigint", "E": "integer", "F": "bigint", "G": "bigint", "H": "smallint", "I": "bigint", "Big": "varchar(2000000)", "Huge": "text"}
	for name, typ := range want {
		if got := field(t, models[0], name).SQLType; got != typ {
			t.Errorf("%s: %s, want %s", name, got, typ)
		}
	}
}

func TestReadReportsWhatItWouldGuess(t *testing.T) {
	_, rep := readSource(t, `package m

import "gorm.io/gorm"

type T struct {
	ID    uint
	Name  *string `+"`gorm:\"not null\"`"+`
	Memo  string  `+"`gorm:\"comment:hello;check:memo <> ''\"`"+`
	Both  string  `+"`gorm:\"unique;uniqueIndex:idx_both\"`"+`
	Comp  string  `+"`gorm:\"index:idx_c,composite:cx\"`"+`
}

func (T) TableName() string { return prefix() + "ts" }

var cfg = gorm.Config{NamingStrategy: nil}
`)
	if len(rep.Of(importer.Unsupported)) != 4 {
		t.Fatalf("want a non-literal TableName, a pointer with not null, comment and check reported: %s", rep)
	}
	if !rep.Has(importer.Naming) {
		t.Fatalf("a NamingStrategy in the package must be reported: %s", rep)
	}
	if len(rep.Of(importer.DroppedIndex)) != 2 {
		t.Fatalf("unique plus uniqueIndex and the composite option are each reported: %s", rep)
	}
}

func TestReadConstraintActions(t *testing.T) {
	models, rep := readSource(t, `package m

type Folder struct {
	ID    uint
	Files []File `+"`gorm:\"constraint:OnDelete:SET NULL,OnUpdate:CASCADE\"`"+`
}

type File struct {
	ID       uint
	FolderID uint
}

type Note struct {
	ID       uint
	FolderID uint
	Folder   Folder `+"`gorm:\"constraint:OnDelete:CASCADE,Deferrable:true\"`"+`
}
`)
	var file, note importer.Model
	for _, m := range models {
		switch m.Name {
		case "File":
			file = m
		case "Note":
			note = m
		}
	}
	if len(file.ForeignKeys) != 1 || file.ForeignKeys[0].OnDelete != "SET NULL" || file.ForeignKeys[0].OnUpdate != "CASCADE" || file.ForeignKeys[0].Name != "fk_folders_files" {
		t.Fatalf("has-many constraint actions: %+v", file.ForeignKeys)
	}
	if len(note.ForeignKeys) != 1 || note.ForeignKeys[0].OnDelete != "CASCADE" {
		t.Fatalf("belongs-to constraint actions: %+v", note.ForeignKeys)
	}
	if got := rep.Of(importer.Unsupported); len(got) != 1 {
		t.Fatalf("the Deferrable option is not imported and must be reported: %s", rep)
	}
}

// Each of these is a place where gorm's own rules differ from a first guess.
func TestReadFollowsGormTagSemantics(t *testing.T) {
	models, rep := readSource(t, `package m

import (
	"database/sql"
	"time"

	"gorm.io/gorm"
)

type A struct {
	ID    uint `+"`gorm:\"primaryKey;autoIncrement\"`"+`
	Other uint `+"`gorm:\"autoIncrement\"`"+`
	Age   int  `+"`gorm:\"size:16\"`"+`
	Big   uint `+"`gorm:\"size:31\"`"+`
	At    time.Time `+"`gorm:\"precision:3\"`"+`
	NAt   sql.NullTime `+"`gorm:\"precision:6\"`"+`
	NS    sql.NullString `+"`gorm:\"size:40\"`"+`
	NF    sql.NullFloat64 `+"`gorm:\"precision:10;scale:2\"`"+`
	CreatedAt time.Time
	UpdatedAt int64
	Typed uint `+"`gorm:\"primaryKey;autoIncrement;type:bigint\"`"+`
}

type C struct {
	TenantID uint `+"`gorm:\"primaryKey\"`"+`
	Seq      uint `+"`gorm:\"primaryKey;autoIncrement\"`"+`
}

type P struct {
	gorm.Model `+"`gorm:\"embeddedPrefix:p_\"`"+`
}
`)
	a, c, p := models[0], models[1], models[2]
	if f := field(t, a, "Other"); !f.Serial {
		t.Fatalf("a bare autoIncrement switches it on for a non-key field: %+v", f)
	}
	if f := field(t, c, "Seq"); !f.Serial || field(t, c, "TenantID").Serial {
		t.Fatalf("a composite key keeps the serial only where autoIncrement was written: %+v", c.Fields)
	}
	for name, want := range map[string]string{
		"Age": "smallint", "Big": "integer", "At": "timestamptz(3)", "NAt": "timestamptz(6)",
		"NS": "varchar(40)", "NF": "numeric(10,2)",
	} {
		if got := field(t, a, name).SQLType; got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}
	if got := len(rep.Of(importer.GoDefault)); got != 4 {
		t.Fatalf("CreatedAt, UpdatedAt and the two from gorm.Model are set in Go by name: %d\n%s", got, rep)
	}
	if got := len(rep.Of(importer.Naming)); got != 1 {
		t.Fatalf("type: on an auto-increment column is reported: %d\n%s", got, rep)
	}
	if f := field(t, p, "DeletedAt"); f.Column != "p_deleted_at" || f.IndexName != "idx_ps_p_deleted_at" {
		t.Fatalf("embeddedPrefix on gorm.Model: %+v", f)
	}
}

func TestSplitTagHonoursEscapedSemicolons(t *testing.T) {
	got := splitTag(`default:a\;b;not null`)
	if len(got) != 2 || got[0] != "default:a;b" || got[1] != "not null" {
		t.Fatalf("splitTag = %q", got)
	}
}
