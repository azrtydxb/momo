// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
)

// TestLitFloat32LimitStaysInRange: the shortest text that round-trips the
// largest float32, 3.4028235e+38, is above the largest float32 when a server
// reads it as a double, and MariaDB refuses such a value in a FLOAT context
// (error 1292). An inlined float32 literal must stay within the range.
//
// Catches: a float32 literal written with strconv's shortest form, which
// prints math.MaxFloat32 one digit too high.
func TestLitFloat32LimitStaysInRange(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		for _, x := range []float32{math.MaxFloat32, -math.MaxFloat32} {
			got, err := exprtest.CompileColumns(d, momo.Lit(x))
			if err != nil {
				t.Fatalf("%s %v: %v", d, x, err)
			}
			text := strings.TrimPrefix(got.SQL, "SELECT ")
			text, _, _ = strings.Cut(text, " FROM ")
			text = strings.Trim(text, "()")
			f, err := strconv.ParseFloat(text, 64)
			if err != nil {
				t.Fatalf("%s %v: literal %q: %v", d, x, text, err)
			}
			if math.Abs(f) > math.MaxFloat32 {
				t.Errorf("%s %v: literal %s is above math.MaxFloat32 as a double", d, x, text)
			}
			if float32(f) != x {
				t.Errorf("%s %v: literal %s reads back as %v", d, x, text, float32(f))
			}
		}
	}
}
