// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"testing"
)

type optionalFakeDriver struct{ Driver }

func (optionalFakeDriver) PoolStats() PoolStats { return PoolStats{MaxConns: 4, AcquiredConns: 1} }
func (optionalFakeDriver) Capabilities() Capabilities {
	return Capabilities{Vectors: true}
}

// optionalFull implements every optional capability so the interface shapes
// are checked at compile time.
type optionalFull struct{ optionalFakeDriver }

func (optionalFull) Pin(context.Context) (PinnedConn, error) { return nil, nil }
func (optionalFull) CopyFrom(context.Context, string, []string, [][]any) (int64, error) {
	return 0, nil
}
func (optionalFull) SendBatch(context.Context, []Compiled) ([]Rows, error) { return nil, nil }
func (optionalFull) ServerVersion(context.Context) (int, error)            { return 170002, nil }

type optionalPinned struct{}

func (optionalPinned) Query(context.Context, string, []any) (Rows, error) { return nil, nil }
func (optionalPinned) Exec(context.Context, string, []any) (int64, error) { return 0, nil }
func (optionalPinned) Begin(context.Context, TxOptions) (DriverTx, error) { return nil, nil }
func (optionalPinned) Release() error                                     { return nil }

var (
	_ Pinner             = optionalFull{}
	_ Copier             = optionalFull{}
	_ Batcher            = optionalFull{}
	_ PoolStater         = optionalFull{}
	_ ServerVersioner    = optionalFull{}
	_ CapabilityReporter = optionalFull{}
	_ PinnedConn         = optionalPinned{}
)

func TestOptionalInterfacesAreSatisfiedByAdapters(t *testing.T) {
	var d any = optionalFakeDriver{}
	if _, ok := d.(PoolStater); !ok {
		t.Fatal("fakeDriver must satisfy PoolStater")
	}
	if got := d.(CapabilityReporter).Capabilities(); !got.Vectors {
		t.Fatalf("Capabilities().Vectors = false, want true")
	}
	if got := d.(PoolStater).PoolStats().MaxConns; got != 4 {
		t.Fatalf("MaxConns = %d, want 4", got)
	}
	// A plain driver must not claim optional capabilities.
	if _, ok := d.(Pinner); ok {
		t.Fatal("driver without Pin must not satisfy Pinner")
	}
}
