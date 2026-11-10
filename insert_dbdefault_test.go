// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/schema"
)

// dbdRow has a database-generated id and timestamp (dbdefault) next to a
// column the caller always sets.
type dbdRow struct {
	ID    string
	Title string
	At    time.Time
}

var dbdRowMeta = momo.Meta[dbdRow]{
	Table: "dbd_rows",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK | momo.FlagHasDefault | momo.FlagDBDefault},
		{Name: "title"},
		{Name: "at", Flags: momo.FlagHasDefault | momo.FlagDBDefault},
	},
	Ptr:   func(m *dbdRow, i int) any { return [...]any{&m.ID, &m.Title, &m.At}[i] },
	Value: func(m *dbdRow, i int) any { return [...]any{m.ID, m.Title, m.At}[i] },
	TableSchema: &schema.Table{
		Name: "dbd_rows",
		Columns: []schema.Column{
			{Name: "id", Type: "uuid"},
			{Name: "title", Type: "text"},
			{Name: "at", Type: "timestamptz"},
		},
		PrimaryKey: []string{"id"},
	},
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*dbdRow) MomoMeta() *momo.Meta[dbdRow] { return &dbdRowMeta }

var (
	dbdID    = momo.NewColumn[string]("dbd_rows", "id", momo.FlagPK)
	dbdTitle = momo.NewColumn[string]("dbd_rows", "title", 0)
	dbdStamp = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
)

func dbdSetup(name dialect.Name, rows [][]any, opts ...momo.Option) (*momo.DB, *fakedriver.Driver) {
	drv := fakedriver.New(name)
	drv.SetRows(rows)
	return momo.New(drv, opts...), drv
}

func lastSQL(t *testing.T, drv *fakedriver.Driver) fakedriver.Call {
	t.Helper()
	calls := drv.Calls()
	if len(calls) == 0 {
		t.Fatal("no statement reached the driver")
	}
	return calls[len(calls)-1]
}

func TestDBDefaultZeroFieldsAreLeftOutAndReadBack(t *testing.T) {
	db, drv := dbdSetup(dialect.Postgres, [][]any{{"gen-1", dbdStamp}})
	m := dbdRow{Title: "a"}
	if _, err := momo.Insert[dbdRow](db).Model(&m).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := lastSQL(t, drv)
	if want := `INSERT INTO "dbd_rows" ("title") VALUES ($1) RETURNING "id", "at"`; c.SQL != want {
		t.Fatalf("SQL = %s, want %s", c.SQL, want)
	}
	if len(c.Args) != 1 || c.Args[0] != "a" {
		t.Fatalf("args = %v", c.Args)
	}
	if m.ID != "gen-1" || !m.At.Equal(dbdStamp) || m.Title != "a" {
		t.Fatalf("model not read back: %+v", m)
	}
}

func TestDBDefaultSetValueIsWrittenAsGiven(t *testing.T) {
	db, drv := dbdSetup(dialect.Postgres, [][]any{{dbdStamp}})
	m := dbdRow{ID: "mine", Title: "a"}
	if _, err := momo.Insert[dbdRow](db).Model(&m).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := lastSQL(t, drv)
	if want := `INSERT INTO "dbd_rows" ("id", "title") VALUES ($1, $2) RETURNING "at"`; c.SQL != want {
		t.Fatalf("SQL = %s, want %s", c.SQL, want)
	}
	if c.Args[0] != "mine" || m.ID != "mine" || m.At.IsZero() {
		t.Fatalf("args %v, model %+v", c.Args, m)
	}
}

func TestDBDefaultAllSetHasNoReadBack(t *testing.T) {
	db, drv := dbdSetup(dialect.Postgres, nil)
	m := dbdRow{ID: "x", Title: "a", At: dbdStamp}
	if _, err := momo.Insert[dbdRow](db).Model(&m).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := lastSQL(t, drv).SQL, `INSERT INTO "dbd_rows" ("id", "title", "at") VALUES ($1, $2, $3)`; got != want {
		t.Fatalf("SQL = %s, want %s", got, want)
	}
}

