// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a sequence change rendered as empty or invalid SQL on SQLite
// instead of being refused with ErrUnsupported.
func TestSQLiteRenderRefusesSequences(t *testing.T) {
	q := schema.Sequence{Name: "jobs_seq"}
	for _, c := range []migrate.Change{
		{Kind: migrate.CreateSequence, Name: "jobs_seq", After: &q},
		{Kind: migrate.AlterSequence, Name: "jobs_seq", Before: &q, After: &q},
		{Kind: migrate.DropSequence, Name: "jobs_seq", Before: &q},
	} {
		stmts, err := Dialect{}.Render([]migrate.Change{c})
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: want ErrUnsupported, got stmts %+v err %v", c.Kind, stmts, err)
		}
	}
}
