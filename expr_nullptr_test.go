// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
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

// Catches: SetPtr dropping the nil case inside an upsert DoUpdate (NULL must be
// written, not a bound nil), and a non-nil SetPtr in Values not binding its value.
func TestNullColumnPtrUpsertAndValuesSQL(t *testing.T) {
	s := "x"
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		db := stateDB(d)

		c, err := momo.Upsert[suite.Document](db).
			Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("t")).
			OnConflict(suite.DocTitle).DoUpdate(suite.DocBody.SetPtr(nil)).SQL()
		if err != nil || !strings.Contains(c.SQL, "= NULL") {
			t.Errorf("%s DoUpdate SetPtr(nil): %q %v", d, c.SQL, err)
		}

		c, err = momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1), suite.DocBody.SetPtr(&s)).SQL()
		if err != nil || !containsArg(c.Args, "x") {
			t.Errorf("%s Values SetPtr(&x): %q %v %v", d, c.SQL, c.Args, err)
		}
	}
}

// Catches: SetPtr or EqPtr losing the column flags, so a sensitive nullable
// column's value reaches a hook event unmasked.
func TestNullColumnPtrSensitiveIsRedacted(t *testing.T) {
	const secret = "s3cret-ptr-value"
	col := momo.NewNullColumn[string]("documents", "secret", momo.FlagSensitive)
	v := secret
	cases := map[string]func(db *momo.DB) error{
		"SetPtr": func(db *momo.DB) error {
			_, err := momo.Update[suite.Document](db).Set(col.SetPtr(&v)).Where(suite.DocID.Eq(1)).Exec(t.Context())
			return err
		},
		"EqPtr": func(db *momo.DB) error {
			_, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("t")).Where(col.EqPtr(&v)).Exec(t.Context())
			return err
		},
	}
	for name, run := range cases {
		rec := &obstest.Recorder{}
		db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(rec), momo.WithArgLogging())
		if err := run(db); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		evs := rec.Events()
		if len(evs) == 0 {
			t.Fatalf("%s: no hook events", name)
		}
		for _, e := range evs {
			if strings.Contains(fmt.Sprint(e.Args), secret) || strings.Contains(e.SQL, secret) {
				t.Errorf("%s: event shows the sensitive value: %v", name, e.Args)
			}
		}
		if got := evs[len(evs)-1].Args; !containsArg(got, momo.RedactedArg) {
			t.Errorf("%s: event args = %v, want momo.RedactedArg", name, got)
		}
	}
}
