// SPDX-License-Identifier: Apache-2.0

package floatfmt

import (
	"math"
	"strconv"
	"testing"
)

// Catches: a text form that reads back as another float32, or one above the
// largest float32 when read as a double.
func TestFloat32(t *testing.T) {
	for _, x := range []float32{math.MaxFloat32, -math.MaxFloat32, 0, 1, -2.5, 0.1, math.SmallestNonzeroFloat32, math.Float32frombits(0x00800000)} {
		s := Float32(x)
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Fatalf("%v: %q: %v", x, s, err)
		}
		if math.Abs(f) > math.MaxFloat32 {
			t.Errorf("%v: %q is above math.MaxFloat32 as a double", x, s)
		}
		if float32(f) != x {
			t.Errorf("%v: %q reads back as %v", x, s, float32(f))
		}
	}
	if got := Float32(math.MaxFloat32); got != "3.4028234e+38" {
		t.Errorf("Float32(MaxFloat32) = %q", got)
	}
}
