// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
)

// Catches: EqPtr(nil) rendering "= NULL" (matches no row) or binding a nil
// pointer, and SetPtr(nil) binding a typed nil instead of writing NULL.
var bodyIsNull = regexp.MustCompile("body.? IS NULL")

func TestNullColumnPtrSQL(t *testing.T) {
	s := "x"
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		db := stateDB(d)

		c, err := momo.Select[suite.Document](db).Where(suite.DocBody.EqPtr(nil)).SQL()
		if err != nil || !bodyIsNull.MatchString(c.SQL) ||
			strings.Contains(c.SQL, "= NULL") || len(c.Args) != 0 {
			t.Errorf("%s EqPtr(nil): %q %v %v", d, c.SQL, c.Args, err)
		}

		c, err = momo.Select[suite.Document](db).Where(suite.DocBody.EqPtr(&s)).SQL()
		if err != nil || bodyIsNull.MatchString(c.SQL) || len(c.Args) != 1 || c.Args[0] != "x" {
			t.Errorf("%s EqPtr(&x): %q %v %v", d, c.SQL, c.Args, err)
		}

		c, err = momo.Update[suite.Document](db).Set(suite.DocBody.SetPtr(nil)).Where(suite.DocID.Eq(1)).SQL()
		if err != nil || !strings.Contains(c.SQL, "= NULL") || len(c.Args) != 2 {
			t.Errorf("%s SetPtr(nil): %q %v %v", d, c.SQL, c.Args, err)
		}

		c, err = momo.Update[suite.Document](db).Set(suite.DocBody.SetPtr(&s)).Where(suite.DocID.Eq(1)).SQL()
		if err != nil || len(c.Args) != 3 || c.Args[0] != "x" {
			t.Errorf("%s SetPtr(&x): %q %v %v", d, c.SQL, c.Args, err)
		}

		c, err = momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1), suite.DocBody.SetPtr(nil)).SQL()
		if err != nil || !strings.Contains(c.SQL, "NULL") {
			t.Errorf("%s Values SetPtr(nil): %q %v", d, c.SQL, err)
		}
	}
}
