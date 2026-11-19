// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// plainDriver has the methods of Driver and nothing else: it owns nothing.
type plainDriver struct{ Driver }

func (plainDriver) Dialect() *dialect.Dialect { return dialect.For(dialect.Postgres) }

// ownedDriver also implements io.Closer, as the driver of an Open function does.
type ownedDriver struct {
	plainDriver
	closes int
	err    error
}

func (o *ownedDriver) Close() error {
	o.closes++
	return o.err
}

// Catches: DB.Close skipping the driver an Open function returned, which leaks
// its pool, and closing a driver that wraps a pool the caller owns.
func TestDBCloseClosesOwnedDriverOnly(t *testing.T) {
	o := &ownedDriver{}
	db := New(o)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if o.closes != 1 {
		t.Fatalf("driver closed %d times, want 1", o.closes)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := New(plainDriver{}).Close(); err != nil {
		t.Fatalf("Close of a driver without Close = %v, want nil", err)
	}
}

// Catches: DB.Close swallowing the error of the driver's Close.
func TestDBClosePassesTheDriverError(t *testing.T) {
	want := errors.New("boom")
	if err := New(&ownedDriver{err: want}).Close(); !errors.Is(err, want) {
		t.Fatalf("Close = %v, want %v", err, want)
	}
}
