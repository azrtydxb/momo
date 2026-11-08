// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

// Catches: Copy skipping Go-side id generation, which streamed empty ids for a
// model with a cuid key, or wrote the ids back into the caller's slice.
func TestCopyGeneratesGoSideIDs(t *testing.T) {
	drv := newCopyDriver(dialect.Postgres)
	db := momo.New(drv)
	ms := []idRow{{Name: "a"}, {Name: "b"}, {ID: "keep", Name: "c"}}
	res, err := momo.Insert[idRow](db).Copy(t.Context(), ms)
	if err != nil || res.RowsAffected != 3 {
		t.Fatalf("Copy = %+v, %v", res, err)
	}
	if !slices.Equal(drv.log.cols, []string{"id", "name"}) || len(drv.log.rows) != 3 {
		t.Fatalf("cols %v rows %v", drv.log.cols, drv.log.rows)
	}
	seen := map[string]bool{}
	for i, r := range drv.log.rows[:2] {
		id, _ := r[0].(string)
		if !reCUID.MatchString(id) || seen[id] {
			t.Errorf("row %d id = %q, want a fresh cuid", i, r[0])
		}
		seen[id] = true
	}
	if drv.log.rows[2][0] != "keep" {
		t.Errorf("a preset id was replaced: %v", drv.log.rows[2][0])
	}
	if ms[0].ID != "" || ms[1].ID != "" {
		t.Errorf("Copy wrote ids into the caller's slice: %+v", ms)
	}
}

// copyVecRow has a nullable vector field.
type copyVecRow struct {
	ID  int64
	Emb *vector.Vector
}

var copyVecRowMeta = momo.Meta[copyVecRow]{
	Table:    "copy_vec_rows",
	Columns:  []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "emb"}},
	Ptr:      func(m *copyVecRow, i int) any { return [...]any{&m.ID, &m.Emb}[i] },
	Value:    func(m *copyVecRow, i int) any { return [...]any{m.ID, m.Emb}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*copyVecRow) MomoMeta() *momo.Meta[copyVecRow] { return &copyVecRowMeta }

// Catches: a nil *vector.Vector field panicking in DialectValue while Copy
// encodes its rows; it must stream as NULL.
func TestCopyNilVectorPointerStreamsNull(t *testing.T) {
	drv := newCopyDriver(dialect.Postgres)
	v := vector.Vector{1, 2}
	ms := []copyVecRow{{ID: 1}, {ID: 2, Emb: &v}}
	if _, err := momo.Insert[copyVecRow](momo.New(drv)).Copy(t.Context(), ms); err != nil {
		t.Fatal(err)
	}
	if len(drv.log.rows) != 2 || drv.log.rows[0][1] != nil || drv.log.rows[1][1] == nil {
		t.Fatalf("rows = %#v, want NULL then an encoded vector", drv.log.rows)
	}
}
