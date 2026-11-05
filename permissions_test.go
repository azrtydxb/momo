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
