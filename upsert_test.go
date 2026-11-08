// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

func upsertSQL(t *testing.T, name dialect.Name, build func(q momo.Querier) momo.InsertQuery[suite.Document]) (momo.Compiled, error) {
	t.Helper()
	return build(momo.New(fakedriver.New(name))).SQL()
}

func TestUpsertGoldenSQL(t *testing.T) {
	doc := suite.Document{ID: 77, TenantID: 1, Title: "t"}
	zero := suite.Document{TenantID: 1, Title: "t"}
	const set = `DO UPDATE SET "tenant_id" = EXCLUDED."tenant_id", "title" = EXCLUDED."title", "body" = EXCLUDED."body", "secret" = EXCLUDED."secret", "updated_at" = EXCLUDED."updated_at"`
	cases := []struct {
		name  dialect.Name
		build func(q momo.Querier) momo.InsertQuery[suite.Document]
		want  string
	}{
		{dialect.Postgres, func(q momo.Querier) momo.InsertQuery[suite.Document] {
			return momo.Upsert[suite.Document](q).Model(&doc)
		}, `INSERT INTO "documents" ("id", "tenant_id", "title", "body", "version", "deleted_at", "created_at", "updated_at", "secret") OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT ("id") ` + set},
		{dialect.SQLite, func(q momo.Querier) momo.InsertQuery[suite.Document] {
			return momo.Upsert[suite.Document](q).Model(&doc)
		}, `INSERT INTO "documents" ("id", "tenant_id", "title", "body", "version", "deleted_at", "created_at", "updated_at", "secret") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT ("id") ` + set},
		{dialect.Postgres, func(q momo.Querier) momo.InsertQuery[suite.Document] {
			return momo.Upsert[suite.Document](q).Model(&zero)
		}, `INSERT INTO "documents" ("tenant_id", "title", "body", "version", "deleted_at", "created_at", "updated_at", "secret") VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT ("id") ` + set},
		{dialect.Postgres, func(q momo.Querier) momo.InsertQuery[suite.Document] {
			return momo.Upsert[suite.Document](q).Model(&doc).Omit(suite.DocBody, suite.DocSecret)
		}, `INSERT INTO "documents" ("id", "tenant_id", "title", "version", "deleted_at", "created_at", "updated_at") OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT ("id") DO UPDATE SET "tenant_id" = EXCLUDED."tenant_id", "title" = EXCLUDED."title", "updated_at" = EXCLUDED."updated_at"`},
		{dialect.Postgres, func(q momo.Querier) momo.InsertQuery[suite.Document] {
			return momo.Upsert[suite.Document](q).Model(&doc).OnConflict(suite.DocTitle).DoUpdate(suite.DocTenantID.SetExcluded())
		}, `INSERT INTO "documents" ("tenant_id", "title", "body", "version", "deleted_at", "created_at", "updated_at", "secret") VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT ("title") DO UPDATE SET "tenant_id" = EXCLUDED."tenant_id", "updated_at" = EXCLUDED."updated_at"`},
		{dialect.Postgres, func(q momo.Querier) momo.InsertQuery[suite.Document] {
			return momo.Upsert[suite.Document](q).Model(&doc).Returning(suite.DocID)
		}, `INSERT INTO "documents" ("id", "tenant_id", "title", "body", "version", "deleted_at", "created_at", "updated_at", "secret") OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT ("id") ` + set + ` RETURNING "id"`},
	}
	for i, c := range cases {
		got, err := upsertSQL(t, c.name, c.build)
		if err != nil {
			t.Fatalf("case %d (%s): %v", i, c.name, err)
		}
		if got.SQL != c.want {
			t.Errorf("case %d (%s):\n got %s\nwant %s", i, c.name, got.SQL, c.want)
		}
	}
}

// Catches: MySQL failing an upsert that ON DUPLICATE KEY UPDATE can express, or
// rendering the PostgreSQL EXCLUDED reference.
func TestUpsertMySQLRendersOnDuplicateKey(t *testing.T) {
	doc := suite.Document{ID: 1, TenantID: 1, Title: "t"}
	c, err := upsertSQL(t, dialect.MySQL, func(q momo.Querier) momo.InsertQuery[suite.Document] {
		return momo.Upsert[suite.Document](q).Model(&doc)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{" AS `momo_new` ON DUPLICATE KEY UPDATE ", "`title` = `momo_new`.`title`"} {
		if !strings.Contains(c.SQL, want) {
			t.Errorf("SQL %s lacks %s", c.SQL, want)
		}
	}
	if strings.Contains(c.SQL, "EXCLUDED") || strings.Contains(c.SQL, "ON CONFLICT") {
		t.Errorf("SQL %s uses PostgreSQL syntax", c.SQL)
	}
}

func TestUpsertRefusesReservedColumnsInDoUpdate(t *testing.T) {
	doc := suite.Document{ID: 1, TenantID: 1, Title: "t"}
	for name, a := range map[string]momo.Assignment{
		"primary key": suite.DocID.SetExcluded(),
		"version":     suite.DocVersion.SetExcluded(),
		"soft-delete": suite.DocDeletedAt.SetExcluded(),
		"created-at":  suite.DocCreatedAt.SetExcluded(),
		"updated-at":  suite.DocUpdatedAt.SetExcluded(),
	} {
		_, err := upsertSQL(t, dialect.Postgres, func(q momo.Querier) momo.InsertQuery[suite.Document] {
			return momo.Upsert[suite.Document](q).Model(&doc).OnConflict(suite.DocID).DoUpdate(a)
		})
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
}

func TestUpsertValidation(t *testing.T) {
	a := suite.Document{ID: 1, TenantID: 1, Title: "a"}
	b := suite.Document{TenantID: 1, Title: "b"}
	_, err := upsertSQL(t, dialect.Postgres, func(q momo.Querier) momo.InsertQuery[suite.Document] {
		return momo.Upsert[suite.Document](q).Models([]suite.Document{a, b})
	})
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("mixed explicit and zero identity: err = %v, want ErrValidation", err)
	}
}

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
