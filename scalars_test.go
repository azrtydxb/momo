// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// PostgreSQL only: ::int is PostgreSQL cast syntax.
func TestScalarsRawPlaceholders(t *testing.T) {
	db := testopen.Postgres(t)
	got, err := momo.Scalars[int](context.Background(), db, "SELECT ?::int + ?", 1, 2)
	if err != nil || len(got) != 1 || got[0] != 3 {
		t.Fatalf("Scalars = %v, %v; want [3], nil", got, err)
	}
}
