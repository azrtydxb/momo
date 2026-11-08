// SPDX-License-Identifier: Apache-2.0

package vector

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestDialectValues(t *testing.T) {
	v := Vector{1, 2}
	got, err := v.DialectValue(dialect.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]byte, 8)
	binary.LittleEndian.PutUint32(want[0:], 0x3f800000) // float32(1)
	binary.LittleEndian.PutUint32(want[4:], 0x40000000) // float32(2)
	if b, ok := got.([]byte); !ok || !bytes.Equal(b, want) {
		t.Fatalf("sqlite encoding = %x, want %x", got, want)
	}
	pg, err := v.DialectValue(dialect.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if pg == nil {
		t.Fatal("postgres value must be passed through to the driver")
	}
	my, err := v.DialectValue(dialect.MySQL)
	if err != nil {
		t.Fatal(err)
	}
	if my != "[1,2]" {
		t.Fatalf("mysql encoding = %#v, want the text form [1,2]", my)
	}
	bits, err := (Bits{Len: 8, Data: []byte{0x80}}).DialectValue(dialect.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := bits.([]byte); !ok || !bytes.Equal(b, []byte{0x80}) {
		t.Fatalf("sqlite bits = %v, want packed bytes 80", bits)
	}
	if _, err := (HalfVector{1}).DialectValue(dialect.SQLite); !errors.Is(err, ErrDialect) {
		t.Fatalf("halfvec on sqlite: err = %v, want ErrDialect", err)
	}
	if _, err := (SparseVector{Dim: 2, Indices: []int32{0}, Values: []float32{1}}).DialectValue(dialect.SQLite); !errors.Is(err, ErrDialect) {
		t.Fatalf("sparsevec on sqlite: err = %v, want ErrDialect", err)
	}
}

// TestDialectValuesMatrix checks every vector type on every dialect.
func TestDialectValuesMatrix(t *testing.T) {
	pgOnly := map[dialect.Name]bool{dialect.Postgres: true}
	pgSQLite := map[dialect.Name]bool{dialect.Postgres: true, dialect.SQLite: true}
	pgSQLiteMySQL := map[dialect.Name]bool{dialect.Postgres: true, dialect.SQLite: true, dialect.MySQL: true}
	types := []struct {
		name string
		fn   func(dialect.Name) (any, error)
		ok   map[dialect.Name]bool
	}{
		{"vector", Vector{1}.DialectValue, pgSQLiteMySQL},
		{"halfvec", HalfVector{1}.DialectValue, pgOnly},
		{"sparsevec", SparseVector{Dim: 2, Indices: []int32{0}, Values: []float32{1}}.DialectValue, pgOnly},
		{"bit", Bits{Len: 8, Data: []byte{1}}.DialectValue, pgSQLite},
	}
	for _, ty := range types {
		for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
			_, err := ty.fn(d)
			if ty.ok[d] && err != nil {
				t.Errorf("%s on %s: unexpected error %v", ty.name, d, err)
			}
			if !ty.ok[d] && !errors.Is(err, ErrDialect) {
				t.Errorf("%s on %s: err = %v, want ErrDialect", ty.name, d, err)
			}
		}
	}
}
