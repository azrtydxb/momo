// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

// mysqlVectorLiteralFunc is the function that turns the bound text form of a
// vector into the server's VECTOR value: VEC_FromText on MariaDB and
// STRING_TO_VECTOR on MySQL.
func mysqlVectorLiteralFunc(f dialect.Flavor) string {
	if f == dialect.FlavorMariaDB {
		return "VEC_FromText"
	}
	return "STRING_TO_VECTOR"
}

// wrapMySQLVectorArg binds arg as the text form of a vector inside the
// server's text-to-vector function. The caller has already set the
// sensitivity of the bind.
func wrapMySQLVectorArg(b *Builder, flavor dialect.Flavor, arg any) {
	b.WriteString(mysqlVectorLiteralFunc(flavor) + "(")
	b.Arg(arg)
	b.WriteString(")")
}

// bindMySQLVector binds a typed vector value on MySQL or MariaDB and reports
// whether it handled v. A vector.Vector and a non-nil *vector.Vector (the
// nullable field type) go through the wrapper. A nil pointer is left to Arg,
// which binds SQL NULL with no wrapper, as is every other value.
func bindMySQLVector(b *Builder, v any) bool {
	if b.d.Name != dialect.MySQL || !b.d.Vector.Storage {
		return false
	}
	switch x := v.(type) {
	case vector.Vector:
		wrapMySQLVectorArg(b, b.d.Flavor, x)
	case *vector.Vector:
		if x == nil {
			return false
		}
		wrapMySQLVectorArg(b, b.d.Flavor, *x)
	default:
		return false
	}
	return true
}