func TestDBDefaultOmitWins(t *testing.T) {
	db, drv := dbdSetup(dialect.Postgres, nil)
	m := dbdRow{Title: "a", At: dbdStamp}
	if _, err := momo.Insert[dbdRow](db).Omit(dbdID).Model(&m).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := lastSQL(t, drv).SQL, `INSERT INTO "dbd_rows" ("title", "at") VALUES ($1, $2)`; got != want {
		t.Fatalf("SQL = %s, want %s", got, want)
	}
}

func TestDBDefaultModelsMixedRowsWriteDEFAULT(t *testing.T) {
	db, drv := dbdSetup(dialect.Postgres, [][]any{{"gen-1"}, {"kept"}, {"gen-3"}})
	ms := []dbdRow{{Title: "a", At: dbdStamp}, {ID: "kept", Title: "b", At: dbdStamp}, {Title: "c", At: dbdStamp}}
	if _, err := momo.Insert[dbdRow](db).Models(ms).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := lastSQL(t, drv)
	want := `INSERT INTO "dbd_rows" ("id", "title", "at") VALUES (DEFAULT, $1, $2), ($3, $4, $5), (DEFAULT, $6, $7) RETURNING "id"`
	if c.SQL != want {
		t.Fatalf("SQL = %s, want %s", c.SQL, want)
	}
	if ms[0].ID != "gen-1" || ms[1].ID != "kept" || ms[2].ID != "gen-3" {
		t.Fatalf("ids = %q %q %q", ms[0].ID, ms[1].ID, ms[2].ID)
	}
}

func TestDBDefaultModelsAllZeroLeavesTheColumn(t *testing.T) {
	db, drv := dbdSetup(dialect.Postgres, [][]any{{"g1", dbdStamp}, {"g2", dbdStamp}})
	ms := []dbdRow{{Title: "a"}, {Title: "b"}}
	if _, err := momo.Insert[dbdRow](db).Models(ms).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := lastSQL(t, drv)
	if want := `INSERT INTO "dbd_rows" ("title") VALUES ($1), ($2) RETURNING "id", "at"`; c.SQL != want {
		t.Fatalf("SQL = %s, want %s", c.SQL, want)
	}
	if ms[0].ID != "g1" || ms[1].ID != "g2" {
		t.Fatalf("ids = %q %q", ms[0].ID, ms[1].ID)
	}
}

// A chunked bulk insert writes the same column list in every chunk, with
// DEFAULT in the cells of the rows that leave the column to the database.
func TestDBDefaultChunksShareOneColumnList(t *testing.T) {
	db, drv := dbdSetup(dialect.Postgres, nil, momo.WithMaxParams(6))
	ms := []dbdRow{
		{Title: "a", At: dbdStamp}, {Title: "b", At: dbdStamp}, // chunk 1: both rows leave id to the database
		{ID: "x", Title: "c", At: dbdStamp}, {ID: "y", Title: "d", At: dbdStamp},
	}
	if _, err := momo.Insert[dbdRow](db).Models(ms).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	var inserts []string
	for _, c := range drv.Calls() {
		if strings.HasPrefix(c.SQL, "INSERT") {
			inserts = append(inserts, c.SQL)
		}
	}
	if len(inserts) < 2 {
		t.Fatalf("want a chunked insert, got %v", inserts)
	}
	for _, s := range inserts {
		if !strings.HasPrefix(s, `INSERT INTO "dbd_rows" ("id", "title", "at") VALUES `) {
			t.Fatalf("chunk with a different column list: %s", s)
		}
	}
}

