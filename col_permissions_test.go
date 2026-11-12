// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"reflect"
	"strings"
	"testing"
)

func TestPermissionTypesMirrorColumn(t *testing.T) {
	want := predicateMethods(reflect.TypeOf(Column[int64]{}))
	if len(want) == 0 {
		t.Fatal("Column[int64] has no predicate methods")
	}
	wantNull := predicateMethods(reflect.TypeOf(NullColumn[int64]{}))
	if got := predicateMethods(reflect.TypeOf(NullReadOnlyColumn[int64]{})); !reflect.DeepEqual(got, wantNull) {
		t.Fatalf("NullReadOnlyColumn predicate methods %v want %v", got, wantNull)
	}
	for _, typ := range []reflect.Type{
		reflect.TypeOf(ReadOnlyColumn[int64]{}),
		reflect.TypeOf(CreateOnlyColumn[int64]{}),
		reflect.TypeOf(ImmutableColumn[int64]{}),
	} {
		if got := predicateMethods(typ); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s predicate methods %v want %v", typ, got, want)
		}
	}
}

// TestPermissionWrappersAreNotConvertible guards the write permissions. Go
// allows a conversion exactly when reflect says the types are convertible, so
// a conversion such as Column[T] applied to a read-only column would compile
// and reach Set if two wrappers shared an underlying type.
func TestPermissionWrappersAreNotConvertible(t *testing.T) {
	types := map[string]reflect.Type{
		"Column":             reflect.TypeOf(Column[string]{}),
		"NullColumn":         reflect.TypeOf(NullColumn[string]{}),
		"ReadOnlyColumn":     reflect.TypeOf(ReadOnlyColumn[string]{}),
		"NullReadOnlyColumn": reflect.TypeOf(NullReadOnlyColumn[string]{}),
		"CreateOnlyColumn":   reflect.TypeOf(CreateOnlyColumn[string]{}),
		"ImmutableColumn":    reflect.TypeOf(ImmutableColumn[string]{}),
	}
	for fromName, from := range types {
		for toName, to := range types {
			if fromName != toName && from.ConvertibleTo(to) {
				t.Errorf("%s converts to %s, which bypasses the write permission", fromName, toName)
			}
		}
	}
}

// TestReadOnlyWrappersHaveNoSetMethod guards the write permission itself: the
// mirror test above skips every Set* method, so nothing else would catch a
// write method (SetPtr, SetNull, Set) being added to a read-only wrapper.
func TestReadOnlyWrappersHaveNoSetMethod(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(ReadOnlyColumn[int64]{}),
		reflect.TypeOf(NullReadOnlyColumn[int64]{}),
	} {
		pt := reflect.PointerTo(typ)
		for i := 0; i < pt.NumMethod(); i++ {
			if name := pt.Method(i).Name; strings.HasPrefix(name, "Set") {
				t.Errorf("%s has write method %s", typ, name)
			}
		}
	}
}

// predicateMethods lists the exported methods of the pointer method set that are not write methods.
func predicateMethods(t reflect.Type) []string {
	pt := reflect.PointerTo(t)
	var out []string
	for i := 0; i < pt.NumMethod(); i++ {
		if name := pt.Method(i).Name; !strings.HasPrefix(name, "Set") {
			out = append(out, name)
		}
	}
	return out
}
