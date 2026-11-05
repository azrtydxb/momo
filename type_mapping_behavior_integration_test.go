// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/types"
)

// TestTypeMappingDocumentedBehaviors pins the behaviors docs/TYPE_MAPPING.md
// states in prose beyond the per-row round trips.
func TestTypeMappingDocumentedBehaviors(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, "CREATE TABLE tm_beh (id int PRIMARY KEY, b bytea, j jsonb, d date)"); err != nil {
		t.Fatal(err)
	}
	var nilBytes []byte
	ist := time.FixedZone("IST", 5*3600+1800)
	if _, err := momo.Exec(ctx, db, "INSERT INTO tm_beh VALUES (1, ?, ?, ?)",
		nilBytes, types.JSON[[]string]{V: nil}, types.Date{Time: time.Date(2026, 3, 1, 23, 0, 0, 0, ist)}); err != nil {
		t.Fatal(err)
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO tm_beh VALUES (2, ?, ?, ?)",
		[]byte{}, types.JSON[[]string]{V: []string{}}, types.Date{Time: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	nulls, err := momo.Scalars[bool](ctx, db, "SELECT b IS NULL FROM tm_beh ORDER BY id")
	if err != nil || len(nulls) != 2 || !nulls[0] || nulls[1] {
		t.Fatalf("nil/empty []byte NULL-ness = %v, %v", nulls, err)
	}
	js, err := momo.Scalars[string](ctx, db, "SELECT CAST(j AS text) FROM tm_beh ORDER BY id")
	if err != nil || len(js) != 2 || js[0] != "null" || js[1] != "[]" {
		t.Fatalf("JSON nil/empty slice = %v, %v", js, err)
	}
	ds, err := momo.Scalars[types.Date](ctx, db, "SELECT d FROM tm_beh ORDER BY id")
	if err != nil || len(ds) != 2 {
		t.Fatal(err)
	}
	// A Date keeps the year, month and day of its own location, so 23:00 at
	// +05:30 on 2026-03-01 stores 2026-03-01; it reads back at midnight UTC.
	want := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for _, d := range ds {
		if !d.Time.Equal(want) {
			t.Errorf("date read back %v, want %v", d.Time, want)
		}
	}
}