func TestDBDefaultMixedRowsRefusedWhereDEFAULTIsNotAvailable(t *testing.T) {
	ms := []dbdRow{{Title: "a", At: dbdStamp}, {ID: "kept", Title: "b", At: dbdStamp}}

	db, _ := dbdSetup(dialect.SQLite, nil)
	_, err := momo.Insert[dbdRow](db).Models(ms).Exec(context.Background())
	if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), `"id"`) {
		t.Fatalf("SQLite mixed rows: err = %v, want ErrUnsupported naming the column", err)
	}

	pg, _ := dbdSetup(dialect.Postgres, nil)
	if _, err := momo.Insert[dbdRow](pg).Models(ms).Unnest().SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Unnest mixed rows: err = %v, want ErrUnsupported", err)
	}
}

func TestDBDefaultSQLiteUniformRowsReadBack(t *testing.T) {
	db, drv := dbdSetup(dialect.SQLite, [][]any{{"g1", dbdStamp}})
	m := dbdRow{Title: "a"}
	if _, err := momo.Insert[dbdRow](db).Model(&m).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := `INSERT INTO "dbd_rows" ("title") VALUES (?) RETURNING "id", "at"`; lastSQL(t, drv).SQL != want {
		t.Fatalf("SQL = %s, want %s", lastSQL(t, drv).SQL, want)
	}
	if m.ID != "g1" {
		t.Fatalf("model not read back: %+v", m)
	}
}

func TestDBDefaultMySQLRefusesWhatItCannotReadBack(t *testing.T) {
	db, drv := dbdSetup(dialect.MySQL, nil)
	m := dbdRow{Title: "a"}
	_, err := momo.Insert[dbdRow](db).Model(&m).Exec(context.Background())
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("zero dbdefault column on MySQL: err = %v, want ErrUnsupported", err)
	}
	if len(drv.Calls()) != 0 {
		t.Fatalf("a refused insert must not reach the driver: %v", drv.Calls())
	}
	full := dbdRow{ID: "x", Title: "a", At: dbdStamp}
	if _, err := momo.Insert[dbdRow](db).Model(&full).Exec(context.Background()); err != nil {
		t.Fatalf("every dbdefault column set: %v", err)
	}
	omitted := dbdRow{Title: "a", At: dbdStamp}
	if _, err := momo.Insert[dbdRow](db).Omit(dbdID).Model(&omitted).Exec(context.Background()); err != nil {
		t.Fatalf("Omit keeps working on MySQL: %v", err)
	}
}

func TestDBDefaultOwnReturningTurnsReadBackOff(t *testing.T) {
	db, drv := dbdSetup(dialect.Postgres, [][]any{{"gen-1"}})
	m := dbdRow{Title: "a", At: dbdStamp}
	rows, err := momo.Insert[dbdRow](db).Model(&m).Returning(dbdID).Rows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if want := `INSERT INTO "dbd_rows" ("title", "at") VALUES ($1, $2) RETURNING "id"`; lastSQL(t, drv).SQL != want {
		t.Fatalf("SQL = %s, want %s", lastSQL(t, drv).SQL, want)
	}
	if !rows.Next() {
		t.Fatal("no row")
	}
	var got dbdRow
	if err := rows.ScanModel(&got); err != nil || got.ID != "gen-1" {
		t.Fatalf("ScanModel: %v %+v", err, got)
	}
}

func TestDBDefaultUpsertKeepsTheStoredValue(t *testing.T) {
	db, drv := dbdSetup(dialect.Postgres, [][]any{{"gen-1", dbdStamp}})
	m := dbdRow{Title: "a"}
	if _, err := momo.Upsert[dbdRow](db).OnConflict(dbdTitle).DoUpdate().Model(&m).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	sql := lastSQL(t, drv).SQL
	if strings.Contains(sql, `"at" = EXCLUDED`) || !strings.Contains(sql, `"title" = EXCLUDED."title"`) {
		t.Fatalf("a conflict must keep the stored dbdefault value: %s", sql)
	}
	if m.ID != "gen-1" {
		t.Fatalf("model not read back: %+v", m)
	}
}
