// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"encoding/json"
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

// fillNonZero sets every exported field reachable from v to a deterministic
// non-zero value derived from its path, and fails on a kind it cannot fill, so
// a new kind of field in the schema needs a decision here.
func fillNonZero(t *testing.T, v reflect.Value, path string) {
	t.Helper()
	switch v.Kind() {
	case reflect.String:
		v.SetString(path)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fillNonZero(t, s.Index(0), path)
		v.Set(s)
	case reflect.Pointer:
		p := reflect.New(v.Type().Elem())
		fillNonZero(t, p.Elem(), path)
		v.Set(p)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if f := v.Type().Field(i); f.IsExported() {
				fillNonZero(t, v.Field(i), path+"."+f.Name)
			}
		}
	default:
		t.Fatalf("schema field %s has kind %s, which the fingerprint fixture cannot fill: decide how it is hashed", path, v.Kind())
	}
}

// The fingerprint is pinned over a schema with every object kind and every
// field set, so a field added anywhere in the schema (even one that stays
// omitted for existing snapshots) changes the hash and fails here. If the new
// field is omitted from the JSON of every existing schema, markers already
// written still verify and the literal may be updated deliberately; otherwise
// the encoding changed and needs a new fingerprintVersion.
func TestStateFingerprintV1CoversEveryField(t *testing.T) {
	var s schema.Schema
	fillNonZero(t, reflect.ValueOf(&s).Elem(), "s")
	s.FormatVersion, s.Dialect = 1, "postgres"
	for i := 0; i < reflect.TypeOf(s).NumField(); i++ {
		if f := reflect.ValueOf(s).Field(i); f.Kind() == reflect.Slice && f.Len() == 0 {
			t.Fatalf("the fixture leaves %s empty", reflect.TypeOf(s).Field(i).Name)
		}
	}
	for k, o := range splitObjects(&s) {
		if _, err := json.MarshalIndent(o, "", "  "); err != nil {
			t.Fatalf("object %s cannot be encoded: %v", k, err)
		}
	}
	// Updated for the deferrability fields (issue 237): they are omitted from the
	// JSON of every schema that does not set them, which TestStateFingerprintV1IsPinned
	// still proves, so the markers already written still verify.
	const want = "8ff8164a68a8b7465932750140626caea2d2f93c8388b2285bc49c3d263294c8"
	if got := stateFingerprint(&s); got != want {
		t.Fatalf("fingerprint v1 of the full fixture: got %s, want %s", got, want)
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
