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
	const want = `insert zero     priority=0 done=false note=NULL
insert defaults priority=3 done=false note=NULL
update non-zero     priority=5 done=true note="call Ada"
update zero values  priority=0 done=false note="call Ada"
update empty string priority=0 done=false note=""
update NULL         priority=0 done=false note=NULL
tasks with NULL note: 2
model update ok, version 0 -> 1
stale update: optimistic lock conflict=true
soft delete: live=1 with deleted=2 only deleted=["zero"]
plain Delete refused: ErrValidation
force delete removed 1 row
`
	if got := out.String(); got != want {
		t.Errorf("output differs.\ngot:\n%s\nwant:\n%s", got, want)
	}
}
