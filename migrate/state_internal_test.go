// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"reflect"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// Every list of objects in schema.Schema must be handled by the fold. A kind
// added to the schema and forgotten in splitObjects would be dropped silently
// from a merged state; this fails first.
func TestSplitObjectsKeepsEveryObjectKind(t *testing.T) {
	var s schema.Schema
	v := reflect.ValueOf(&s).Elem()
	kinds := 0
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() != reflect.Slice {
			continue
		}
		f.Set(reflect.MakeSlice(f.Type(), 1, 1))
		kinds++
	}
	if kinds == 0 {
		t.Fatal("found no list field in schema.Schema")
	}
	if got, want := countObjs(splitObjects(&s)), objectCount(&s); got != want || want != kinds {
		t.Fatalf("splitObjects holds %d objects of the %d kinds the schema has (counted %d): a kind is not handled", got, kinds, want)
	}
	back, err := assemble(&s, splitObjects(&s))
	if err != nil {
		t.Fatal(err)
	}
	if objectCount(back) != kinds {
		t.Fatalf("assemble returns %d objects, want one of each of %d kinds", objectCount(back), kinds)
	}
}
