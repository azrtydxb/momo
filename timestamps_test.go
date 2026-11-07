// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

func clockAt(t time.Time) momo.Option { return momo.WithClock(func() time.Time { return t }) }

func TestAutoTimestampsSQL(t *testing.T) {
	fixed := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	db := momo.New(fakedriver.New(dialect.Postgres), clockAt(fixed))

	c, err := momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1)).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if want := `INSERT INTO "documents" ("tenant_id", "created_at", "updated_at") VALUES ($1, $2, $3)`; c.SQL != want {
		t.Fatalf("SQL = %s\nwant  %s", c.SQL, want)
	}
	if c.Args[1] != fixed || c.Args[2] != fixed {
		t.Fatalf("args = %v", c.Args)
	}

	// Omit leaves the database default on a model insert.
	doc := suite.Document{TenantID: 1}
	c, err = momo.Insert[suite.Document](db).Model(&doc).Omit(suite.DocBody, suite.DocSecret, suite.DocCreatedAt, suite.DocUpdatedAt).SQL()
	if err != nil || strings.Contains(c.SQL, "created_at") || strings.Contains(c.SQL, "updated_at") {
		t.Fatalf("omitted timestamps written: %s %v", c.SQL, err)
	}

	c, err = momo.Update[suite.Document](db).Set(suite.DocTitle.Set("x")).Where(suite.DocID.Eq(1)).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if want := `UPDATE "documents" SET "title" = $1, "updated_at" = $2 WHERE "documents"."id" = $3`; c.SQL != want {
		t.Fatalf("SQL = %s\nwant  %s", c.SQL, want)
	}
	if c.Args[1] != fixed {
		t.Fatalf("args = %v", c.Args)
	}

	// The clock is read once per statement, even for multi-row inserts.
	n := 0
	cdb := momo.New(fakedriver.New(dialect.Postgres), momo.WithClock(func() time.Time { n++; return fixed }))
	rows := []suite.Document{{TenantID: 1, Title: "a"}, {TenantID: 2, Title: "b"}}
	if _, err := momo.Insert[suite.Document](cdb).Models(rows).SQL(); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("clock read %d times, want 1", n)
	}

	// An explicit Set of updated_at stays refused.
	if _, err := momo.Update[suite.Document](db).Set(suite.DocUpdatedAt.Set(fixed)).Where(suite.DocID.Eq(1)).SQL(); err == nil {
		t.Fatal("Set(updated_at) must still be refused on update")
	}
}
