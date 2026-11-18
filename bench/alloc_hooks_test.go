// SPDX-License-Identifier: Apache-2.0

//go:build integration

package bench

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/bench/adapters"
	"github.com/azrtydxb/momo/driver/pgxdriver"
)

// hookedRow has an id generator and field hooks, so the statements on it plan
// the field hooks: an insert runs the id generator and a Validate hook, an
// update runs a Validate and an OnUpdate hook. The plain benchmarks of this
// package use a model with neither, so they never build a field plan.
type hookedRow struct {
	ID   string
	Name string
	Hits int64
}

var hookedMeta = momo.Meta[hookedRow]{
	Table: "bench_hooked",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "name"},
		{Name: "hits"},
	},
	Ptr:      func(m *hookedRow, i int) any { return [...]any{&m.ID, &m.Name, &m.Hits}[i] },
	Value:    func(m *hookedRow, i int) any { return [...]any{m.ID, m.Name, m.Hits}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK:     []int{0},
	IDGens: []momo.IDGen{{Column: 0, Name: "cuid"}},
	Fields: []momo.FieldHook[hookedRow]{
		{Column: 1, Validate: func(v any) error {
			if s, _ := v.(string); s == "" {
				return errors.New("name is empty")
			}
			return nil
		}},
		{Column: 2, OnUpdate: func(*hookedRow) any { return int64(1) }},
	},
}

func (*hookedRow) MomoMeta() *momo.Meta[hookedRow] { return &hookedMeta }

// hookedDB creates the table of hookedRow with one row, hooked-1, and opens a
// DB on it.
func hookedDB(b *testing.B) *momo.DB {
	b.Helper()
	url := os.Getenv("BENCH_PG_URL")
	if url == "" {
		b.Skip("BENCH_PG_URL is not set")
	}
	ctx := context.Background()
	pool, err := adapters.NewPool(ctx, url)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(pool.Close)
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS bench_hooked",
		"CREATE TABLE bench_hooked (id text PRIMARY KEY, name text NOT NULL, hits bigint NOT NULL DEFAULT 0)",
		"INSERT INTO bench_hooked (id, name) VALUES ('hooked-1', 'seed')",
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			b.Fatal(err)
		}
	}
	return momo.New(pgxdriver.New(pool))
}

// BenchmarkAllocs_InsertHooked is an insert of one model that has an id
// generator and a Validate hook.
func BenchmarkAllocs_InsertHooked(b *testing.B) {
	db := hookedDB(b)
	ctx := context.Background()
	b.Run("momo", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			row := hookedRow{Name: "n"}
			if _, err := momo.Insert[hookedRow](db).Model(&row).Exec(ctx); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkAllocs_UpdateHooked is an update of the same model: a Validate hook
// on the column it sets and an OnUpdate hook on another.
func BenchmarkAllocs_UpdateHooked(b *testing.B) {
	db := hookedDB(b)
	ctx := context.Background()
	name := momo.NewColumn[string]("bench_hooked", "name", 0)
	id := momo.NewColumn[string]("bench_hooked", "id", momo.FlagPK)
	b.Run("momo", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := momo.Update[hookedRow](db).Set(name.Set("renamed")).Where(id.Eq("hooked-1")).Exec(ctx); err != nil {
				b.Fatal(err)
			}
		}
	})
}
