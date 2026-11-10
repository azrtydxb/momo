// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

// A nullable vector field is a *vector.Vector: non-nil binds the sqlite-vec
// blob, nil binds NULL. Before the SQLite binding a nil pointer reached
// DialectValue and panicked.
func TestSQLiteVectorNullablePointer(t *testing.T) {
	sq := dialect.For(dialect.SQLite)
	v := vector.Vector{1, 2}
	want := []byte{0, 0, 0x80, 0x3f, 0, 0, 0, 0x40}
	got, err := compile(sq, func(b *Builder) { b.argFor(0, &v) })
	if err != nil {
		t.Fatal(err)
	}
	if blob, ok := got.Args[0].([]byte); got.SQL != "?" || len(got.Args) != 1 || !ok || !bytes.Equal(blob, want) {
		t.Fatalf("non-nil pointer: %q %#v, want a bare placeholder with the blob %x", got.SQL, got.Args, want)
	}
	var none *vector.Vector
	got, err = compile(sq, func(b *Builder) { b.argFor(0, none) })
	if err != nil {
		t.Fatal(err)
	}
	if got.SQL != "?" || len(got.Args) != 1 || got.Args[0] != nil {
		t.Fatalf("nil pointer: %q %#v, want a bare NULL bind", got.SQL, got.Args)
	}
	var noBits *vector.Bits
	got, err = compile(sq, func(b *Builder) { b.argFor(0, noBits) })
	if err != nil || got.SQL != "?" || got.Args[0] != nil {
		t.Fatalf("nil bits pointer: %q %#v, %v, want a bare NULL bind", got.SQL, got.Args, err)
	}
	bits := vector.Bits{Len: 8, Data: []byte{0xB0}}
	got, err = compile(sq, func(b *Builder) { b.argFor(0, &bits) })
	if blob, ok := got.Args[0].([]byte); err != nil || !ok || !bytes.Equal(blob, []byte{0xB0}) {
		t.Fatalf("bits pointer: %#v, %v, want the packed byte", got.Args, err)
	}
	// Other dialects keep their own binding: PostgreSQL gets the typed value.
	pg, err := compile(dialect.For(dialect.Postgres), func(b *Builder) { b.argFor(0, v) })
	if err != nil || pg.SQL != "$1" {
		t.Fatalf("postgres: %q, %v", pg.SQL, err)
	}
}

// Equality binds the blob on SQLite, and a value that does not validate (NaN,
// empty) is refused when the query is built.
func TestSQLiteVectorBindAndValidate(t *testing.T) {
	sq := dialect.For(dialect.SQLite)
	col := NewVectorColumn[vector.Vector]("docs", "emb", 2, 0)
	got, err := compile(sq, col.Eq(vector.Vector{1, 2}).AppendSQL)
	if err != nil {
		t.Fatal(err)
	}
	if blob, ok := got.Args[0].([]byte); got.SQL != `"docs"."emb" = ?` || !ok || len(blob) != 8 {
		t.Fatalf("Eq: %q %#v, want a placeholder with an 8 byte blob", got.SQL, got.Args)
	}
	_, err = compile(sq, col.Eq(vector.Vector{}).AppendSQL)
	if !errors.Is(err, vector.ErrInvalidVector) {
		t.Fatalf("empty vector: err = %v, want ErrInvalidVector", err)
	}
}

// Everything sqlite-vec cannot do is ErrUnsupported when the query is built:
// the halfvec and sparsevec types as arguments. The distance metrics it lacks
// are covered in package vecop.
func TestSQLiteVectorUnsupported(t *testing.T) {
	sq := dialect.For(dialect.SQLite)
	half := NewVectorColumn[vector.HalfVector]("docs", "h", 2, 0)
	sparse := NewVectorColumn[vector.SparseVector]("docs", "s", 2, 0)
	sv := vector.SparseVector{Dim: 2, Indices: []int32{0}, Values: []float32{1}}
	for name, fn := range map[string]func(*Builder){
		"halfvec bind":   func(b *Builder) { b.argFor(0, vector.HalfVector{1, 2}) },
		"sparsevec bind": func(b *Builder) { b.argFor(0, sv) },
		"halfvec eq":     half.Eq(vector.HalfVector{1, 2}).AppendSQL,
		"sparsevec eq":   sparse.Eq(sv).AppendSQL,
		"halfvec ptr":    func(b *Builder) { v := vector.HalfVector{1, 2}; b.argFor(0, &v) },
	} {
		if _, err := compile(sq, fn); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", name, err)
		}
	}
	// A nil pointer to a type SQLite lacks is just NULL: nothing is stored.
	var noHalf *vector.HalfVector
	if got, err := compile(sq, func(b *Builder) { b.argFor(0, noHalf) }); err != nil || got.Args[0] != nil {
		t.Errorf("nil halfvec pointer: %#v, %v, want NULL", got.Args, err)
	}
}

