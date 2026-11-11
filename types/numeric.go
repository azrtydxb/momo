// SPDX-License-Identifier: Apache-2.0

package types

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// Numeric is an exact decimal number: numeric on PostgreSQL, TEXT on SQLite
// and MySQL. The digits live in a string, so no precision is lost to binary
// floating point and Momo needs no decimal library.
//
// Decimal holds a finite value such as "-12.50" or "0.000000001"; empty means
// 0. PostgreSQL numeric also holds NaN and the infinities: set NaN, Infinity
// or NegInfinity (at most one, with Decimal empty) to write one, and Scan sets
// the flag when it reads one. SQLite and MySQL have no such values: Momo
// rejects a Numeric with a flag set there with ErrValidation. IsFinite
// reports whether the value is an ordinary number.
//
// NaN has been a numeric value for a long time, but Infinity and -Infinity
// need PostgreSQL 14 or later; Momo's CI covers PostgreSQL 16 and 17. An older
// server rejects them with a database error.
//
// Numeric does no arithmetic. Use a decimal library for that, or let the
// database do it. Scan does not accept a pgtype.Numeric; read the column as
// Numeric, or convert with its String form.
type Numeric struct {
	Decimal     string
	NaN         bool
	Infinity    bool
	NegInfinity bool
}

// ParseNumeric parses PostgreSQL numeric text: an optionally signed decimal
// ("12", "-1.5", ".5", "7."), or NaN, Infinity or -Infinity (also written
// inf, any case). Surrounding spaces are ignored; an exponent is not accepted.
func ParseNumeric(s string) (Numeric, error) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "nan":
		return Numeric{NaN: true}, nil
	case "infinity", "+infinity", "inf", "+inf":
		return Numeric{Infinity: true}, nil
	case "-infinity", "-inf":
		return Numeric{NegInfinity: true}, nil
	}
	if !validDecimal(s) {
		return Numeric{}, fmt.Errorf("types: invalid Numeric %q", s)
	}
	return Numeric{Decimal: s}, nil
}

// validDecimal reports whether s is [sign] digits [. digits] with at least one
// digit.
func validDecimal(s string) bool {
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		s = s[1:]
	}
	digits, dot := 0, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= '0' && c <= '9':
			digits++
		case c == '.' && !dot:
			dot = true
		default:
			return false
		}
	}
	return digits > 0
}

// IsNaN reports whether n is NaN.
func (n Numeric) IsNaN() bool { return n.NaN }

// IsInfinite reports whether n is Infinity or -Infinity.
func (n Numeric) IsInfinite() bool { return n.Infinity || n.NegInfinity }

// IsFinite reports whether n is an ordinary number: not NaN and not infinite.
func (n Numeric) IsFinite() bool { return !n.NaN && !n.Infinity && !n.NegInfinity }

// Sign returns -1, 0 or +1 for a negative, zero or positive n. Infinity is +1,
// -Infinity is -1 and NaN is 0.
func (n Numeric) Sign() int {
	switch {
	case n.Infinity:
		return 1
	case n.NegInfinity:
		return -1
	case n.NaN:
		return 0
	}
	d := n.Decimal
	for i := 0; i < len(d); i++ {
		if d[i] >= '1' && d[i] <= '9' {
			if d[0] == '-' {
				return -1
			}
			return 1
		}
	}
	return 0
}

// String returns the PostgreSQL text form: the digits, or NaN, Infinity or
// -Infinity.
func (n Numeric) String() string {
	switch {
	case n.NaN:
		return "NaN"
	case n.Infinity:
		return "Infinity"
	case n.NegInfinity:
		return "-Infinity"
	case n.Decimal == "":
		return "0"
	}
	return n.Decimal
}

// SQLType returns the column type for d.
func (Numeric) SQLType(d dialect.Name) string {
	if d == dialect.Postgres {
		return "numeric"
	}
	return "TEXT"
}

// Value encodes n as its text form. It is an error to set more than one flag,
// to set a flag together with Decimal, or to leave in Decimal anything that is
// not a decimal number.
func (n Numeric) Value() (driver.Value, error) {
	flags := 0
	for _, f := range []bool{n.NaN, n.Infinity, n.NegInfinity} {
		if f {
			flags++
		}
	}
	switch {
	case flags > 1:
		return nil, errors.New("types: Numeric has more than one of NaN, Infinity and NegInfinity set")
	case flags == 1 && n.Decimal != "":
		return nil, errors.New("types: Numeric has both Decimal and a NaN or infinity flag set")
	case flags == 0 && n.Decimal != "" && !validDecimal(n.Decimal):
		return nil, fmt.Errorf("types: invalid Numeric %q", n.Decimal)
	}
	return n.String(), nil
}

// Scan accepts the text form as string or []byte, an int64, or a float64 (what
// SQLite returns for an aggregate or an expression). A float64 is written in
// plain decimal form without exponent, with the shortest digits that read back
// the same; NaN and the infinities set their flags. Scan(nil) returns an
// error: use a *Numeric for a nullable column.
func (n *Numeric) Scan(src any) error {
	switch v := src.(type) {
	case int64:
		*n = Numeric{Decimal: strconv.FormatInt(v, 10)}
		return nil
	case float64:
		switch {
		case math.IsNaN(v):
			*n = Numeric{NaN: true}
		case math.IsInf(v, 1):
			*n = Numeric{Infinity: true}
		case math.IsInf(v, -1):
			*n = Numeric{NegInfinity: true}
		default:
			*n = Numeric{Decimal: strconv.FormatFloat(v, 'f', -1, 64)}
		}
		return nil
	}
	s, err := text("Numeric", src)
	if err != nil {
		return err
	}
	v, err := ParseNumeric(s)
	if err != nil {
		return err
	}
	*n = v
	return nil
}
