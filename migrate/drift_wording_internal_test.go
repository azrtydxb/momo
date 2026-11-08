// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"testing"

	"github.com/azrtydxb/momo/schema"
)

func driftTable(cols ...string) *schema.Schema {
	t := schema.Table{Name: "users"}
	for _, c := range cols {
		t.Columns = append(t.Columns, schema.Column{Name: c, Type: "text"})
	}
	return &schema.Schema{Tables: []schema.Table{t}}
}

// A column that exists only live is a change of its table, worded "not in
// snapshot"; a column only the snapshot has is Missing. Neither is an
// unmanaged object. This pins the report that docs/migrations.md describes.
func TestCompareReportsOneSidedColumns(t *testing.T) {
	rep := compare(driftTable("id"), driftTable("id", "extra"), false, nil)
	if len(rep.Changed) != 1 || rep.Changed[0].Object.Kind != "column" || rep.Changed[0].Object.Name != "users.extra" || rep.Changed[0].Detail != "not in snapshot" {
		t.Fatalf("Changed = %+v, want users.extra \"not in snapshot\"", rep.Changed)
	}
	if len(rep.Missing) != 0 || len(rep.Unmanaged) != 0 {
		t.Fatalf("a live-only column must be neither Missing nor Unmanaged: %+v", rep)
	}

	rep = compare(driftTable("id", "gone"), driftTable("id"), false, nil)
	if len(rep.Missing) != 1 || rep.Missing[0].Name != "users.gone" || len(rep.Changed) != 0 || len(rep.Unmanaged) != 0 {
		t.Fatalf("a snapshot-only column must be Missing alone: %+v", rep)
	}
}

// The columns of a table that is itself unmanaged are not listed again.
func TestCompareDoesNotRepeatChildrenOfUnmanagedTable(t *testing.T) {
	rep := compare(&schema.Schema{}, driftTable("id", "extra"), false, nil)
	if len(rep.Unmanaged) != 1 || rep.Unmanaged[0].Kind != "table" || len(rep.Changed) != 0 {
		t.Fatalf("want the table alone as Unmanaged, got %+v", rep)
	}
}
