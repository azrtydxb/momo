// SPDX-License-Identifier: Apache-2.0

package vector

import "testing"

func FuzzVectorDecode(f *testing.F) {
	f.Add("[1,2,3]")
	f.Add("[]")
	f.Add("[NaN]")
	f.Add("{1:1,3:2}/5")
	f.Fuzz(func(t *testing.T, in string) {
		var v Vector
		if err := v.Scan(in); err != nil {
			return
		}
		enc, err := v.Value()
		if err != nil {
			t.Fatalf("Value after successful Scan: %v", err)
		}
		var again Vector
		if err := again.Scan(enc); err != nil {
			t.Fatalf("re-scan of %q failed: %v", enc, err)
		}
		if len(again) != len(v) {
			t.Fatalf("round trip changed length: %d -> %d", len(v), len(again))
		}
		for i := range v {
			if again[i] != v[i] {
				t.Fatalf("round trip changed element %d: %v -> %v", i, v[i], again[i])
			}
		}
	})
}

func FuzzSparseDecode(f *testing.F) {
	f.Add("{1:1,3:2}/5")
	f.Add("{}/3")
	f.Fuzz(func(t *testing.T, in string) {
		var s SparseVector
		if err := s.Scan(in); err != nil {
			return
		}
		if err := s.Validate(s.Dim); err != nil {
			t.Fatalf("Scan accepted %q but Validate rejects: %v", in, err)
		}
		enc, err := s.Value()
		if err != nil {
			t.Fatal(err)
		}
		var again SparseVector
		if err := again.Scan(enc); err != nil {
			t.Fatalf("re-scan of %q: %v", enc, err)
		}
		if again.Dim != s.Dim || len(again.Indices) != len(s.Indices) {
			t.Fatalf("round trip changed %+v -> %+v", s, again)
		}
	})
}
