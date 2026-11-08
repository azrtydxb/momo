// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"reflect"
	"testing"
	"time"
)

func TestTypedColumn(t *testing.T) {
	s := "x"
	ts := time.Unix(1, 0)
	cases := []struct {
		name string
		in   []any
		want any
	}{
		{"int64", []any{int64(1), int64(2)}, []int64{1, 2}},
		{"string", []any{"a", "b"}, []string{"a", "b"}},
		{"time", []any{ts}, []time.Time{ts}},
		{"null keeps NULL", []any{int64(1), nil}, []*int64{ptrTo(int64(1)), nil}},
		{"leading null", []any{nil, "a"}, []*string{nil, ptrTo("a")}},
		{"pointer values", []any{&s, nil}, []*string{&s, nil}},
		{"all null is unknown", []any{nil, nil}, []any{nil, nil}},
		{"mixed types stay untyped", []any{int64(1), "a"}, []any{int64(1), "a"}},
		{"unknown type stays untyped", []any{struct{}{}}, []any{struct{}{}}},
	}
	for _, c := range cases {
		got := typedColumn(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: typedColumn = %#v, want %#v", c.name, got, c.want)
		}
	}
}

func ptrTo[T any](v T) *T { return &v }
