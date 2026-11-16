// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"reflect"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// The fingerprint a migration's base marker records must not change when the
// schema structs do: it is pinned to a literal. If this fails, the inputs of
// the encoding changed (a field without omitempty, a JSON tag, the layout), and
// every marker already written would be reported stale. Do not edit the literal:
// add a new fingerprintVersion with its own encoding and keep v1 readable.
func TestStateFingerprintV1IsPinned(t *testing.T) {
	s := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{Name: "t", Columns: []schema.Column{{Name: "id", Type: "bigint"}}}}}
	const want = "56f0c210dc6c4e52d36f6fd1c668d7ee88065204ae998bef9c253a198b43c74d"
	if got := stateFingerprint(s); got != want {
		t.Fatalf("fingerprint v1 changed: got %s, want %s; a change here needs a new fingerprintVersion", got, want)
	}
	if got := derivedBase(s); got != "derived:v1:"+want {
		t.Fatalf("marker = %q", got)
	}
}

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
