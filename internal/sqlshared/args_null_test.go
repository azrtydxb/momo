// SPDX-License-Identifier: Apache-2.0

package sqlshared

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/types"
)

// Catches: a special Date or Numeric slipping past the SQLite and MySQL check
// inside sql.Null[...] (value or pointer), and an overbroad check that rejects
// a NULL wrapper whose special value is not Valid, or a finite one.
func TestCheckArgsSpecialInsideNull(t *testing.T) {
	nd := sql.Null[types.Date]{V: types.Date{Infinity: true}, Valid: true}
	nn := sql.Null[types.Numeric]{V: types.Numeric{NaN: true}, Valid: true}
	for name, bad := range map[string]any{
		"Null[Date]": nd, "*Null[Date]": &nd, "Null[Numeric]": nn, "*Null[Numeric]": &nn,
	} {
		err := checkArgs("exec", []any{"x", bad})
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		} else if !strings.Contains(err.Error(), "argument 2") {
			t.Errorf("%s: error %q does not name the argument", name, err)
		}
	}
	day := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)
	okDate := sql.Null[types.Date]{V: types.Date{Time: day}, Valid: true}
	okNum := sql.Null[types.Numeric]{V: types.Numeric{Decimal: "1.5"}, Valid: true}
	null := sql.Null[types.Numeric]{V: types.Numeric{NaN: true}} // not Valid: stored as NULL
	var nilp *sql.Null[types.Date]
	if err := checkArgs("exec", []any{okDate, &okDate, okNum, &okNum, null, &null, nilp}); err != nil {
		t.Errorf("legal arguments rejected: %v", err)
	}
}
