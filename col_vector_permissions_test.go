// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/vector"
)

// TestVectorPermissionWrappersAreNotConvertible mirrors the scalar guard in
// col_permissions_test.go: a conversion between two vector wrappers would
// compile and reach a Set the target has, so no pair may be convertible.
func TestVectorPermissionWrappersAreNotConvertible(t *testing.T) {
	types := map[string]reflect.Type{
		"VectorColumn":             reflect.TypeOf(VectorColumn[vector.Vector]{}),
		"NullVectorColumn":         reflect.TypeOf(NullVectorColumn[vector.Vector]{}),
		"ReadOnlyVectorColumn":     reflect.TypeOf(ReadOnlyVectorColumn[vector.Vector]{}),
		"NullReadOnlyVectorColumn": reflect.TypeOf(NullReadOnlyVectorColumn[vector.Vector]{}),
		"CreateOnlyVectorColumn":   reflect.TypeOf(CreateOnlyVectorColumn[vector.Vector]{}),
		"ImmutableVectorColumn":    reflect.TypeOf(ImmutableVectorColumn[vector.Vector]{}),
	}
	for fromName, from := range types {
		for toName, to := range types {
			if fromName != toName && from.ConvertibleTo(to) {
				t.Errorf("%s converts to %s, which bypasses the write permission", fromName, toName)
			}
		}
	}
}

// TestReadOnlyVectorWrappersHaveNoSetMethod fails when a write method is added
// to a read-only vector wrapper.
func TestReadOnlyVectorWrappersHaveNoSetMethod(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(ReadOnlyVectorColumn[vector.Vector]{}),
		reflect.TypeOf(NullReadOnlyVectorColumn[vector.Vector]{}),
	} {
		pt := reflect.PointerTo(typ)
		for i := 0; i < pt.NumMethod(); i++ {
			if name := pt.Method(i).Name; strings.HasPrefix(name, "Set") {
				t.Errorf("%s has write method %s", typ, name)
			}
		}
	}
}

// TestNullReadOnlyVectorMirrorsReadOnly: the nullable variant offers the same
// read surface as ReadOnlyVectorColumn.
func TestNullReadOnlyVectorMirrorsReadOnly(t *testing.T) {
	want := predicateMethods(reflect.TypeOf(ReadOnlyVectorColumn[vector.Vector]{}))
	if len(want) == 0 {
		t.Fatal("ReadOnlyVectorColumn has no predicate methods")
	}
	if got := predicateMethods(reflect.TypeOf(NullReadOnlyVectorColumn[vector.Vector]{})); !reflect.DeepEqual(got, want) {
		t.Fatalf("NullReadOnlyVectorColumn predicate methods %v want %v", got, want)
	}
}
