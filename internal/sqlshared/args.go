// SPDX-License-Identifier: Apache-2.0

package sqlshared

import (
	"fmt"
	"math"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/types"
)

// CheckArgs is checkArgs for the MySQL adapter, which shares the Date and
// Numeric rule.
func CheckArgs(op string, args []any) error { return checkArgs(op, args) }

// checkArgs rejects a float NaN argument (float32, float64 or a non-nil
// pointer to one). SQLite has no NaN and stores a bound NaN as NULL, which
// would silently change the data; +Inf and -Inf are legal REAL values. It
// also rejects a types.Date or types.Numeric that is not finite (infinity,
// -infinity or NaN): neither SQLite nor MySQL has such a date or number, and
// the TEXT column would keep the word. The error names the argument
// position, never its value.
func checkArgs(op string, args []any) error {
	for i, a := range args {
		var nan, special bool
		switch v := a.(type) {
		case float64:
			nan = math.IsNaN(v)
		case float32:
			nan = math.IsNaN(float64(v))
		case *float64:
			nan = v != nil && math.IsNaN(*v)
		case *float32:
			nan = v != nil && math.IsNaN(float64(*v))
		case types.Date:
			special = !v.IsFinite()
		case *types.Date:
			special = v != nil && !v.IsFinite()
		case types.Numeric:
			special = !v.IsFinite()
		case *types.Numeric:
			special = v != nil && !v.IsFinite()
		}
		if special {
			return &momo.Error{Kind: momo.ErrValidation, Op: op,
				Err: fmt.Errorf("argument %d is infinity or NaN, which this database cannot store in a Date or Numeric column (PostgreSQL only)", i+1)}
		}
		if nan {
			return &momo.Error{Kind: momo.ErrValidation, Op: op,
				Err: fmt.Errorf("sqlite: argument %d is NaN, which SQLite cannot store (it would become NULL)", i+1)}
		}
	}
	return nil
}
