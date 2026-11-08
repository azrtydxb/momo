// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// pubRow has a database-generated integer key and a Go-side generated public id.
type pubRow struct {
	ID       int64
	PublicID string
	Name     string
}

var pubRowMeta = momo.Meta[pubRow]{
	Table: "pub_rows",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity},
		{Name: "public_id"},
		{Name: "name"},
	},
	Ptr:      func(m *pubRow, i int) any { return [...]any{&m.ID, &m.PublicID, &m.Name}[i] },
	Value:    func(m *pubRow, i int) any { return [...]any{m.ID, m.PublicID, m.Name}[i] },
	Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK:     []int{0},
	IDGens: []momo.IDGen{{Column: 1, Name: "uuid"}},
}

func (*pubRow) MomoMeta() *momo.Meta[pubRow] { return &pubRowMeta }

// Catches: LAST_INSERT_ID overwriting a Go-side generated id, or the Go-side id
// generation clobbering the database-generated key; each field has one writer.
func TestMySQLKeysHaveOneWriterEach(t *testing.T) {
	db := momo.New(idDriver{fakedriver.New(dialect.MySQL)})
	m := pubRow{Name: "a"}
	if _, err := momo.Insert[pubRow](db).Model(&m).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	if m.ID != 42 {
		t.Fatalf("database-generated key = %d, want 42 from LAST_INSERT_ID", m.ID)
	}
	if !reUUID.MatchString(m.PublicID) {
		t.Fatalf("Go-side id = %q, want a uuid", m.PublicID)
	}
}

// Catches: an ON DUPLICATE KEY UPDATE that overwrites the stored public id with
// the fresh one generated for the new row.
func TestMySQLUpsertKeepsGeneratedIDOutOfUpdate(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.MySQL))
	m := pubRow{ID: 7, Name: "a"}
	c, err := momo.Upsert[pubRow](db).Model(&m).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.SQL, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("SQL = %s", c.SQL)
	}
	update := c.SQL[strings.Index(c.SQL, "ON DUPLICATE KEY UPDATE"):]
	if strings.Contains(update, "`public_id`") {
		t.Fatalf("the update list assigns the generated id: %s", update)
	}
	if !strings.Contains(update, "`name`") {
		t.Fatalf("the update list lost name: %s", update)
	}
}
