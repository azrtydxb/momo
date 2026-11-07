// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"database/sql"
	"errors"
	"testing"
)

type namedBytes []byte

type (
	namedSlice []string
	namedMap   map[string]int
	namedIface interface{ String() string }
	namedStr   string
)

func (namedStr) String() string { return "" }

// TestScanNullNamedKinds covers named slice, map and interface types: they take
// their zero value like the unnamed kinds.
func TestScanNullNamedKinds(t *testing.T) {
	s := namedSlice{"a"}
	if err := scanNull(&s); err != nil || s != nil {
		t.Fatalf("named slice: err=%v v=%v", err, s)
	}
	m := namedMap{"a": 1}
	if err := scanNull(&m); err != nil || m != nil {
		t.Fatalf("named map: err=%v v=%v", err, m)
	}
	var i namedIface = namedStr("x")
	if err := scanNull(&i); err != nil || i != nil {
		t.Fatalf("named interface: err=%v v=%v", err, i)
	}
	var n namedStr
	if err := scanNull(&n); err == nil {
		t.Fatal("NULL into a named string must fail")
	}
}

// TestScanNullMatchedJoinColumn pins how a NULL in a matched joined row is
// stored: nullable destinations take their zero value, a non-nullable one is
// an ErrValidation rather than a silent zero.
func TestScanNullMatchedJoinColumn(t *testing.T) {
	var i int
	if err := scanNull(&i); err == nil {
		t.Fatal("NULL into *int must fail")
	} else {
		var me *Error
		if !errors.As(err, &me) || me.Kind != ErrValidation {
			t.Fatalf("NULL into *int: want ErrValidation, got %v", err)
		}
	}

	var s string
	if err := scanNull(&s); err == nil {
		t.Fatal("NULL into *string must fail")
	}

	ns := sql.NullString{String: "x", Valid: true}
	if err := scanNull(&ns); err != nil || ns.Valid {
		t.Fatalf("NULL into sql.NullString: err=%v valid=%v", err, ns.Valid)
	}

	p := new(int)
	if err := scanNull(&p); err != nil || p != nil {
		t.Fatalf("NULL into **int: err=%v p=%v", err, p)
	}

	nb := namedBytes("x")
	if err := scanNull(&nb); err != nil || nb != nil {
		t.Fatalf("NULL into named slice: err=%v v=%v", err, nb)
	}

	m := map[string]int{"a": 1}
	if err := scanNull(&m); err != nil || m != nil {
		t.Fatalf("NULL into map: err=%v v=%v", err, m)
	}

	var a any = 1
	if err := scanNull(&a); err != nil || a != nil {
		t.Fatalf("NULL into interface: err=%v v=%v", err, a)
	}
}
