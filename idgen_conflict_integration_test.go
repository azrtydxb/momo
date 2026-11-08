// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func idgenExec(t *testing.T, db *momo.DB, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := db.Driver().Exec(context.Background(), stmt, nil); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func idgenScanString(t *testing.T, db *momo.DB, query string, arg string) string {
	t.Helper()
	rows, err := db.Driver().Query(context.Background(), query, []any{arg})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return ""
	}
	var s string
	if err := rows.Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestIDGenConflictDuplicatesAndChunks covers DO NOTHING with two models that
// share a target value, and a chunked upsert that spans several statements.
func TestIDGenConflictDuplicatesAndChunks(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		ctx := context.Background()
		idgenExec(t, db, "DROP TABLE IF EXISTS id_uniq",
			"CREATE TABLE id_uniq (id text primary key, email text not null unique, name text not null)")
		email := momo.NewColumn[string]("id_uniq", "email", momo.FlagUnique)
		name := momo.NewColumn[string]("id_uniq", "name", 0)
		idOf := func(addr string) string {
			return idgenScanString(t, db, "SELECT id FROM id_uniq WHERE email = "+placeholder(db), addr)
		}

		// Two models with one email in a single DO NOTHING: the first is inserted.
		dup := []idUniq{{Email: "dup@x", Name: "first"}, {Email: "dup@x", Name: "second"}}
		if _, err := momo.Insert[idUniq](db).Models(dup).OnConflict(email).DoNothing().Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if !reCUID.MatchString(dup[0].ID) || dup[0].ID != idOf("dup@x") || dup[1].ID != "" {
			t.Fatalf("duplicate ids = %q, %q (stored %q)", dup[0].ID, dup[1].ID, idOf("dup@x"))
		}

		// A chunked upsert: 3 columns per row and 6 parameters per statement.
		for _, e := range []string{"e2@x", "e5@x"} {
			seed := idUniq{ID: "old-" + e, Email: e, Name: "seed"}
			if _, err := momo.Insert[idUniq](db).Model(&seed).Exec(ctx); err != nil {
				t.Fatal(err)
			}
		}
		limited := momo.New(db.Driver(), momo.WithMaxParams(6))
		ms := make([]idUniq, 7)
		for i := range ms {
			ms[i] = idUniq{Email: fmt.Sprintf("e%d@x", i+1), Name: "chunk"}
		}
		if _, err := momo.Insert[idUniq](limited).Models(ms).OnConflict(email).DoUpdate(name.SetExcluded()).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		for _, m := range ms {
			if m.ID == "" || m.ID != idOf(m.Email) {
				t.Fatalf("%s: model id %q, stored %q", m.Email, m.ID, idOf(m.Email))
			}
		}
		if ms[1].ID != "old-e2@x" || ms[4].ID != "old-e5@x" {
			t.Fatalf("existing rows lost their ids: %q, %q", ms[1].ID, ms[4].ID)
		}
	})
}

// idPub has a caller-assigned key and a generated public id.
type idPub struct {
	ID       string
	PublicID string
	Name     string
}

var idPubMeta = momo.Meta[idPub]{
	Table:    "id_pub",
	Columns:  []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "public_id", Flags: momo.FlagUnique}, {Name: "name"}},
	Ptr:      func(m *idPub, i int) any { return [...]any{&m.ID, &m.PublicID, &m.Name}[i] },
	Value:    func(m *idPub, i int) any { return [...]any{m.ID, m.PublicID, m.Name}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK:     []int{0},
	IDGens: []momo.IDGen{{Column: 1, Name: "uuid"}},
}

func (*idPub) MomoMeta() *momo.Meta[idPub] { return &idPubMeta }

// TestIDGenNonPKIDOnPrimaryKeyUpsert proves a generated column that is not the
// primary key reads back from the database on a primary-key upsert.
func TestIDGenNonPKIDOnPrimaryKeyUpsert(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		ctx := context.Background()
		idgenExec(t, db, "DROP TABLE IF EXISTS id_pub",
			"CREATE TABLE id_pub (id text primary key, public_id text not null unique, name text not null)")
		name := momo.NewColumn[string]("id_pub", "name", 0)
		first := idPub{ID: "k1", Name: "one"}
		if _, err := momo.Insert[idPub](db).Model(&first).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if !reUUID.MatchString(first.PublicID) {
			t.Fatalf("public id = %q", first.PublicID)
		}
		again := idPub{ID: "k1", Name: "two"}
		pk := momo.NewColumn[string]("id_pub", "id", momo.FlagPK)
		if _, err := momo.Insert[idPub](db).Model(&again).OnConflict(pk).DoUpdate(name.SetExcluded()).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		stored := idgenScanString(t, db, "SELECT public_id FROM id_pub WHERE id = "+placeholder(db), "k1")
		if again.PublicID != stored || stored != first.PublicID {
			t.Fatalf("public id: model %q, stored %q, first %q", again.PublicID, stored, first.PublicID)
		}
	})
}

// TestIDGenDefaultUpsertSetKeepsPublicID proves the default DO UPDATE set does
// not overwrite a stored generated id with the fresh one, and that the model
// reads the stored id back.
func TestIDGenDefaultUpsertSetKeepsPublicID(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		ctx := context.Background()
		idgenExec(t, db, "DROP TABLE IF EXISTS id_pub",
			"CREATE TABLE id_pub (id text primary key, public_id text not null unique, name text not null)")
		first := idPub{ID: "k1", Name: "one"}
		if _, err := momo.Insert[idPub](db).Model(&first).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		again := idPub{ID: "k1", Name: "two"}
		if _, err := momo.Upsert[idPub](db).Model(&again).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		stored := idgenScanString(t, db, "SELECT public_id FROM id_pub WHERE id = "+placeholder(db), "k1")
		name := idgenScanString(t, db, "SELECT name FROM id_pub WHERE id = "+placeholder(db), "k1")
		if stored != first.PublicID || again.PublicID != stored || name != "two" {
			t.Fatalf("public id: first %q, stored %q, model %q; name %q", first.PublicID, stored, again.PublicID, name)
		}
	})
}

// idPad has a conflict target that PostgreSQL pads on read.
type idPad struct {
	ID   string
	Code string
}

var idPadMeta = momo.Meta[idPad]{
	Table:    "id_pad",
	Columns:  []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "code", Flags: momo.FlagUnique}},
	Ptr:      func(m *idPad, i int) any { return [...]any{&m.ID, &m.Code}[i] },
	Value:    func(m *idPad, i int) any { return [...]any{m.ID, m.Code}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK:     []int{0},
	IDGens: []momo.IDGen{{Column: 0, Name: "cuid"}},
}

func (*idPad) MomoMeta() *momo.Meta[idPad] { return &idPadMeta }

// TestIDGenConflictTargetThatDoesNotRoundTripFails proves an unmatched
// RETURNING row is an error, not a silently missing id.
func TestIDGenConflictTargetThatDoesNotRoundTripFails(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	idgenExec(t, db, "DROP TABLE IF EXISTS id_pad",
		"CREATE TABLE id_pad (id text primary key, code char(8) not null unique)")
	code := momo.NewColumn[string]("id_pad", "code", momo.FlagUnique)
	m := idPad{Code: "ab"}
	_, err := momo.Insert[idPad](db).Model(&m).OnConflict(code).DoNothing().Exec(ctx)
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}
