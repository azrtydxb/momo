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
	db := exampletest.Open(t)
	var out bytes.Buffer
	if err := run(context.Background(), db, &out); err != nil {
		t.Fatal(err)
	}
	const want = `nearest: engine repair (0.004), engine notes (0.019), engine history (0.168)
nearest news: engine notes (0.019), engine history (0.168)
ef_search 100: engine repair (0.004)
exact: engine repair (0.004)
plan uses items_embedding_hnsw: true
`
	if got := out.String(); got != want {
		t.Errorf("output differs.\ngot:\n%s\nwant:\n%s", got, want)
	}
}
