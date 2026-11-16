// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"
	"strconv"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

// isNilVectorPointer reports whether v is a nil pointer to one of the vector
// types, the nullable field types. Arg binds those as SQL NULL: left alone,
// the DialectValue method they inherit from the value type dereferences nil.
func isNilVectorPointer(v any) bool {
	switch p := v.(type) {
	case *vector.Vector:
		return p == nil
	case *vector.HalfVector:
		return p == nil
	case *vector.SparseVector:
		return p == nil
	case *vector.Bits:
		return p == nil
	}
	return false
}

// SQLiteVectorType returns the column definition of a float32 vector of dim
// elements on SQLite: a BLOB whose length is checked, for example
// BLOB CHECK (length("emb") = 12) for dim 3. sqlite-vec stores vectors in
// plain BLOB columns and has no declared dimension, so the CHECK is what keeps
// a vector of the wrong size out of the table. column is the name of the
// column being declared and is quoted. A dim below one is ErrUnsupported
// (a dimensionless column has no length to check) and one above
// vector.MaxDim is ErrValidation. Migration code calls this when it renders a
// vector column.
func SQLiteVectorType(column string, dim int) (string, error) {
	if dim < 1 {
		return "", &Error{Kind: ErrUnsupported, Op: "migrate", Column: column,
			Err: fmt.Errorf("a SQLite vector column needs a dimension for its length CHECK, got %d", dim)}
	}
	if dim > vector.MaxDim {
		return "", &Error{Kind: ErrValidation, Op: "migrate", Column: column,
			Err: fmt.Errorf("dimension %d outside 1..%d", dim, vector.MaxDim)}
	}
	return sqliteBlobCheck(column, strconv.Itoa(4*dim)), nil
}

// SQLiteBitsType is SQLiteVectorType for a bit vector of n bits: a BLOB whose
// length is the packed size, (n+7)/8 bytes. An n below one is ErrUnsupported
// and one above vector.MaxBits is ErrValidation.
func SQLiteBitsType(column string, n int) (string, error) {
	if n < 1 {
		return "", &Error{Kind: ErrUnsupported, Op: "migrate", Column: column,
			Err: fmt.Errorf("a SQLite bit vector column needs a length for its length CHECK, got %d", n)}
	}
	if n > vector.MaxBits {
		return "", &Error{Kind: ErrValidation, Op: "migrate", Column: column,
			Err: fmt.Errorf("length %d outside 1..%d", n, vector.MaxBits)}
	}
	return sqliteBlobCheck(column, strconv.Itoa((n+7)/8)), nil
}

func sqliteBlobCheck(column, bytes string) string {
	d := dialect.For(dialect.SQLite)
	return "BLOB CHECK (length(" + string(d.AppendIdent(nil, column)) + ") = " + bytes + ")"
}