func TestSQLiteVectorColumnTypes(t *testing.T) {
	cases := []struct {
		name string
		got  func() (string, error)
		want string
	}{
		{"vector 3", func() (string, error) { return SQLiteVectorType("emb", 3) }, `BLOB CHECK (length("emb") = 12)`},
		{"vector 1536", func() (string, error) { return SQLiteVectorType("emb", 1536) }, `BLOB CHECK (length("emb") = 6144)`},
		{"quoted name", func() (string, error) { return SQLiteVectorType(`a"b`, 2) }, `BLOB CHECK (length("a""b") = 8)`},
		{"bits 8", func() (string, error) { return SQLiteBitsType("bits", 8) }, `BLOB CHECK (length("bits") = 1)`},
		{"bits 9", func() (string, error) { return SQLiteBitsType("bits", 9) }, `BLOB CHECK (length("bits") = 2)`},
		{"bits 128", func() (string, error) { return SQLiteBitsType("bits", 128) }, `BLOB CHECK (length("bits") = 16)`},
	}
	for _, c := range cases {
		if got, err := c.got(); err != nil || got != c.want {
			t.Errorf("%s: %q, %v; want %q", c.name, got, err, c.want)
		}
	}
	for name, fn := range map[string]func() (string, error){
		"dimensionless vector": func() (string, error) { return SQLiteVectorType("emb", 0) },
		"negative vector":      func() (string, error) { return SQLiteVectorType("emb", -1) },
		"zero bits":            func() (string, error) { return SQLiteBitsType("bits", 0) },
	} {
		if _, err := fn(); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", name, err)
		}
	}
	if _, err := SQLiteVectorType("emb", vector.MaxDim+1); !errors.Is(err, ErrValidation) {
		t.Errorf("oversize vector: err = %v, want ErrValidation", err)
	}
	if _, err := SQLiteBitsType("bits", vector.MaxBits+1); !errors.Is(err, ErrValidation) {
		t.Errorf("oversize bits: err = %v, want ErrValidation", err)
	}
}

func TestCheckVectorIndexMethod(t *testing.T) {
	cases := []struct {
		name   string
		d      *dialect.Dialect
		method string
		ok     bool
	}{
		{"postgres hnsw", dialect.For(dialect.Postgres), "hnsw", true},
		{"postgres ivfflat", dialect.For(dialect.Postgres), "ivfflat", true},
		{"postgres vector", dialect.For(dialect.Postgres), "vector", false},
		{"mariadb vector", dialect.MySQLFlavor(dialect.FlavorMariaDB), "vector", true},
		{"mariadb hnsw", dialect.MySQLFlavor(dialect.FlavorMariaDB), "hnsw", false},
		{"mysql vector", dialect.MySQLFlavor(dialect.FlavorMySQL), "vector", false},
		{"sqlite hnsw", dialect.For(dialect.SQLite), "hnsw", false},
		{"sqlite ivfflat", dialect.For(dialect.SQLite), "ivfflat", false},
		{"sqlite vector", dialect.For(dialect.SQLite), "vector", false},
		{"sqlite empty", dialect.For(dialect.SQLite), "", false},
	}
	for _, c := range cases {
		err := checkVectorIndexMethod(c.d, c.method)
		if c.ok && err != nil {
			t.Errorf("%s: %v, want nil", c.name, err)
		}
		if !c.ok && !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", c.name, err)
		}
	}
	err := checkVectorIndexMethod(dialect.For(dialect.SQLite), "hnsw")
	if err == nil || !strings.Contains(err.Error(), "sqlite") || !strings.Contains(err.Error(), "none") {
		t.Fatalf("diagnostic %v does not name the dialect and say no index exists", err)
	}
}

// A nil pointer to a vector type is the nullable field before it has a value:
// every dialect binds SQL NULL. Left to the DialectValue method the pointer
// inherits from the value type, it dereferenced nil and panicked on PostgreSQL,
// on MySQL and on MariaDB for every type but *vector.Vector.
func TestNilVectorPointerBindsNull(t *testing.T) {
	var (
		v  *vector.Vector
		h  *vector.HalfVector
		sp *vector.SparseVector
		bt *vector.Bits
	)
	dialects := map[string]*dialect.Dialect{
		"postgres": dialect.For(dialect.Postgres),
		"sqlite":   dialect.For(dialect.SQLite),
		"mariadb":  dialect.MySQLFlavor(dialect.FlavorMariaDB),
		"mysql":    dialect.MySQLFlavor(dialect.FlavorMySQL),
	}
	for dname, d := range dialects {
		for tname, p := range map[string]any{"vector": v, "halfvec": h, "sparsevec": sp, "bit": bt} {
			got, err := compile(d, func(b *Builder) { b.argFor(0, p) })
			if err != nil {
				t.Errorf("%s %s: %v", dname, tname, err)
				continue
			}
			if len(got.Args) != 1 || got.Args[0] != nil || strings.Contains(got.SQL, "(") {
				t.Errorf("%s %s: %q %#v, want a bare NULL bind", dname, tname, got.SQL, got.Args)
			}
		}
	}
	// A sensitive column binds NULL the same way.
	got, err := compile(dialect.For(dialect.Postgres), func(b *Builder) { b.argFor(FlagSensitive, v) })
	if err != nil || len(got.Args) != 1 || got.Args[0] != nil {
		t.Errorf("sensitive nil pointer: %#v, %v", got.Args, err)
	}
}
