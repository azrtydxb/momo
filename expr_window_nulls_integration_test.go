// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/suite"
)

// Catches: momo sending a RANGE-offset window with an emulated NULLS term to
// MySQL or MariaDB, which reject it (the statement has two ORDER BY keys), or
// refusing it on PostgreSQL and SQLite where it is native and works.
func TestRangeOffsetWindowWithNullsTerm(t *testing.T) {
	parityEach(t, func(t *testing.T, db *momo.DB, name string) {
		seedParityDocs(t, db, parityDoc{1, "a", nil, 1}, parityDoc{1, "b", nil, 2}, parityDoc{1, "c", nil, 4})
		ctx := context.Background()
		mysqlFamily := strings.HasPrefix(name, "mysql") || name == "mariadb"
		w := momo.Over().OrderBy(suite.DocVersion.Asc().NullsLast()).Range(momo.Preceding(1), momo.CurrentRow())
		rows, err := momo.Select[momo.Row1[int64]](db).From(momo.TableRef{Name: "documents"}).
			Columns(momo.Sum(suite.DocVersion).Over(w)).All(ctx)
		if mysqlFamily {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("RANGE offset with NULLS LAST = %v, want ErrUnsupported", err)
			}
			// What the refusal prevents: the emulated statement is a server error.
			_, serr := momo.Exec(ctx, db, "SELECT SUM(version) OVER (ORDER BY (version IS NULL) ASC, version ASC RANGE BETWEEN 1 PRECEDING AND CURRENT ROW) FROM documents")
			if serr == nil {
				t.Fatal("the emulated two-key RANGE window ran; the guard is no longer needed")
			}
			return
		}
		if err != nil || len(rows) != 3 {
			t.Fatalf("native RANGE offset with NULLS LAST: %v, %d rows", err, len(rows))
		}
	})
}
