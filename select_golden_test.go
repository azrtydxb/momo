// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestSelectGolden(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	sq := momo.New(fakedriver.New(dialect.SQLite))
	my := momo.New(fakedriver.New(dialect.MySQL))

	cases := []struct {
		name string
		q    func() (momo.Compiled, error)
		want string
	}{
		{"where and order with nulls", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](pg).Where(DocTitle.Eq("a")).OrderBy(DocTitle.Asc().NullsLast()).SQL()
		}, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE ("documents"."title" = $1) AND ("documents"."deleted_at" IS NULL) ORDER BY "documents"."title" ASC NULLS LAST`},
		{"limit and offset postgres", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](pg).Limit(10).Offset(20).SQL()
		}, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE "documents"."deleted_at" IS NULL LIMIT $1 OFFSET $2`},
		{"offset without limit on sqlite", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](sq).Offset(5).SQL()
		}, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE "documents"."deleted_at" IS NULL LIMIT -1 OFFSET ?`},
		{"self join with alias", func() (momo.Compiled, error) {
			p := momo.TableRef{Name: "documents", Alias: "parent"}
			return momo.Select[unitDocument](pg).Join(p, DocTenantID.As("documents").EqExpr(DocTenantID.As("parent"))).SQL()
		}, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" INNER JOIN "documents" AS "parent" ON "documents"."tenant_id" = "parent"."tenant_id" WHERE "documents"."deleted_at" IS NULL`},
	}
	for _, c := range cases {
		got, err := c.q()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got.SQL != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got.SQL, c.want)
		}
	}

	if _, err := momo.Select[unitDocument](my).OrderBy(DocTitle.Asc().NullsFirst()).SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("NULLS FIRST on MySQL = %v, want ErrUnsupported", err)
	}
	if _, err := momo.Select[unitDocument](sq).ForUpdate().SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("FOR UPDATE on SQLite = %v, want ErrUnsupported", err)
	}
}
