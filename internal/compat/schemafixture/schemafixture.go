// SPDX-License-Identifier: Apache-2.0

// Package schemafixture holds the helpers the importer fixture modules share:
// reading the schema a source ORM created, and building the schema that
// imported models describe. The fixtures compare the two with migrate.Diff.
package schemafixture

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
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

// ImportedSchema builds the PostgreSQL schema the imported models describe:
// the columns, keys, unique constraints, indexes and foreign keys the emitter
// writes into the model tags and relation fields, with the names the source
// database used.
func ImportedSchema(models []importer.Model) *schema.Schema {
	s := &schema.Schema{FormatVersion: schema.FormatVersion, Dialect: dialect.Postgres}
	for _, m := range models {
		t := schema.Table{Name: m.Table}
		for _, f := range m.Fields {
			c := schema.Column{Name: f.Column, Type: f.SQLType, Nullable: f.Nullable}
			if f.DefaultKind != "go" {
				c.Default = f.Default
			}
			if f.Serial {
				c.Identity = "serial"
			}
			t.Columns = append(t.Columns, c)
			if f.PK {
				t.PrimaryKey = append(t.PrimaryKey, f.Column)
			}
			part := []schema.IndexPart{{Column: f.Column}}
			switch {
			case f.UniqueIndex:
				t.Indexes = append(t.Indexes, schema.Index{Name: f.UniqueName, Unique: true, Parts: part})
			case f.Unique:
				name := f.UniqueName
				if name == "" {
					name = m.Table + "_" + f.Column + "_key"
				}
				t.Uniques = append(t.Uniques, schema.Unique{Name: name, Columns: []string{f.Column}})
			}
			if f.Index {
				t.Indexes = append(t.Indexes, schema.Index{Name: f.IndexName, Parts: part})
			}
		}
		for _, fk := range m.ForeignKeys {
			t.ForeignKeys = append(t.ForeignKeys, schema.ForeignKey{
				Name: fk.Name, Columns: fk.Columns, RefTable: fk.RefTable, RefColumns: fk.RefColumns,
				OnDelete: fk.OnDelete, OnUpdate: fk.OnUpdate,
			})
		}
		s.Tables = append(s.Tables, t)
	}
	s.Sort()
	return s
}
