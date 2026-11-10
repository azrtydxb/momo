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
	d := momo.TableRef{Name: "documents", Alias: "d"}
	other := momo.TableRef{Name: "tenants", Alias: "t"}
	join := momo.Select[unitDocument](pg).From(d).Join(other, DocTenantID.As("d").EqExpr(DocID.As("t")))

	cases := []struct {
		name string
		q    momo.SelectQuery[unitDocument]
		tail string
	}{
		{"no key update", join.ForNoKeyUpdate(), " FOR NO KEY UPDATE"},
		{"no key update nowait", join.ForNoKeyUpdate("NOWAIT"), " FOR NO KEY UPDATE NOWAIT"},
		{"no key update skip locked of", join.ForNoKeyUpdate("SKIP LOCKED").LockOf("d"), ` FOR NO KEY UPDATE OF "d" SKIP LOCKED`},
		{"of before mode", join.LockOf("d").ForNoKeyUpdate(), ` FOR NO KEY UPDATE OF "d"`},
		{"key share", join.ForKeyShare(), " FOR KEY SHARE"},
		{"key share nowait of", join.ForKeyShare("NOWAIT").LockOf("d", "t"), ` FOR KEY SHARE OF "d", "t" NOWAIT`},
		{"key share skip locked", join.ForKeyShare("SKIP LOCKED"), " FOR KEY SHARE SKIP LOCKED"},
	}
	for _, c := range cases {
		got, err := c.q.SQL()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if len(got.SQL) < len(c.tail) || got.SQL[len(got.SQL)-len(c.tail):] != c.tail {
			t.Errorf("%s: SQL %q does not end with %q", c.name, got.SQL, c.tail)
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
