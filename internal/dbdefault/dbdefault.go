// SPDX-License-Identifier: Apache-2.0

// Package dbdefault classifies column defaults for the dbdefault tag: it tells
// the generator and momo generate --from-db when a Go zero value (false, 0)
// would differ from the database default and so needs a pointer field.
package dbdefault

import (
	"strconv"
	"strings"
)

// Scalar reports whether goType is a Go bool or number type, the types whose
// zero value is a meaningful value (false, 0) rather than "unset".
func Scalar(goType string) bool {
	switch goType {
	case "bool", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64":
		return true
	}
	return false
}

// NonZeroConst reports whether def is a constant default that is true or a
// non-zero number, for a column of the Go type goType (a bool or number type;
// see Scalar). A computed default (a function call), an empty default and a
// default it cannot read are not constants, so they report false.
func NonZeroConst(goType, def string) bool {
	if !Scalar(goType) {
		return false
	}
	d := strings.ToLower(strings.TrimSpace(def))
	if i := strings.Index(d, "::"); i >= 0 {
		d = strings.TrimSpace(d[:i])
	}
	for len(d) >= 2 && d[0] == '(' && d[len(d)-1] == ')' {
		d = strings.TrimSpace(d[1 : len(d)-1])
	}
	d = strings.Trim(d, "'\"")
	if goType == "bool" {
		return d == "true" || d == "1" || d == "t" || d == "y" || d == "yes" || d == "on"
	}
	f, err := strconv.ParseFloat(d, 64)
	return err == nil && f != 0
}
