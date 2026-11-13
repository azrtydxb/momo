// SPDX-License-Identifier: Apache-2.0

package types

import (
	"math"
	"testing"
)

// Catches: NumericFromInt64 losing digits at the int64 extremes or adding a
// fraction.
func TestNumericFromInt64(t *testing.T) {
	for in, want := range map[int64]string{
		0:             "0",
		-42:           "-42",
		math.MaxInt64: "9223372036854775807",
		math.MinInt64: "-9223372036854775808",
	} {
		if got := NumericFromInt64(in); got != (Numeric{Decimal: want}) {
			t.Errorf("NumericFromInt64(%d) = %+v, want %q", in, got, want)
		}
	}
}

// Catches: NumericFromFloat64 writing an exponent or the long binary expansion
// (0.1 as 0.1000000000000000055...), or accepting NaN and infinity.
func TestNumericFromFloat64(t *testing.T) {
	for in, want := range map[float64]string{
		0.1:      "0.1",
		0.000001: "0.000001",
		-12.5:    "-12.5",
		3:        "3",
		1e21:     "1000000000000000000000",
		1e-7:     "0.0000001",
	} {
		got, err := NumericFromFloat64(in)
		if err != nil || got != (Numeric{Decimal: want}) {
			t.Errorf("NumericFromFloat64(%v) = %+v, %v; want %q", in, got, err, want)
		}
		if _, err := got.Value(); err != nil {
			t.Errorf("NumericFromFloat64(%v) result is not a valid Numeric: %v", in, err)
		}
	}
	for _, f := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got, err := NumericFromFloat64(f); err == nil {
			t.Errorf("NumericFromFloat64(%v) = %+v, want an error", f, got)
		}
	}
}
