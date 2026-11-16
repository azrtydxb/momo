// SPDX-License-Identifier: Apache-2.0

package vector

import (
	"database/sql/driver"
	"math"
	"strconv"
	"strings"
	"testing"
)

// TestVectorTextStaysInFloat32Range: a server that parses the text form as a
// double and then range-checks it against the float32 limit (MariaDB, error
// 1292) refuses a component that prints above math.MaxFloat32. The shortest
// text that round-trips the largest float32, 3.4028235e+38, is such a value.
//
// Catches: a text form whose magnitude, read as a double, exceeds the largest
// float32.
func TestVectorTextStaysInFloat32Range(t *testing.T) {
	for _, x := range []float32{math.MaxFloat32, -math.MaxFloat32, 1, -2.5, math.Float32frombits(0x00800000)} {
		for name, val := range map[string]any{
			"Vector":       mustValue(t, Vector{x, 0}),
			"SparseVector": mustValue(t, SparseVector{Dim: 2, Indices: []int32{0}, Values: []float32{x}}),
		} {
			text := val.(string)
			for _, tok := range strings.FieldsFunc(text, func(r rune) bool { return strings.ContainsRune("[],{}:/", r) }) {
				f, err := strconv.ParseFloat(tok, 64)
				if err != nil {
					continue
				}
				if math.Abs(f) > math.MaxFloat32 {
					t.Errorf("%s %v: text %q holds %s, above math.MaxFloat32 as a double", name, x, text, tok)
				}
			}
			if name == "Vector" {
				var back Vector
				if err := back.Scan(text); err != nil || back[0] != x {
					t.Errorf("%s %v: text %q scans back to %v, %v", name, x, text, back, err)
				}
			}
		}
	}
}

func mustValue(t *testing.T, v driver.Valuer) any {
	t.Helper()
	val, err := v.Value()
	if err != nil {
		t.Fatal(err)
	}
	return val
}
