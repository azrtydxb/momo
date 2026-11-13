// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// ageUpdatedAt writes an old updated_at through raw SQL, so a later bump is
// detectable whatever the clock of the statement under test says.
func ageUpdatedAt(t *testing.T, db *momo.DB, id int64) {
	t.Helper()
	if _, err := momo.Exec(context.Background(), db, "UPDATE documents SET updated_at = '2001-02-03 04:05:06' WHERE id = ?", id); err != nil {
		t.Fatal(err)
	}
}

// Catches: a touch (Update with no Set) failing, not bumping updated_at, or
// writing any other column, on PostgreSQL, SQLite and MySQL.
func TestUpdateTouchChangesOnlyUpdatedAt(t *testing.T) {
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		clocked := momo.New(db.Driver(), clockAt(now))
		doc := suite.Document{ID: 701, TenantID: 3, Title: "touch", Version: 4}
		if _, err := momo.Upsert[suite.Document](db).Model(&doc).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		ageUpdatedAt(t, db, 701)
		before, err := momo.Select[suite.Document](db).Where(suite.DocID.Eq(701)).One(ctx)
		if err != nil || before.UpdatedAt.Equal(now) {
			t.Fatalf("setup: %+v %v", before, err)
		}
		res, err := momo.Update[suite.Document](clocked).Where(suite.DocID.Eq(701)).Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("touch: %+v %v", res, err)
		}
		after, err := momo.Select[suite.Document](db).Where(suite.DocID.Eq(701)).One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !after.UpdatedAt.Equal(now) {
			t.Fatalf("updated_at = %v, want %v", after.UpdatedAt, now)
		}
		after.UpdatedAt = before.UpdatedAt
		if after.ID != before.ID || after.TenantID != before.TenantID || after.Title != before.Title ||
			after.Version != before.Version || !after.CreatedAt.Equal(before.CreatedAt) || after.Secret != before.Secret {
			t.Fatalf("touch changed another column: before %+v after %+v", before, after)
		}
	})
}

// Catches: an upsert that hits the conflict leaving updated_at at the old
// value, or an upsert that inserts not stamping both timestamps with the
// statement time.
func TestUpsertUpdatedAtOnConflictAndOnInsert(t *testing.T) {
	t1 := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	t2 := t1.Add(3 * time.Hour)
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		db1 := momo.New(db.Driver(), clockAt(t1))
		db2 := momo.New(db.Driver(), clockAt(t2))

		fresh := suite.Document{ID: 711, TenantID: 1, Title: "fresh"}
		if _, err := momo.Upsert[suite.Document](db1).Model(&fresh).OnConflict(suite.DocID).DoUpdate(suite.DocTitle.SetExcluded()).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := momo.Select[suite.Document](db).Where(suite.DocID.Eq(711)).One(ctx)
		if err != nil || !got.CreatedAt.Equal(t1) || !got.UpdatedAt.Equal(t1) {
			t.Fatalf("insert branch: created %v updated %v (%v), want both %v", got.CreatedAt, got.UpdatedAt, err, t1)
		}

		ageUpdatedAt(t, db, 711)
		again := suite.Document{ID: 711, TenantID: 1, Title: "changed"}
		if _, err := momo.Upsert[suite.Document](db2).Model(&again).OnConflict(suite.DocID).DoUpdate(suite.DocTitle.SetExcluded()).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		got, err = momo.Select[suite.Document](db).Where(suite.DocID.Eq(711)).One(ctx)
		if err != nil || got.Title != "changed" || !got.UpdatedAt.Equal(t2) || !got.CreatedAt.Equal(t1) {
			t.Fatalf("conflict branch: title %q created %v updated %v (%v), want updated %v and created %v", got.Title, got.CreatedAt, got.UpdatedAt, err, t2, t1)
		}
	})
}

