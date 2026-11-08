// SPDX-License-Identifier: Apache-2.0

package gorm

import (
	"os"
	"path/filepath"
	"regexp"
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
	if len(models) != 2 || models[0].Name != "Org" || models[1].Name != "User" {
		t.Fatalf("want Org and User in source order, got %+v", models)
	}
	if rep.Has(importer.Unsupported) {
		t.Fatalf("the fixture must import cleanly: %s", rep)
	}
	org, user := models[0], models[1]
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
	Stamp string `+"`gorm:\"default:now\"`"+`
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
