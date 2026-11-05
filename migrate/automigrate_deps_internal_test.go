// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

func TestSkipDependentsKinds(t *testing.T) {
	skip := []Skipped{
		{Change: Change{Kind: CreateEnum, Name: "mood"}, Reason: "x"},
		{Change: Change{Kind: CreateTable, Table: "gone", Name: "gone"}, Reason: "x"},
		{Change: Change{Kind: AddColumn, Table: "things", Name: "must"}, Reason: "x"},
	}
	kept := []Change{
		{Kind: CreateTable, Table: "feels", Name: "feels", After: &schema.Table{Name: "feels", Columns: []schema.Column{{Name: "m", Type: "mood", Enum: "mood"}}}},
		{Kind: SetRLS, Table: "gone", Name: "gone"},
		{Kind: CreatePolicy, Table: "gone", Name: "p"},
		{Kind: AddFK, Table: "pets", Name: "fk", After: &schema.ForeignKey{Columns: []string{"o"}, RefTable: "gone", RefColumns: []string{"id"}}},
		{Kind: CreateIndex, Table: "things", Name: "i", After: &schema.Index{Parts: []schema.IndexPart{{Column: "must"}}}},
		{Kind: CreateIndex, Table: "things", Name: "ok", After: &schema.Index{Parts: []schema.IndexPart{{Column: "fine"}}}},
		// Transitive: the column's table add was skipped, then the index on the column follows.
		{Kind: CreateIndex, Table: "pets", Name: "ci", After: &schema.Index{Parts: []schema.IndexPart{{Column: "o"}}}},
		{Kind: AddColumn, Table: "pets", Name: "o", After: &schema.Column{Name: "o", Type: "int", Nullable: true, Enum: "mood"}},
	}
	got, sk := skipDependents(kept, skip)
	if len(got) != 1 || got[0].Name != "ok" {
		t.Fatalf("kept: %+v", got)
	}
	if len(sk) != len(skip)+len(kept)-1 {
		t.Fatalf("skipped: %d", len(sk))
	}
	for _, s := range sk[len(skip):] {
		if !strings.HasPrefix(s.Reason, "depends on a skipped change: ") {
			t.Fatalf("reason: %q", s.Reason)
		}
	}
}
