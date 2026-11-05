// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestUpsertUpdatesAllColumns(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		doc := suite.Document{ID: 77, TenantID: 1, Title: "first"}
		if _, err := momo.Upsert[suite.Document](db).Model(&doc).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		doc.Title = "second"
		doc.TenantID = 9
		if _, err := momo.Upsert[suite.Document](db).Model(&doc).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := momo.Select[suite.Document](db).Where(suite.DocID.Eq(77)).One(ctx)
		if err != nil || got.Title != "second" || got.TenantID != 9 {
			t.Fatalf("upsert did not update all columns: %+v %v", got, err)
		}
		n, err := momo.Select[suite.Document](db).Count(ctx)
		if err != nil || n != 1 {
			t.Fatalf("rows = %d, %v; want 1", n, err)
		}
	})
}

func TestUpsertZeroIdentityInsertsNewRow(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		doc := suite.Document{TenantID: 1, Title: "fresh"}
		if _, err := momo.Upsert[suite.Document](db).Model(&doc).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("fresh")).One(ctx)
		if err != nil || got.ID == 0 {
			t.Fatalf("zero identity must be generated: %+v %v", got, err)
		}
	})
}

func upsertSQL(t *testing.T, name dialect.Name, build func(q momo.Querier) momo.InsertQuery[suite.Document]) (momo.Compiled, error) {
	t.Helper()
	return build(momo.New(fakedriver.New(name))).SQL()
}

func TestUpsertGoldenSQL(t *testing.T) {
	doc := suite.Document{ID: 77, TenantID: 1, Title: "t"}
	zero := suite.Document{TenantID: 1, Title: "t"}
	const set = `DO UPDATE SET "tenant_id" = EXCLUDED."tenant_id", "title" = EXCLUDED."title", "body" = EXCLUDED."body", "secret" = EXCLUDED."secret"`
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
		}, `INSERT INTO "documents" ("id", "tenant_id", "title", "version", "deleted_at", "created_at", "updated_at") OVERRIDING SYSTEM VALUE VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT ("id") DO UPDATE SET "tenant_id" = EXCLUDED."tenant_id", "title" = EXCLUDED."title"`},
		{dialect.Postgres, func(q momo.Querier) momo.InsertQuery[suite.Document] {
			return momo.Upsert[suite.Document](q).Model(&doc).OnConflict(suite.DocTitle).DoUpdate(suite.DocTenantID.SetExcluded())
		}, `INSERT INTO "documents" ("tenant_id", "title", "body", "version", "deleted_at", "created_at", "updated_at", "secret") VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT ("title") DO UPDATE SET "tenant_id" = EXCLUDED."tenant_id"`},
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

func TestUpsertMySQLUnsupported(t *testing.T) {
	doc := suite.Document{ID: 1, TenantID: 1, Title: "t"}
	_, err := upsertSQL(t, dialect.MySQL, func(q momo.Querier) momo.InsertQuery[suite.Document] {
		return momo.Upsert[suite.Document](q).Model(&doc)
	})
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
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