// Catches: a conflict branch that copies the struct's own UpdatedAt (the
// load, change, upsert round trip writes the old timestamp back), or one that
// skips the bump when Omit(updated_at) keeps the column out of the INSERT.
func TestUpsertBumpsAStaleStructAndAnOmittedColumn(t *testing.T) {
	t1 := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	t2 := t1.Add(5 * time.Hour)
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		db1 := momo.New(db.Driver(), clockAt(t1))
		db2 := momo.New(db.Driver(), clockAt(t2))
		seed := suite.Document{ID: 731, TenantID: 1, Title: "seed"}
		if _, err := momo.Upsert[suite.Document](db1).Model(&seed).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		ageUpdatedAt(t, db, 731)
		loaded, err := momo.Select[suite.Document](db).Where(suite.DocID.Eq(731)).One(ctx)
		if err != nil || loaded.UpdatedAt.Equal(t2) {
			t.Fatalf("setup: %+v %v", loaded, err)
		}
		loaded.Title = "edited" // the struct still carries the old UpdatedAt
		if _, err := momo.Upsert[suite.Document](db2).Model(&loaded).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := momo.Select[suite.Document](db).Where(suite.DocID.Eq(731)).One(ctx)
		if err != nil || got.Title != "edited" || !got.UpdatedAt.Equal(t2) {
			t.Fatalf("stale struct: title %q updated %v (%v), want updated %v", got.Title, got.UpdatedAt, err, t2)
		}

		ageUpdatedAt(t, db, 731)
		again := suite.Document{ID: 731, TenantID: 1, Title: "omitted"}
		if _, err := momo.Upsert[suite.Document](db2).Model(&again).Omit(suite.DocUpdatedAt).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		got, err = momo.Select[suite.Document](db).Where(suite.DocID.Eq(731)).One(ctx)
		if err != nil || got.Title != "omitted" || !got.UpdatedAt.Equal(t2) {
			t.Fatalf("Omit(updated_at): title %q updated %v (%v), want updated %v", got.Title, got.UpdatedAt, err, t2)
		}
	})
}

// Catches: a plain Insert with OnConflict and DoUpdate (not Upsert) leaving
// updated_at alone on conflict, or copying the struct's stale UpdatedAt, and an
// insert that does not conflict losing its normal timestamps. MySQL is skipped:
// its ON DUPLICATE KEY UPDATE accepts only the table's single unique key as a
// target, and a plain Insert never writes the identity primary key.
func TestPlainInsertDoUpdateBumpsUpdatedAt(t *testing.T) {
	t1 := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	t2 := t1.Add(5 * time.Hour)
	stale := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d == dialect.MySQL {
			t.Skip("MySQL needs the conflict target to be the only unique key")
		}
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "CREATE UNIQUE INDEX plain_title_uq ON documents (tenant_id, title)"); err != nil {
			t.Fatal(err)
		}
		db1 := momo.New(db.Driver(), clockAt(t1))
		db2 := momo.New(db.Driver(), clockAt(t2))
		insert := func(c *momo.DB, title string, updatedAt time.Time) {
			t.Helper()
			doc := suite.Document{TenantID: 1, Title: title, Secret: title, UpdatedAt: updatedAt}
			_, err := momo.Insert[suite.Document](c).Model(&doc).
				OnConflict(suite.DocTenantID, suite.DocTitle).DoUpdate(suite.DocSecret.SetExcluded()).Exec(ctx)
			if err != nil {
				t.Fatal(err)
			}
		}
		insert(db1, "plain", time.Time{})
		got, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("plain")).One(ctx)
		if err != nil || !got.CreatedAt.Equal(t1) || !got.UpdatedAt.Equal(t1) {
			t.Fatalf("insert branch: created %v updated %v (%v), want both %v", got.CreatedAt, got.UpdatedAt, err, t1)
		}
		if _, err := momo.Exec(ctx, db, "UPDATE documents SET updated_at = '2001-02-03 04:05:06' WHERE title = ?", "plain"); err != nil {
			t.Fatal(err)
		}
		insert(db2, "plain", stale)
		got, err = momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("plain")).One(ctx)
		if err != nil || !got.UpdatedAt.Equal(t2) || !got.CreatedAt.Equal(t1) {
			t.Fatalf("conflict branch: created %v updated %v (%v), want updated %v and created %v", got.CreatedAt, got.UpdatedAt, err, t2, t1)
		}
	})
}
