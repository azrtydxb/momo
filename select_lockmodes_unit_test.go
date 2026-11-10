// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestNoKeyUpdateAndKeyShareGolden(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	base := momo.Select[unitDocument](pg)

	cases := []struct {
		name string
		q    momo.SelectQuery[unitDocument]
		tail string
	}{
		{"no key update", base.ForNoKeyUpdate(), " FOR NO KEY UPDATE"},
		{"no key update nowait", base.ForNoKeyUpdate("NOWAIT"), " FOR NO KEY UPDATE NOWAIT"},
		{"no key update skip locked of", base.ForNoKeyUpdate("SKIP LOCKED").LockOf("documents"), ` FOR NO KEY UPDATE OF "documents" SKIP LOCKED`},
		{"of before mode", base.LockOf("documents").ForNoKeyUpdate(), ` FOR NO KEY UPDATE OF "documents"`},
		{"key share", base.ForKeyShare(), " FOR KEY SHARE"},
		{"key share nowait of", base.ForKeyShare("NOWAIT").LockOf("documents"), ` FOR KEY SHARE OF "documents" NOWAIT`},
		{"key share skip locked", base.ForKeyShare("SKIP LOCKED"), " FOR KEY SHARE SKIP LOCKED"},
	}
	const head = `SELECT ` + unitDocCols + ` FROM "documents" WHERE "documents"."deleted_at" IS NULL`
	for _, c := range cases {
		got, err := c.q.SQL()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got.SQL != head+c.tail {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got.SQL, head+c.tail)
		}
	}
}

func TestNoKeyUpdateAndKeyShareUnsupported(t *testing.T) {
	for _, n := range []dialect.Name{dialect.MySQL, dialect.SQLite} {
		db := momo.New(fakedriver.New(n))
		for name, q := range map[string]momo.SelectQuery[unitDocument]{
			"ForNoKeyUpdate":        momo.Select[unitDocument](db).ForNoKeyUpdate(),
			"ForNoKeyUpdate NOWAIT": momo.Select[unitDocument](db).ForNoKeyUpdate("NOWAIT"),
			"ForKeyShare":           momo.Select[unitDocument](db).ForKeyShare(),
			"ForKeyShare SKIP":      momo.Select[unitDocument](db).ForKeyShare("SKIP LOCKED"),
			"ForNoKeyUpdate OF":     momo.Select[unitDocument](db).ForNoKeyUpdate().LockOf("documents"),
			"ForKeyShare OF":        momo.Select[unitDocument](db).ForKeyShare().LockOf("documents"),
		} {
			if _, err := q.SQL(); !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s on %s = %v, want ErrUnsupported", name, n, err)
			}
		}
	}
}

func TestNoKeyUpdateValidation(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	base := momo.Select[unitDocument](pg)
	for name, q := range map[string]momo.SelectQuery[unitDocument]{
		"both wait options": base.ForNoKeyUpdate("NOWAIT", "SKIP LOCKED"),
		"unknown option":    base.ForKeyShare("WAIT"),
		"distinct":          base.Distinct().ForNoKeyUpdate(),
		"group by":          base.GroupBy(DocTenantID).ForKeyShare(),
		"unknown lock-of":   base.ForNoKeyUpdate().LockOf("nope"),
	} {
		if _, err := q.SQL(); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
}
