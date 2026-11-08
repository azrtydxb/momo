// SPDX-License-Identifier: Apache-2.0

package vector

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/azrtydxb/momo/dialect"
)

func unsupportedDialect(typ string, d dialect.Name) error {
	return fmt.Errorf("%w: %s on %s", ErrDialect, typ, d)
}

// DialectValue encodes v for dialect d. PostgreSQL passes the value through
// so the driver chooses text or binary; SQLite gets a little-endian float32
// blob; MySQL and MariaDB get the text form, which the SQL wraps in
// STRING_TO_VECTOR or VEC_FromText.
func (v Vector) DialectValue(d dialect.Name) (any, error) {
	switch d {
	case dialect.Postgres:
		return v, nil
	case dialect.MySQL:
		return v.Value()
	case dialect.SQLite:
		b := make([]byte, 4*len(v))
		for i, x := range v {
			binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(x))
		}
		return b, nil
	}
	return nil, unsupportedDialect("vector", d)
}

// DialectValue encodes h for dialect d; only PostgreSQL supports halfvec.
func (h HalfVector) DialectValue(d dialect.Name) (any, error) {
	if d == dialect.Postgres {
		return h, nil
	}
	return nil, unsupportedDialect("halfvec", d)
}

// DialectValue encodes s for dialect d; only PostgreSQL supports sparsevec.
func (s SparseVector) DialectValue(d dialect.Name) (any, error) {
	if d == dialect.Postgres {
		return s, nil
	}
	return nil, unsupportedDialect("sparsevec", d)
}

// DialectValue encodes b for dialect d. SQLite gets the packed bytes; its SQL
// wraps them in vec_bit.
func (b Bits) DialectValue(d dialect.Name) (any, error) {
	switch d {
	case dialect.Postgres:
		return b, nil
	case dialect.SQLite:
		return b.Data, nil
	}
	return nil, unsupportedDialect("bit", d)
}
