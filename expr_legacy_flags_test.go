// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
)

// legacyUpper is upper(src) written against the extension API before the
// column flags had a type: its Flags method returns a uint16.
type legacyUpper struct {
	momo.Ops[string]
	src momo.Expr
}

func legacyUpperOf(src momo.Expr) legacyUpper {
	u := legacyUpper{src: src}
	u.Ops = momo.NewOps[string](u)
	return u
}

func (u legacyUpper) AppendSQL(b *momo.Builder) {
	b.WriteString("upper(")
	u.src.AppendSQL(b)
	b.WriteString(")")
}

func (u legacyUpper) Flags() uint16 {
	if f, ok := u.src.(momo.Flagged); ok {
		return uint16(f.Flags())
	}
	return 0
}

// Catches: a wrapper that still returns Flags() uint16 compiling after the
// flags got a type but no longer being recognised, so Ops binds the value
// compared with a sensitive column in plaintext (fail-open masking).
func TestLegacyUint16FlagsWrapperStaysMasked(t *testing.T) {
	for _, sensitive := range []bool{true, false} {
		t.Run(fmt.Sprintf("sensitive=%v", sensitive), func(t *testing.T) {
			var flags momo.ColumnFlags
			if sensitive {
				flags = momo.FlagSensitive
			}
			col := momo.NewColumn[string]("documents", "secret", flags)
			rec := &obstest.Recorder{}
			drv := fakedriver.New(dialect.Postgres)
			db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
			if _, err := momo.Select[suite.Document](db).Where(legacyUpperOf(col).Eq("pw")).All(t.Context()); err != nil {
				t.Fatal(err)
			}
			evs, calls := rec.Events(), drv.Calls()
			if len(evs) == 0 || len(calls) == 0 {
				t.Fatal("no statement recorded")
			}
			seen, sent := evs[len(evs)-1].Args, calls[len(calls)-1].Args
			if len(seen) != 1 || len(sent) != 1 {
				t.Fatalf("want one argument, hook saw %v, driver got %v", seen, sent)
			}
			if sent[0] == momo.RedactedArg {
				t.Fatal("the driver received the placeholder")
			}
			if got := seen[0] == momo.RedactedArg; got != sensitive {
				t.Fatalf("hook saw %v, masked = %v, want masked = %v", seen[0], got, sensitive)
			}
		})
	}
}
