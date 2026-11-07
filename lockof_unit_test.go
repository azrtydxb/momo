// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestLockOfGolden(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	my := momo.New(fakedriver.New(dialect.MySQL))
	sq := momo.New(fakedriver.New(dialect.SQLite))

	d := momo.TableRef{Name: "documents", Alias: "d"}
	other := momo.TableRef{Name: "tenants", Alias: `t"x`}
	join := func(db *momo.DB) momo.SelectQuery[unitDocument] {
		return momo.Select[unitDocument](db).From(d).Join(other, DocTenantID.As("d").EqExpr(DocID.As(`t"x`)))
	}

	cases := []struct {
		name string
		q    momo.SelectQuery[unitDocument]
		tail string
	}{
		{"update of skip locked", join(pg).ForUpdate("SKIP LOCKED").LockOf("d"), ` FOR UPDATE OF "d" SKIP LOCKED`},
		{"order independent", join(pg).LockOf("d").ForUpdate("SKIP LOCKED"), ` FOR UPDATE OF "d" SKIP LOCKED`},
		{"share of nowait two tables", join(pg).ForShare("NOWAIT").LockOf("d", `t"x`), ` FOR SHARE OF "d", "t""x" NOWAIT`},
		{"update of only", join(pg).ForUpdate().LockOf("d"), ` FOR UPDATE OF "d"`},
		{"mysql", join(my).ForUpdate("SKIP LOCKED").LockOf("d"), " FOR UPDATE OF `d` SKIP LOCKED"},
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

	for name, err := range map[string]error{
		"sqlite":  buildErr(join(sq).ForUpdate().LockOf("d")),
		"no lock": buildErr(join(pg).LockOf("d")),
		"empty":   buildErr(join(pg).ForUpdate().LockOf()),
		"blank":   buildErr(join(pg).ForUpdate().LockOf("")),
	} {
		want := momo.ErrValidation
		if name == "sqlite" {
			want = momo.ErrUnsupported
		}
		if !errors.Is(err, want) {
			t.Errorf("%s: err = %v, want %v", name, err, want)
		}
	}

	// Immutability: LockOf must not change the base query.
	base := join(pg).ForUpdate()
	_ = base.LockOf("d")
	if c, _ := base.SQL(); c.SQL[len(c.SQL)-len(" FOR UPDATE"):] != " FOR UPDATE" {
		t.Errorf("base query mutated: %s", c.SQL)
	}
}

func buildErr(q momo.SelectQuery[unitDocument]) error {
	_, err := q.SQL()
	return err
}

func TestAdvisoryXactLockStatements(t *testing.T) {
	ctx := context.Background()
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{true}})
	db := momo.New(drv)
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if err := tx.AdvisoryXactLock(ctx, 42); err != nil {
			return err
		}
		if err := tx.AdvisoryXactLockString(ctx, "catalog:1"); err != nil {
			return err
		}
		if ok, err := tx.TryAdvisoryXactLock(ctx, 7); err != nil || !ok {
			t.Errorf("TryAdvisoryXactLock = %v, %v; want true, nil", ok, err)
		}
		if ok, err := tx.TryAdvisoryXactLockString(ctx, "doc:9"); err != nil || !ok {
			t.Errorf("TryAdvisoryXactLockString = %v, %v; want true, nil", ok, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"SELECT pg_advisory_xact_lock($1)",
		"SELECT pg_advisory_xact_lock(hashtext($1))",
		"SELECT pg_try_advisory_xact_lock($1)",
		"SELECT pg_try_advisory_xact_lock(hashtext($1))",
	}
	calls := drv.Calls()
	if len(calls) != len(want) {
		t.Fatalf("calls = %+v, want %d statements", calls, len(want))
	}
	for i, w := range want {
		if calls[i].SQL != w {
			t.Errorf("call %d = %q, want %q", i, calls[i].SQL, w)
		}
	}
	if calls[1].Args[0] != "catalog:1" || calls[0].Args[0] != int64(42) {
		t.Errorf("args = %v, %v", calls[0].Args, calls[1].Args)
	}
}

func TestAdvisoryXactLockRefusals(t *testing.T) {
	ctx := context.Background()
	db := momo.New(fakedriver.New(dialect.Postgres))
	if err := db.AdvisoryXactLock(ctx, 1); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("AdvisoryXactLock outside a tx = %v, want ErrValidation", err)
	}
	if err := db.AdvisoryXactLockString(ctx, "k"); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("AdvisoryXactLockString outside a tx = %v, want ErrValidation", err)
	}
	if _, err := db.TryAdvisoryXactLock(ctx, 1); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("TryAdvisoryXactLock outside a tx = %v, want ErrValidation", err)
	}
	if _, err := db.TryAdvisoryXactLockString(ctx, "k"); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("TryAdvisoryXactLockString outside a tx = %v, want ErrValidation", err)
	}

	for _, n := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		other := momo.New(fakedriver.New(n))
		err := other.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if err := tx.AdvisoryXactLock(ctx, 1); !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s AdvisoryXactLock = %v, want ErrUnsupported", n, err)
			}
			if _, err := tx.TryAdvisoryXactLockString(ctx, "k"); !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s TryAdvisoryXactLockString = %v, want ErrUnsupported", n, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
