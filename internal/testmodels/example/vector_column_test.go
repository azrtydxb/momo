// SPDX-License-Identifier: Apache-2.0

package example_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testmodels/example"
	"github.com/azrtydxb/momo/internal/testmodels/example/document"
	"github.com/azrtydxb/momo/vecop"
	"github.com/azrtydxb/momo/vector"
)

// TestGeneratedVectorColumnDistance proves the generated descriptor of a
// vector field is a momo.VectorColumn: the distance functions accept it
// without a hand-written NewVectorColumn, and it carries the tagged dim.
func TestGeneratedVectorColumnDistance(t *testing.T) {
	var col momo.VectorColumn[vector.Vector] = document.Embedding
	if got := col.Dim(); got != 1536 {
		t.Fatalf("Dim() = %d, want 1536", got)
	}
	var _ momo.ColumnRef = document.Embedding
	if got := document.As("d").Embedding.ColumnName(); got != "embedding" {
		t.Fatalf("ColumnName() = %q", got)
	}

	db := momo.New(pgxdriver.New(nil))
	c, err := momo.Select[example.Document, *example.Document](db).
		OrderBy(vecop.Cosine(document.Embedding, make(vector.Vector, 1536)).Asc()).
		SQL()
	if err != nil {
		t.Fatal(err)
	}
	if c.SQL == "" {
		t.Fatal("empty SQL")
	}
	_, err = momo.Select[example.Document, *example.Document](db).
		OrderBy(vecop.Cosine(document.Embedding, vector.Vector{1, 2, 3}).Asc()).
		SQL()
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("wrong-sized query vector error = %v, want ErrValidation", err)
	}
}
