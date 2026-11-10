// SPDX-License-Identifier: Apache-2.0

package momotest_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/momotest"
)

// recTB records failures instead of failing the real test. Fatalf also panics
// (see runAssert) so the code after it does not run, as with a real TB.
type recTB struct {
	testing.TB
	errs    []string
	helpers int
}

func (r *recTB) Helper() { r.helpers++ }

func (r *recTB) Errorf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

type fatalSentinel struct{}

func (r *recTB) Fatalf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
	panic(fatalSentinel{})
}

// runAssert runs fn and absorbs the panic a recTB.Fatalf raises.
func runAssert(fn func()) {
	defer func() {
		if p := recover(); p != nil {
			if _, ok := p.(fatalSentinel); !ok {
				panic(p)
			}
		}
	}()
	fn()
}

func TestMomotestSQLAssert(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	c, err := momo.Select[models.Author](db).Where(author.Name.Eq("hunter2")).SQL()
	if err != nil {
		t.Fatal(err)
	}

	good := &recTB{TB: t}
	momotest.AssertSQL(good, c, strings.ReplaceAll(c.SQL, " FROM ", "\n\tFROM "))
	if len(good.errs) != 0 {
		t.Fatalf("whitespace-only difference failed: %v", good.errs)
	}
	if good.helpers == 0 {
		t.Fatal("AssertSQL did not call tb.Helper")
	}

	bad := &recTB{TB: t}
	momotest.AssertSQL(bad, c, c.SQL+" LIMIT 1")
	if len(bad.errs) != 1 {
		t.Fatalf("mismatch reported %d errors, want 1", len(bad.errs))
	}
	msg := bad.errs[0]
	if !strings.Contains(msg, "SQL mismatch") || !strings.Contains(msg, c.SQL) || !strings.Contains(msg, c.SQL+" LIMIT 1") || !strings.Contains(msg, "args: 1 bound") {
		t.Fatalf("message lacks both SQL texts or the argument count: %s", msg)
	}
	if strings.Contains(msg, "hunter2") {
		t.Fatalf("message leaks an argument value: %s", msg)
	}
	if n := len(drv.Calls()); n != 0 {
		t.Fatalf("AssertSQL path executed %d statements; it must only compile", n)
	}

	secret := momo.NewColumn[string]("authors", "name", momo.FlagSensitive)
	sc, err := momo.Select[models.Author](db).Where(secret.Eq("hunter2")).SQL()
	if err != nil {
		t.Fatal(err)
	}
	sbad := &recTB{TB: t}
	momotest.AssertSQL(sbad, sc, sc.SQL+" LIMIT 1")
	if len(sbad.errs) != 1 || !strings.Contains(sbad.errs[0], momo.RedactedArg) || strings.Contains(sbad.errs[0], "hunter2") {
		t.Fatalf("sensitive dump must show %s and never the value: %v", momo.RedactedArg, sbad.errs)
	}
}

// TestMomotestSQLAssertInsert: the helper compares any statement a builder
// compiles, an INSERT and a raw subquery included.
func TestMomotestSQLAssertInsert(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	ins, err := momo.Insert[models.Author](db).Model(&models.Author{ID: 5, Name: "hunter2"}).SQL()
	if err != nil {
		t.Fatal(err)
	}
	ok := &recTB{TB: t}
	momotest.AssertSQL(ok, ins, "  "+strings.Join(strings.Fields(ins.SQL), "  "))
	if len(ok.errs) != 0 {
		t.Fatalf("insert with reformatted whitespace failed: %v", ok.errs)
	}
	bad := &recTB{TB: t}
	momotest.AssertSQL(bad, ins, "INSERT INTO other DEFAULT VALUES")
	if len(bad.errs) != 1 || strings.Contains(bad.errs[0], "hunter2") {
		t.Fatalf("insert mismatch: %v", bad.errs)
	}

	sub := momo.RawSubquery("SELECT id FROM authors WHERE name = ?", "hunter2")
	c, err := momo.Select[models.Author](db).From(momo.FromSubquery(sub, "a")).SQL()
	if err != nil {
		t.Fatal(err)
	}
	bad2 := &recTB{TB: t}
	momotest.AssertSQL(bad2, c, "SELECT 1")
	if len(bad2.errs) != 1 || strings.Contains(bad2.errs[0], "hunter2") {
		t.Fatalf("raw subquery mismatch leaks or is silent: %v", bad2.errs)
	}
}
