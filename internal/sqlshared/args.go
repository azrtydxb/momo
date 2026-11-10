// SPDX-License-Identifier: Apache-2.0

package sqlshared

import (
	"fmt"
	"math"

	"github.com/azrtydxb/momo"
)

// checkArgs rejects a float NaN argument (float32, float64 or a non-nil
// pointer to one). SQLite has no NaN and stores a bound NaN as NULL, which
// would silently change the data; +Inf and -Inf are legal REAL values. The
// error names the argument position, never its value.
func checkArgs(op string, args []any) error {
	for i, a := range args {
		var nan bool
		switch v := a.(type) {
		case float64:
			nan = math.IsNaN(v)
		case float32:
			nan = math.IsNaN(float64(v))
		case *float64:
			nan = v != nil && math.IsNaN(*v)
		case *float32:
			nan = v != nil && math.IsNaN(float64(*v))
		}
		if nan {
			return &momo.Error{Kind: momo.ErrValidation, Op: op,
				Err: fmt.Errorf("sqlite: argument %d is NaN, which SQLite cannot store (it would become NULL)", i+1)}
		}
	}
	return nil
}
