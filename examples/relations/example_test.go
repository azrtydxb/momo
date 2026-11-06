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
	const want = `find or create: first=true second=false same row=true
graph: note.AuthorID set=true, comment.DocumentID matches=true
load Engines comments=[fourth third]
load Quiet   comments=[]
load Note    comments=[nice]
loadall Engines by Ada, tags=[go orm], comments=1
loadall Quiet by Ada, tags=[], comments=0
loadall Note  by Grace, tags=[], comments=1
preload Engines by Ada, first comment=[first]
preload Quiet  by Ada, first comment=[]
preload Note   by Grace, first comment=[nice]
has an unapproved comment: Engines
has no comments: Quiet
most commented first: Engines, Note, Quiet
link go+orm: 2 tags
unlink go: 1 tags
replace with go: 1 tags
`
	if got := out.String(); got != want {
		t.Errorf("output differs.\ngot:\n%s\nwant:\n%s", got, want)
	}
}
