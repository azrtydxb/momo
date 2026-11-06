// SPDX-License-Identifier: Apache-2.0

package gen

import "github.com/azrtydxb/momo/gen/tags"

// AccessFor derives a field's write access from its tags. At most one of
// read_only, create_only and immutable may be present; create_only and
// immutable are rejected on pointer types. The returned diagnostics carry
// no position; the caller sets it.
func AccessFor(t tags.Tags, field string, pointer bool) (Access, []Diagnostic) {
	n := 0
	for _, b := range []bool{t.ReadOnly, t.CreateOnly, t.Immutable} {
		if b {
			n++
		}
	}
	if n > 1 {
		return Writable, []Diagnostic{{Msg: "conflicting write permissions on " + field}}
	}
	switch {
	case t.ReadOnly:
		return ReadOnly, nil
	case t.CreateOnly, t.Immutable:
		a := CreateOnly
		if t.Immutable {
			a = Immutable
		}
		if pointer {
			return Writable, []Diagnostic{{Msg: "create_only and immutable fields must not be pointers"}}
		}
		return a, nil
	}
	return Writable, nil
}

// DescriptorType returns the momo descriptor type and constructor names for
// an access mode, without type arguments.
func DescriptorType(a Access, pointer bool) (typeName, ctor string) {
	switch a {
	case ReadOnly:
		if pointer {
			return "NullReadOnlyColumn", "NewNullReadOnlyColumn"
		}
		return "ReadOnlyColumn", "NewReadOnlyColumn"
	case CreateOnly:
		return "CreateOnlyColumn", "NewCreateOnlyColumn"
	case Immutable:
		return "ImmutableColumn", "NewImmutableColumn"
	}
	if pointer {
		return "NullColumn", "NewNullColumn"
	}
	return "Column", "NewColumn"
}

// VectorDescriptorType returns the momo vector descriptor type and
// constructor names for an access mode, without type arguments. Every
// variant is accepted by the distance builders, so a restricted vector can
// still be searched.
func VectorDescriptorType(a Access, pointer bool) (typeName, ctor string) {
	switch a {
	case ReadOnly:
		return "ReadOnlyVectorColumn", "NewReadOnlyVectorColumn"
	case CreateOnly:
		return "CreateOnlyVectorColumn", "NewCreateOnlyVectorColumn"
	case Immutable:
		return "ImmutableVectorColumn", "NewImmutableVectorColumn"
	}
	if pointer {
		return "NullVectorColumn", "NewNullVectorColumn"
	}
	return "VectorColumn", "NewVectorColumn"
}
