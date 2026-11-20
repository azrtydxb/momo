// SPDX-License-Identifier: Apache-2.0

package vectortype

import (
	"fmt"
	"strconv"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

// SQLite returns the column definition of a float32 vector of dim elements on
// SQLite: a BLOB whose length is checked, for example
// BLOB CHECK (length("emb") = 12) for dim 3. sqlite-vec stores vectors in
// plain BLOB columns and has no declared dimension, so the CHECK is what keeps
// a vector of the wrong size out of the table. column is the name of the
// column being declared and is quoted. A dim below one is momo.ErrUnsupported
// (a dimensionless column has no length to check) and one above
// vector.MaxDim is momo.ErrValidation. Migration code calls this when it
// renders a vector column.
func SQLite(column string, dim int) (string, error) {
	if dim < 1 {
		return "", &momo.Error{Kind: momo.ErrUnsupported, Op: "migrate", Column: column,
			Err: fmt.Errorf("a SQLite vector column needs a dimension for its length CHECK, got %d", dim)}
	}
	if dim > vector.MaxDim {
		return "", &momo.Error{Kind: momo.ErrValidation, Op: "migrate", Column: column,
			Err: fmt.Errorf("dimension %d outside 1..%d", dim, vector.MaxDim)}
	}
	return sqliteBlobCheck(column, strconv.Itoa(4*dim)), nil
}

// SQLiteBits is SQLite for a bit vector of n bits: a BLOB whose length is the
// packed size, (n+7)/8 bytes. An n below one is momo.ErrUnsupported and one
// above vector.MaxBits is momo.ErrValidation.
func SQLiteBits(column string, n int) (string, error) {
	if n < 1 {
		return "", &momo.Error{Kind: momo.ErrUnsupported, Op: "migrate", Column: column,
			Err: fmt.Errorf("a SQLite bit vector column needs a length for its length CHECK, got %d", n)}
	}
	if n > vector.MaxBits {
		return "", &momo.Error{Kind: momo.ErrValidation, Op: "migrate", Column: column,
			Err: fmt.Errorf("length %d outside 1..%d", n, vector.MaxBits)}
	}
	return sqliteBlobCheck(column, strconv.Itoa((n+7)/8)), nil
}

func sqliteBlobCheck(column, bytes string) string {
	d := dialect.For(dialect.SQLite)
	return "BLOB CHECK (length(" + string(d.AppendIdent(nil, column)) + ") = " + bytes + ")"
}
