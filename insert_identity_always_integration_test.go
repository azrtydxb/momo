// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// alwaysRow is a model whose id column is GENERATED ALWAYS AS IDENTITY, which
// is what the identity:always tag declares: the generated metadata marks it
// FlagIdentity, exactly as for a by-default identity.
type alwaysRow struct {
	ID   int64
	Body string
}

var alwaysRowMeta = momo.Meta[alwaysRow]{
	Table:      "identity_always_rows",
	Columns:    []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity}, {Name: "body"}},
	Ptr:        func(m *alwaysRow, i int) any { return [...]any{&m.ID, &m.Body}[i] },
	Value:      func(m *alwaysRow, i int) any { return [...]any{m.ID, m.Body}[i] },
	Identity:   0,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (*alwaysRow) MomoMeta() *momo.Meta[alwaysRow] { return &alwaysRowMeta }

// TestInsertModelOnIdentityAlwaysColumn is issue 56: Insert.Model leaves the
// GENERATED ALWAYS column out of the INSERT (PostgreSQL would refuse a value:
// "cannot insert a non-DEFAULT value into column") and RETURNING reads the
// generated id back into the model.
func TestInsertModelOnIdentityAlwaysColumn(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	for _, s := range []string{
		"DROP TABLE IF EXISTS identity_always_rows",
		"CREATE TABLE identity_always_rows (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, body text NOT NULL)",
	} {
		if _, err := momo.Exec(ctx, db, s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	t.Cleanup(func() { _, _ = momo.Exec(ctx, db, "DROP TABLE IF EXISTS identity_always_rows") })

	// The control: a client-supplied value is refused by the database.
	if _, err := momo.Exec(ctx, db, "INSERT INTO identity_always_rows (id, body) VALUES (99, 'x')"); err == nil {
		t.Fatal("PostgreSQL accepted a supplied value for a GENERATED ALWAYS column")
	}

	idCol := momo.NewColumn[int64]("identity_always_rows", "id", momo.FlagPK|momo.FlagIdentity)
	row := alwaysRow{ID: 12345, Body: "first"} // a non-zero id must still be left out
	rows, err := momo.Insert[alwaysRow](db).Model(&row).Returning(idCol).Rows(ctx)
	if err != nil {
		t.Fatalf("insert on a GENERATED ALWAYS column: %v", err)
	}
	if !rows.Next() {
		t.Fatal("no RETURNING row")
	}
	var got alwaysRow
	if err := rows.ScanModel(&got); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if got.ID == 0 || got.ID == 12345 {
		t.Fatalf("returned id = %d, want the generated one", got.ID)
	}
}
