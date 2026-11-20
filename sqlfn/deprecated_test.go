// SPDX-License-Identifier: Apache-2.0

//lint:file-ignore SA1019 these tests exercise the deprecated names

package sqlfn_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/sqlfn"
	"github.com/azrtydxb/momo/timeop"
)

// Catches: a deprecated sqlfn name that stops forwarding to its replacement,
// so a caller that has not migrated yet compiles different SQL (or an
// error where the replacement succeeds).
func TestDeprecatedNamesMatchReplacements(t *testing.T) {
	cases := []struct {
		name     string
		old, now momo.Expr
	}{
		{"CurrentDate", sqlfn.CurrentDate(), timeop.CurrentDate()},
		{"Infinity", sqlfn.Infinity(), timeop.Infinity()},
		{"NegInfinity", sqlfn.NegInfinity(), timeop.NegInfinity()},
		{"SubstrFrom", sqlfn.SubstrFrom(docTitle, 4), sqlfn.SubstrToEnd(docTitle, 4)},
	}
	for _, c := range cases {
		for _, d := range typedDialects {
			old, oldErr := exprtest.CompileColumns(d, c.old)
			now, nowErr := exprtest.CompileColumns(d, c.now)
			if (oldErr == nil) != (nowErr == nil) {
				t.Errorf("%s %s: deprecated err = %v, replacement err = %v", c.name, d, oldErr, nowErr)
				continue
			}
			if old.SQL != now.SQL || len(old.Args) != len(now.Args) {
				t.Errorf("%s %s: deprecated = %s %v, replacement = %s %v", c.name, d, old.SQL, old.Args, now.SQL, now.Args)
			}
		}
	}
}
