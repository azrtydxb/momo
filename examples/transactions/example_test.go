// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/azrtydxb/momo/examples/internal/exampletest"
)

// TestExample runs the example end to end on a scratch database that has the
// example's migrations applied, and pins what it prints.
func TestExample(t *testing.T) {
	exampletest.CheckGenerated(t)
	db := exampletest.Open(t, "migrations")
	var out bytes.Buffer
	if err := run(context.Background(), db, &out); err != nil {
		t.Fatal(err)
	}
	const want = `transfer 30 ada->grace: err=<nil>
  balances: ada=70 grace=80
transfer 500 ada->grace: check violation=true
  balances: ada=70 grace=80
savepoint: unique violation=true
  balances: ada=71 grace=80
write in read-only transaction refused: SQLSTATE 25006
  balances: ada=71 grace=80
serializable retry: attempts=2 err=<nil>
  balances: ada=86 grace=80
`
	if got := out.String(); got != want {
		t.Errorf("output differs.\ngot:\n%s\nwant:\n%s", got, want)
	}
}
