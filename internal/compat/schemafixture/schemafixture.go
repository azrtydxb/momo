// SPDX-License-Identifier: Apache-2.0

// Package schemafixture holds the helpers the importer fixture modules share:
// reading the schema a source ORM created, building the schema Momo derives
// from the imported models through the real generator, and comparing the two
// with migrate.Diff.
package schemafixture

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/importer"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// Inspect reads the schema of the database db is connected to (the managed
// schema of its search_path), without the momo history table.
func Inspect(t testing.TB, db *momo.DB) *schema.Schema {
	t.Helper()
	e, err := migrate.NewEngine(db.Driver(), fstest.MapFS{}, migrate.WithDialect(mpg.New()), migrate.WithoutSum())
	if err != nil {
		t.Fatal(err)
	}
	s, err := e.Inspect(context.Background())
	if err != nil {
		t.Fatalf("inspect the source schema: %v", err)
	}
	return s
}

// Emitted is the path a user takes: it writes the imported models as Momo
// source into the package directory pkg under moduleDir (removed when the test
// ends), loads that package with the generator, and returns the schema the
// generator builds from the momo tags. moduleDir must be the directory of a Go
// module that can import github.com/azrtydxb/momo.
func Emitted(t testing.TB, moduleDir, pkg string, models []importer.Model, rep *importer.Report) *schema.Schema {
	t.Helper()
	dir := filepath.Join(moduleDir, pkg)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: moduleDir})
	if err := importer.Emit(models, rep, w, dir, pkg); err != nil {
		t.Fatalf("emit the imported models: %v", err)
	}
	if _, err := w.Commit(); err != nil {
		t.Fatalf("write the imported models: %v", err)
	}
	g, diags, err := gen.Load([]string{"./" + pkg}, gen.Config{Dir: moduleDir, Dialects: []dialect.Name{dialect.Postgres}})
	if err != nil {
		t.Fatalf("load the imported models: %v", err)
	}
	if len(diags) > 0 {
		var sb strings.Builder
		for _, d := range diags {
			fmt.Fprintf(&sb, "\n  %s", d.String())
		}
		t.Fatalf("the generator rejects the imported models:%s", sb.String())
	}
	return g.Schema
}

// AsMomoWould rewrites the two things in a source schema that Momo shapes
// differently on purpose, so the rest can be compared exactly: a foreign key
// is named <table>_<column>_fkey, and a plain UNIQUE column becomes a unique
// index of the same name. The importer reports the renames as naming changes.
// It returns a copy.
func AsMomoWould(t testing.TB, s *schema.Schema) *schema.Schema {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	out, err := schema.Load(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	for i := range out.Tables {
		tb := &out.Tables[i]
		for j := range tb.ForeignKeys {
			fk := &tb.ForeignKeys[j]
			if len(fk.Columns) == 1 {
				fk.Name = tb.Name + "_" + fk.Columns[0] + "_fkey"
			}
		}
		for _, u := range tb.Uniques {
			parts := make([]schema.IndexPart, len(u.Columns))
			for k, c := range u.Columns {
				parts[k] = schema.IndexPart{Column: c}
			}
			tb.Indexes = append(tb.Indexes, schema.Index{Name: u.Name, Unique: true, Parts: parts})
		}
		tb.Uniques = nil
	}
	out.Sort()
	return out
}
