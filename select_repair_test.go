// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestSoftDeleteFilterWithColumns(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	all := []momo.Expr{DocID, DocTenantID, DocTitle, DocPublished, DocDeletedAt}
	c, err := momo.Select[unitDocument](db).Columns(all...).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE "documents"."deleted_at" IS NULL`
	if c.SQL != want {
		t.Fatalf("soft-delete filter lost when columns are restricted:\n got %s\nwant %s", c.SQL, want)
	}
}

func TestTableRefSchemaQualified(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	c, err := momo.Select[unitDocument](db).From(momo.TableRef{Name: "documents", Schema: "tenant"}).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if want := `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "tenant"."documents" WHERE "documents"."deleted_at" IS NULL`; c.SQL != want {
		t.Fatalf("got %s\nwant %s", c.SQL, want)
	}
}
