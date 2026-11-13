// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlfn_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/sqlfn"
	"github.com/azrtydxb/momo/types"
)

// Ledger holds a running cost in a numeric(12,6) column.
type Ledger struct {
	Name string
	Cost types.Numeric
}

var (
	ledgerName = momo.NewColumn[string]("ledgers", "name", momo.FlagPK)
	ledgerCost = momo.NewColumn[types.Numeric]("ledgers", "cost", 0)
)

var ledgerMeta = &momo.Meta[Ledger]{
	Table: "ledgers",
	Columns: []momo.ColumnMeta{
		{Name: "name", Flags: ledgerName.Flags()},
		{Name: "cost", Flags: ledgerCost.Flags()},
	},
	Ptr:      func(m *Ledger, i int) any { return [...]any{&m.Name, &m.Cost}[i] },
	Value:    func(m *Ledger, i int) any { return [...]any{m.Name, m.Cost}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*Ledger) MomoMeta() *momo.Meta[Ledger] { return ledgerMeta }

// Catches: arithmetic on a numeric column that errors on PostgreSQL (a text
// parameter against numeric), loses exactness through float8, or computes the
// wrong value for Add, Sub, Mul or Div.
func TestNumericArithmeticExactOnPostgres(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	for _, s := range []string{
		"DROP TABLE IF EXISTS ledgers",
		"CREATE TABLE ledgers (name text primary key, cost numeric(12,6) not null)",
		"INSERT INTO ledgers (name, cost) VALUES ('a', 0.1)",
	} {
		if _, err := db.Driver().Exec(ctx, s, nil); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	step := func(e momo.Expression[types.Numeric], want string) {
		t.Helper()
		if _, err := momo.Update[Ledger](db).Set(ledgerCost.SetExpr(e)).Where(ledgerName.Eq("a")).Exec(ctx); err != nil {
			t.Fatalf("update to %s: %v", want, err)
		}
		got, err := momo.Select[Ledger](db).Where(ledgerName.Eq("a")).One(ctx)
		if err != nil || got.Cost.Decimal != want {
			t.Fatalf("cost = %+v, %v; want %s", got.Cost, err, want)
		}
	}
	// 0.1 + 0.000001 is exact in numeric; in float8 it is 0.10000100000000001.
	step(sqlfn.Add(ledgerCost, types.Numeric{Decimal: "0.000001"}), "0.100001")
	step(sqlfn.Sub(ledgerCost, types.Numeric{Decimal: "0.000001"}), "0.100000")
	step(sqlfn.Mul(ledgerCost, types.Numeric{Decimal: "3"}), "0.300000")
	step(sqlfn.Div(ledgerCost, types.Numeric{Decimal: "4"}), "0.075000")
}
