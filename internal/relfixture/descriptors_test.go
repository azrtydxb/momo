// SPDX-License-Identifier: Apache-2.0

package relfixture_test

import (
	"reflect"
	"testing"

	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/employee"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/store"
)

func TestFixtureNullKeysAreNotMatched(t *testing.T) {
	if _, ok := document.Author.ParentKey(&models.Document{}); ok {
		t.Fatal("document with NULL author_id produced a key")
	}
	if _, ok := employee.Manager.ParentKey(&models.Employee{}); ok {
		t.Fatal("employee with NULL manager_id produced a key")
	}
	if _, ok := store.Region.ParentKey(&models.Store{TenantID: 1}); ok {
		t.Fatal("store with NULL region_code produced a key")
	}
	code := "EU"
	k, ok := store.Region.ParentKey(&models.Store{TenantID: 1, RegionCode: &code})
	if !ok || !reflect.DeepEqual(k, []any{int64(1), "EU"}) {
		t.Fatalf("composite key = %v, %v", k, ok)
	}
}

func TestFixtureMetaPtrAndValueAgree(t *testing.T) {
	d := &models.Document{ID: 7, Title: "t"}
	m := d.MomoMeta()
	if len(m.Columns) != 3 || m.Columns[1].Flags == 0 {
		t.Fatalf("document columns = %+v", m.Columns)
	}
	if got := *(m.Ptr(d, 0).(*int64)); got != 7 {
		t.Fatalf("Ptr(0) = %d", got)
	}
	if m.Value(d, 2) != "t" {
		t.Fatalf("Value(2) = %v", m.Value(d, 2))
	}
}
