// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

const unitDocCols = `"documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at"`

func TestLockingHintsTableNameGolden(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	sq := momo.New(fakedriver.New(dialect.SQLite))
	my := momo.New(fakedriver.New(dialect.MySQL))

	cases := []struct {
		name string
		q    func() (momo.Compiled, error)
		want string
	}{
		{"for update skip locked postgres", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](pg).ForUpdate("SKIP LOCKED").SQL()
		}, `SELECT ` + unitDocCols + ` FROM "documents" WHERE "documents"."deleted_at" IS NULL FOR UPDATE SKIP LOCKED`},
		{"for share nowait postgres", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](pg).ForShare("NOWAIT").SQL()
		}, `SELECT ` + unitDocCols + ` FROM "documents" WHERE "documents"."deleted_at" IS NULL FOR SHARE NOWAIT`},
		{"indexed by sqlite quotes the name", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](sq).IndexedBy(`idx"x`).SQL()
		}, `SELECT ` + unitDocCols + ` FROM "documents" INDEXED BY "idx""x" WHERE "documents"."deleted_at" IS NULL`},
		{"table name keeps model qualifier via alias", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](pg).Where(DocTenantID.Eq(1)).WithTableName(`docs"unix`).SQL()
		}, `SELECT ` + unitDocCols + ` FROM "docs""unix" AS "documents" WHERE ("documents"."tenant_id" = $1) AND ("documents"."deleted_at" IS NULL)`},
		{"table name then indexed by on sqlite", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](sq).WithTableName("docs_unix").IndexedBy("i").SQL()
		}, `SELECT ` + unitDocCols + ` FROM "docs_unix" AS "documents" INDEXED BY "i" WHERE "documents"."deleted_at" IS NULL`},
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

	for name, f := range map[string]func() error{
		"IndexedBy on postgres": func() error { _, err := momo.Select[unitDocument](pg).IndexedBy("i").SQL(); return err },
		"IndexedBy on mysql":    func() error { _, err := momo.Select[unitDocument](my).IndexedBy("i").SQL(); return err },
		"ForUpdate on sqlite":   func() error { _, err := momo.Select[unitDocument](sq).ForUpdate("NOWAIT").SQL(); return err },
	} {
		if err := f(); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s = %v, want ErrUnsupported", name, err)
		}
	}
	for name, f := range map[string]func() error{
		"empty table name": func() error { _, err := momo.Select[unitDocument](pg).WithTableName("").SQL(); return err },
		"empty index name": func() error { _, err := momo.Select[unitDocument](sq).IndexedBy("").SQL(); return err },
		"IndexedBy on subquery": func() error {
			_, err := momo.Select[unitDocument](sq).From(momo.FromSubquery(momo.RawSubquery("SELECT 1"), "c")).IndexedBy("i").SQL()
			return err
		},
	} {
		if err := f(); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s = %v, want ErrValidation", name, err)
		}
	}

	// Immutability: deriving must not change the base query.
	base := momo.Select[unitDocument](pg)
	_ = base.WithTableName("other")
	if c, _ := base.SQL(); c.SQL != `SELECT `+unitDocCols+` FROM "documents" WHERE "documents"."deleted_at" IS NULL` {
		t.Errorf("base query mutated: %s", c.SQL)
	}
}
