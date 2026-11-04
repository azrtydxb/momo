// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestKeysetPagination(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	c, err := momo.Select[unitDocument](pg).OrderBy(DocTenantID.Desc(), DocTitle.Asc()).After(int64(7), "m", int64(42)).Limit(10).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE ("documents"."tenant_id" < $1 OR ("documents"."tenant_id" = $2 AND "documents"."title" > $3) OR ("documents"."tenant_id" = $4 AND "documents"."title" = $5 AND "documents"."id" > $6)) AND ("documents"."deleted_at" IS NULL) ORDER BY "documents"."tenant_id" DESC, "documents"."title" ASC, "documents"."id" ASC LIMIT $7`
	if c.SQL != want {
		t.Fatalf("keyset SQL:\n got %s\nwant %s", c.SQL, want)
	}
	wantArgs := []any{int64(7), int64(7), "m", int64(7), "m", int64(42), int64(10)}
	if len(c.Args) != len(wantArgs) {
		t.Fatalf("keyset args: %#v", c.Args)
	}
	for i := range wantArgs {
		if c.Args[i] != wantArgs[i] {
			t.Fatalf("arg %d = %#v, want %#v", i, c.Args[i], wantArgs[i])
		}
	}
	if _, err := momo.Select[unitDocument](pg).After(1).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("After without OrderBy = %v, want ErrValidation", err)
	}
	if _, err := momo.Select[unitDocument](pg).OrderBy(DocTitle.Asc()).After("a").SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("After with too few values = %v, want ErrValidation", err)
	}
}
