// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
)

const testRoot = "example.com/fixture"

// Catches: a generator whose output changes between runs (the CI budget would
// then compare different programs), or one that writes source gofmt rejects.
func TestGenerateIsDeterministicAndParses(t *testing.T) {
	s := spec{Models: 9, Queries: 200, PerFile: 15, Root: testRoot}
	a, err := generate(s)
	if err != nil {
		t.Fatal(err)
	}
	b, err := generate(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatalf("file count differs between runs: %d vs %d", len(a), len(b))
	}
	fset := token.NewFileSet()
	for name, src := range a {
		if !bytes.Equal(src, b[name]) {
			t.Errorf("%s differs between runs", name)
		}
		if _, err := parser.ParseFile(fset, name, src, parser.AllErrors); err != nil {
			t.Errorf("%s does not parse: %v", name, err)
		}
	}
	// 200 queries at 15 per file is 14 caller files plus the models file.
	if len(a) != 15 {
		t.Errorf("got %d files, want 15", len(a))
	}
}

// Catches: a shape that is never emitted, which would silently shrink what the
// memory guard covers.
func TestEveryShapeAndRowArityIsEmitted(t *testing.T) {
	s := spec{Models: 12, Queries: len(shapes) * 3, PerFile: 10, Root: testRoot}
	files, err := generate(s)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var all strings.Builder
	for _, n := range names {
		all.Write(files[n])
	}
	for _, sh := range shapes {
		if !strings.Contains(all.String(), " is a "+sh.name+" query.") {
			t.Errorf("shape %s is never emitted", sh.name)
		}
	}
	for n := 1; n <= 8; n++ {
		if !strings.Contains(all.String(), fmt.Sprintf("momo.Row%d[", n)) {
			t.Errorf("no query uses momo.Row%d", n)
		}
	}
}

// Catches: too few models for the relation chains, which would index outside
// the model list.
func TestGenerateRejectsTinyFixtures(t *testing.T) {
	if _, err := generate(spec{Models: 2, Queries: 10, PerFile: 5, Root: testRoot}); err == nil {
		t.Error("two models must be rejected")
	}
	if _, err := generate(spec{Models: 5, Queries: 0, PerFile: 5, Root: testRoot}); err == nil {
		t.Error("zero queries must be rejected")
	}
}
