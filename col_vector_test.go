// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

func TestVectorColumnBasics(t *testing.T) {
	pg := dialect.For(dialect.Postgres)
	col := NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	if col.ColumnName() != "emb" || col.Dim() != 3 {
		t.Fatalf("name/dim = %s/%d", col.ColumnName(), col.Dim())
	}
	c, err := compile(pg, col.As("d").IsNull().AppendSQL)
	if err != nil || c.SQL != `"d"."emb" IS NULL` {
		t.Fatalf("IsNull = %q, %v", c.SQL, err)
	}
	if a := col.Set(vector.Vector{1, 2, 3}); a.ColumnName() != "emb" {
		t.Fatalf("Set column = %s", a.ColumnName())
	}
}
