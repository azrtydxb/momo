// SPDX-License-Identifier: Apache-2.0

package sqlshared

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
)

// Catches: a NaN float (plain or through a pointer) reaching SQLite, which
// would store NULL, and an overbroad check that rejects Inf, a nil pointer or
// an ordinary value.
func TestCheckArgsNaN(t *testing.T) {
	nan32, nan64 := float32(math.NaN()), math.NaN()
	var nilp *float64
	one := 1.5
	for name, bad := range map[string]any{
		"float64": math.NaN(), "float32": float32(math.NaN()), "*float64": &nan64, "*float32": &nan32,
	} {
		err := checkArgs("exec", []any{"x", bad})
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		} else if !strings.Contains(err.Error(), "argument 2") {
			t.Errorf("%s: error %q does not name the argument", name, err)
		}
	}
	if err := checkArgs("exec", []any{math.Inf(1), float32(math.Inf(-1)), nilp, &one, 3, "NaN", nil}); err != nil {
		t.Errorf("legal arguments rejected: %v", err)
	}
}
