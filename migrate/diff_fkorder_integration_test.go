// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// licenceSchema is the two models of the report: Licence with a unique
// licence_id (a unique index, as the unique tag makes it) and LicenceUsage
// with a foreign key to Licence(licence_id). withKey leaves the unique index
// out; withUsage leaves the referencing table out.
func licenceSchema(d dialect.Name, withKey, withUsage bool) *schema.Schema {
	keyType := "text"
	switch d {
	case dialect.SQLite:
		keyType = "TEXT"
	case dialect.MySQL:
		keyType = "varchar(40)"
	}
	var extensions []schema.Extension
	if d == dialect.Postgres {
		extensions = []schema.Extension{{Name: "vector"}} // installed in the test database; inspection reports it
	}
	cols := []schema.Column{{Name: "id", Type: keyType}, {Name: "licence_id", Type: keyType}}
	licence := schema.Table{Name: "licence", Columns: cols, PrimaryKey: []string{"id"}}
	if withKey {
		licence.Indexes = []schema.Index{{Name: "licence_licence_id_key", Unique: true, Parts: []schema.IndexPart{{Column: "licence_id"}}}}
	}
	tables := []schema.Table{licence}
	if withUsage {
		tables = append(tables, schema.Table{
			Name: "licence_usage", Columns: cols, PrimaryKey: []string{"id"},
			ForeignKeys: []schema.ForeignKey{{
				Name: "licence_usage_licence_id_fkey", Columns: []string{"licence_id"},
				RefTable: "licence", RefColumns: []string{"licence_id"}, OnDelete: "CASCADE", OnUpdate: "CASCADE",
			}},
		})
	}
	return &schema.Schema{Dialect: d, Extensions: extensions, Tables: tables}
}

func licenceRun(t *testing.T, db *momo.DB, d dialect.Name) *checkInRun {
	t.Helper()
	return &checkInRun{t: t, db: db, d: d, files: map[string][]byte{}, at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

// mustHoldFK checks that the foreign key is in force: a usage row for a known
// licence is accepted. A row for an unknown licence is rejected where the
// database enforces foreign keys by default.
func mustHoldFK(t *testing.T, r *checkInRun, d dialect.Name) {
	t.Helper()
	if err := r.exec("INSERT INTO licence (id, licence_id) VALUES ('1', 'L-1')"); err != nil {
		t.Fatal(err)
	}
	if err := r.exec("INSERT INTO licence_usage (id, licence_id) VALUES ('u1', 'L-1')"); err != nil {
		t.Fatalf("a usage of a known licence must be accepted: %v", err)
	}
	if d == dialect.SQLite {
		return
	}
	if err := r.exec("INSERT INTO licence_usage (id, licence_id) VALUES ('u2', 'nope')"); err == nil {
		t.Fatal("a usage of an unknown licence must be rejected by the foreign key")
	}
}

// Catches, on PostgreSQL, MySQL and MariaDB: a diff that writes a foreign key
// to a unique-indexed column inside CREATE TABLE before CREATE UNIQUE INDEX
// runs, so that the migration fails to apply (SQLSTATE 42830 on PostgreSQL).
func TestMigrateForeignKeyToUniqueIndexOfNewTable(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		r := licenceRun(t, db, d)
		r.migrate(licenceSchema(d, true, true), "init", migrate.DiffOptions{})
		mustHoldFK(t, r, d)
	})
}

// Catches: the same failure when the unique index is added to an existing
// table in the migration that also creates the table referencing it.
func TestMigrateForeignKeyToNewUniqueIndexOfExistingTable(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		r := licenceRun(t, db, d)
		r.migrate(licenceSchema(d, false, false), "init", migrate.DiffOptions{})
		r.migrate(licenceSchema(d, true, true), "add_usage", migrate.DiffOptions{})
		mustHoldFK(t, r, d)
	})
}
