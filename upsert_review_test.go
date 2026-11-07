// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

// compositeRow is a model with a composite primary key and no identity.
type compositeRow struct {
	A, B int64
	V    string
}

var compositeMeta = &momo.Meta[compositeRow]{
	Table: "pairs",
	Columns: []momo.ColumnMeta{
		{Name: "a", Flags: momo.FlagPK},
		{Name: "b", Flags: momo.FlagPK},
		{Name: "v"},
	},
	Ptr:      func(m *compositeRow, i int) any { return []any{&m.A, &m.B, &m.V}[i] },
	Value:    func(m *compositeRow, i int) any { return []any{m.A, m.B, m.V}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0, 1},
}

func (*compositeRow) MomoMeta() *momo.Meta[compositeRow] { return compositeMeta }

func TestUpsertCompositePrimaryKeyGolden(t *testing.T) {
	row := compositeRow{A: 1, B: 2, V: "x"}
	got, err := momo.Upsert[compositeRow](momo.New(fakedriver.New(dialect.Postgres))).Model(&row).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `INSERT INTO "pairs" ("a", "b", "v") VALUES ($1, $2, $3) ON CONFLICT ("a", "b") DO UPDATE SET "v" = EXCLUDED."v"`
	if got.SQL != want {
		t.Fatalf("got  %s\nwant %s", got.SQL, want)
	}
}

func TestUpsertDoNothingGolden(t *testing.T) {
	doc := suite.Document{ID: 77, TenantID: 1, Title: "t"}
	got, err := upsertSQL(t, dialect.Postgres, func(q momo.Querier) momo.InsertQuery[suite.Document] {
		return momo.Upsert[suite.Document](q).Model(&doc).OnConflict(suite.DocID).DoNothing()
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `INSERT INTO "documents" ("id", "tenant_id", "title", "body", "version", "deleted_at", "created_at", "updated_at", "secret") OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT ("id") DO NOTHING`
	if got.SQL != want {
		t.Fatalf("got  %s\nwant %s", got.SQL, want)
	}
}

func TestUpsertNonPKTargetNeverInsertsIdentity(t *testing.T) {
	doc := suite.Document{ID: 77, TenantID: 1, Title: "t"}
	got, err := upsertSQL(t, dialect.Postgres, func(q momo.Querier) momo.InsertQuery[suite.Document] {
		return momo.Upsert[suite.Document](q).Model(&doc).OnConflict(suite.DocTitle).DoUpdate()
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.SQL, "OVERRIDING") || strings.Contains(got.SQL, `("id"`) {
		t.Fatalf("identity column must not be inserted for a non-PK target: %s", got.SQL)
	}
	want := `INSERT INTO "documents" ("tenant_id", "title", "body", "version", "deleted_at", "created_at", "updated_at", "secret") VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT ("title") DO UPDATE SET "tenant_id" = EXCLUDED."tenant_id", "title" = EXCLUDED."title", "body" = EXCLUDED."body", "secret" = EXCLUDED."secret", "updated_at" = EXCLUDED."updated_at"`
	if got.SQL != want {
		t.Fatalf("got  %s\nwant %s", got.SQL, want)
	}
}

func TestUpsertExplicitDoUpdateStillRefreshesUpdatedAt(t *testing.T) {
	doc := suite.Document{ID: 1, TenantID: 1, Title: "t"}
	got, err := upsertSQL(t, dialect.Postgres, func(q momo.Querier) momo.InsertQuery[suite.Document] {
		return momo.Upsert[suite.Document](q).Model(&doc).OnConflict(suite.DocID).DoUpdate(suite.DocTitle.SetExcluded())
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got.SQL, `DO UPDATE SET "title" = EXCLUDED."title", "updated_at" = EXCLUDED."updated_at"`) {
		t.Fatalf("updated_at not appended: %s", got.SQL)
	}
	if strings.Contains(got.SQL, `"created_at" = `) {
		t.Fatalf("created_at must never be overwritten: %s", got.SQL)
	}
}
