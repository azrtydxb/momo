// SPDX-License-Identifier: Apache-2.0

// Package floatfmt writes a float32 as SQL text. It imports nothing from momo.
package floatfmt

import (
	"math"
	"strconv"
)

// Float32 is the shortest text that reads back as x with strconv's 'g' form.
// The largest float32 is the exception: its shortest text, 3.4028235e+38, is
// above the largest float32 when a server reads it as a double (MariaDB
// answers error 1292), so it is written with one digit fewer, which still
// rounds to the same float32.
func Float32(x float32) string {
	switch x {
	case math.MaxFloat32:
		return "3.4028234e+38"
	case -math.MaxFloat32:
		return "-3.4028234e+38"
	}
	return strconv.FormatFloat(float64(x), 'g', -1, 32)
}
