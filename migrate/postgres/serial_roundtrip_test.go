// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	"github.com/azrtydxb/momo/schema"
)

// serialModelSchema is what the models declare for pk,serial and serial.
func serialModelSchema() *schema.Schema {
	return &schema.Schema{FormatVersion: schema.FormatVersion, Dialect: "postgres", Tables: []schema.Table{{
		Name: "serial_rows",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint", Identity: "serial"},
			{Name: "seq", Type: "integer", Identity: "serial"},
			{Name: "name", Type: "text"},
		},
		PrimaryKey: []string{"id"},
	}}}
}

// TestSerialIntrospectionMatchesModel adopts a database that was created with
// BIGSERIAL by hand: the introspected schema must equal the model, so diff
// proposes nothing.
func TestSerialIntrospectionMatchesModel(t *testing.T) {
	db := testopen.Postgres(t)
	mustExec(t, db.Driver(), `CREATE TABLE serial_rows (id bigserial PRIMARY KEY, seq serial, name text NOT NULL)`)
	got := inspectNow(t, db)
	want := serialModelSchema()
	want.Extensions = got.Extensions // database-wide; testdb installs vector
	if !schema.Equal(got, want) {
		t.Fatalf("introspected schema differs from the model:\n%s", migratetest.Diff(got, want))
	}
	changes, _, err := migrate.Diff(got, want, migrate.DiffOptions{})
	if err != nil || len(changes) != 0 {
		t.Fatalf("diff must be empty: %v %+v", err, changes)
	}
}

// TestSerialRenderedDDLRoundTrips applies the DDL momo renders for the model
// and reads it back: serial columns survive apply then inspect without drift.
func TestSerialRenderedDDLRoundTrips(t *testing.T) {
	db := testopen.Postgres(t)
	want := serialModelSchema()
	stmts := planStatements(t, &schema.Schema{Dialect: "postgres"}, want, migrate.DiffOptions{})
	applyStatements(t, db, stmts)
	got := inspectNow(t, db)
	want.Extensions = got.Extensions
	if !schema.Equal(got, want) {
		t.Fatalf("round trip has a diff:\n%s", migratetest.Diff(got, want))
	}
	var owned int
	queryOne(t, db, `SELECT count(*) FROM pg_depend d JOIN pg_class s ON s.oid = d.objid AND s.relkind = 'S' WHERE d.deptype = 'a' AND d.refobjid = 'serial_rows'::regclass`, &owned)
	if owned != 2 {
		t.Fatalf("serial_rows owns %d sequences, want 2", owned)
	}
}

type serialRow struct {
	ID   int64
	Seq  int32
	Name string
}

var serialRowMeta = momo.Meta[serialRow]{
	Table: "serial_rows",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity},
		{Name: "seq", Flags: momo.FlagIdentity},
		{Name: "name"},
	},
	Ptr:        func(m *serialRow, i int) any { return [...]any{&m.ID, &m.Seq, &m.Name}[i] },
	Value:      func(m *serialRow, i int) any { return [...]any{m.ID, m.Seq, m.Name}[i] },
	Identity:   0,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (*serialRow) MomoMeta() *momo.Meta[serialRow] { return &serialRowMeta }

// TestSerialInsertReturnsGeneratedKey checks that a serial id is omitted from
// the INSERT and read back through RETURNING like an identity id.
func TestSerialInsertReturnsGeneratedKey(t *testing.T) {
	db := testopen.Postgres(t)
	stmts := planStatements(t, &schema.Schema{Dialect: "postgres"}, serialModelSchema(), migrate.DiffOptions{})
	applyStatements(t, db, stmts)
	ctx := context.Background()

	for want := int64(1); want <= 2; want++ {
		row := serialRow{Name: "n"}
		rows, err := momo.Insert[serialRow](db).Model(&row).ReturningAll().Rows(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !rows.Next() {
			t.Fatalf("no RETURNING row: %v", rows.Err())
		}
		var got serialRow
		if err := rows.ScanModel(&got); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		if got.ID != want || got.Seq != int32(want) || got.Name != "n" {
			t.Fatalf("returned %+v, want id and seq %d", got, want)
		}
	}
}
