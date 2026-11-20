// SPDX-License-Identifier: Apache-2.0

package vectortype

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/vector"
)

func TestSQLiteVectorColumnTypes(t *testing.T) {
	cases := []struct {
		name string
		got  func() (string, error)
		want string
	}{
		{"vector 3", func() (string, error) { return SQLite("emb", 3) }, `BLOB CHECK (length("emb") = 12)`},
		{"vector 1536", func() (string, error) { return SQLite("emb", 1536) }, `BLOB CHECK (length("emb") = 6144)`},
		{"quoted name", func() (string, error) { return SQLite(`a"b`, 2) }, `BLOB CHECK (length("a""b") = 8)`},
		{"bits 8", func() (string, error) { return SQLiteBits("bits", 8) }, `BLOB CHECK (length("bits") = 1)`},
		{"bits 9", func() (string, error) { return SQLiteBits("bits", 9) }, `BLOB CHECK (length("bits") = 2)`},
		{"bits 128", func() (string, error) { return SQLiteBits("bits", 128) }, `BLOB CHECK (length("bits") = 16)`},
	}
	for _, c := range cases {
		if got, err := c.got(); err != nil || got != c.want {
			t.Errorf("%s: %q, %v; want %q", c.name, got, err, c.want)
		}
	}
	for name, fn := range map[string]func() (string, error){
		"dimensionless vector": func() (string, error) { return SQLite("emb", 0) },
		"negative vector":      func() (string, error) { return SQLite("emb", -1) },
		"zero bits":            func() (string, error) { return SQLiteBits("bits", 0) },
	} {
		if _, err := fn(); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", name, err)
		}
	}
	if _, err := SQLite("emb", vector.MaxDim+1); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("oversize vector: err = %v, want ErrValidation", err)
	}
	if _, err := SQLiteBits("bits", vector.MaxBits+1); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("oversize bits: err = %v, want ErrValidation", err)
	}
}
