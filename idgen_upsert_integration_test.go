// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// idUniq has a generated text key and a unique email to conflict on.
type idUniq struct {
	ID    string
	Email string
	Name  string
}

var idUniqMeta = momo.Meta[idUniq]{
	Table:    "id_uniq",
	Columns:  []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "email", Flags: momo.FlagUnique}, {Name: "name"}},
	Ptr:      func(m *idUniq, i int) any { return [...]any{&m.ID, &m.Email, &m.Name}[i] },
	Value:    func(m *idUniq, i int) any { return [...]any{m.ID, m.Email, m.Name}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK:     []int{0},
	IDGens: []momo.IDGen{{Column: 0, Name: "cuid"}},
}

func (*idUniq) MomoMeta() *momo.Meta[idUniq] { return &idUniqMeta }

// TestIDGenUpsertKeepsStoredID proves that an upsert on a column other than the
// primary key writes the id the database kept into the caller's model, not the
// freshly generated one, and leaves a row that DO NOTHING skipped alone.
func TestIDGenUpsertKeepsStoredID(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		ctx := context.Background()
		for _, stmt := range []string{
			"DROP TABLE IF EXISTS id_uniq",
			"CREATE TABLE id_uniq (id text primary key, email text not null unique, name text not null)",
		} {
			if _, err := db.Driver().Exec(ctx, stmt, nil); err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
		}
		email := momo.NewColumn[string]("id_uniq", "email", momo.FlagUnique)
		name := momo.NewColumn[string]("id_uniq", "name", 0)
		idOf := func(addr string) string {
			rows, err := db.Driver().Query(ctx, "SELECT id FROM id_uniq WHERE email = "+placeholder(db), []any{addr})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = rows.Close() }()
			if !rows.Next() {
				return ""
			}
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			return id
		}
		seed := idUniq{ID: "old", Email: "a@x", Name: "first"}
		if _, err := momo.Insert[idUniq](db).Model(&seed).Exec(ctx); err != nil {
			t.Fatal(err)
		}

		// DO UPDATE on email: the conflicting model reads back the stored id.
		ms := []idUniq{{Email: "a@x", Name: "second"}, {Email: "b@x", Name: "new"}}
		if _, err := momo.Insert[idUniq](db).Models(ms).OnConflict(email).DoUpdate(name.SetExcluded()).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if ms[0].ID != "old" || idOf("a@x") != "old" {
			t.Fatalf("conflicting model id = %q, stored %q, want old", ms[0].ID, idOf("a@x"))
		}
		if !reCUID.MatchString(ms[1].ID) || idOf("b@x") != ms[1].ID {
			t.Fatalf("new model id = %q, stored %q", ms[1].ID, idOf("b@x"))
		}

		// DO NOTHING on email: the skipped model keeps its (zero) id.
		ms = []idUniq{{Email: "a@x", Name: "again"}, {Email: "c@x", Name: "c"}}
		if _, err := momo.Insert[idUniq](db).Models(ms).OnConflict(email).DoNothing().Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if ms[0].ID != "" || !reCUID.MatchString(ms[1].ID) || idOf("c@x") != ms[1].ID {
			t.Fatalf("DO NOTHING ids = %q, %q (stored c@x %q)", ms[0].ID, ms[1].ID, idOf("c@x"))
		}

		// DO NOTHING without a target.
		ms = []idUniq{{Email: "a@x", Name: "again"}, {Email: "d@x", Name: "d"}}
		if _, err := momo.Insert[idUniq](db).Models(ms).OnConflict().DoNothing().Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if ms[0].ID != "" || !reCUID.MatchString(ms[1].ID) || idOf("d@x") != ms[1].ID {
			t.Fatalf("targetless DO NOTHING ids = %q, %q (stored d@x %q)", ms[0].ID, ms[1].ID, idOf("d@x"))
		}

		// Upsert on the primary key still writes the id back up front.
		up := idUniq{Email: "e@x", Name: "e"}
		if _, err := momo.Upsert[idUniq](db).Model(&up).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if !reCUID.MatchString(up.ID) || idOf("e@x") != up.ID {
			t.Fatalf("pk upsert id = %q, stored %q", up.ID, idOf("e@x"))
		}
	})
}
