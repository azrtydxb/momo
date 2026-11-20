// SPDX-License-Identifier: Apache-2.0

package momo

import "github.com/azrtydxb/momo/vector"

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
