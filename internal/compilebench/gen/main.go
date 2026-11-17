// SPDX-License-Identifier: Apache-2.0

// Command gen writes the compile-memory fixture: a models package of N plain
// model structs (momo generate then adds the real companion code: Meta, typed
// column descriptors and relation descriptors) and a caller package of Q typed
// queries that cover the common builder shapes (Select with Row1..Row8, Where
// compositions, joins, aggregates, Update, Insert, Upsert, relation loads).
//
// The output is deterministic: the same flags always write the same bytes. It
// is a measurement fixture, never part of momo's API: CI generates it, compiles
// the caller package under /usr/bin/time and records the compiler's peak
// memory (.github/workflows/compile-memory.yml, scripts/compile-memory.sh).
package main

import (
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
)

// spec is the size of the fixture.
type spec struct {
	Models  int    // model structs; at least 3 so relation chains exist
	Queries int    // query functions in the caller package
	PerFile int    // query functions per caller source file
	Root    string // import path of the output directory
}

func main() {
	var s spec
	out := flag.String("out", "internal/compilebench/work", "output directory (gitignored)")
	flag.IntVar(&s.Models, "models", 120, "number of model structs")
	flag.IntVar(&s.Queries, "queries", 600, "number of typed query functions")
	flag.IntVar(&s.PerFile, "per-file", 15, "query functions per caller source file")
	flag.StringVar(&s.Root, "import-path", "github.com/azrtydxb/momo/internal/compilebench/work", "import path of the output directory")
	flag.Parse()

	files, err := generate(s)
	if err != nil {
		fmt.Fprintln(os.Stderr, "compilebench gen:", err)
		os.Exit(1)
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		p := filepath.Join(*out, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			fmt.Fprintln(os.Stderr, "compilebench gen:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(p, files[n], 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "compilebench gen:", err)
			os.Exit(1)
		}
	}
	fmt.Printf("wrote %d files: %d models, %d queries under %s\n", len(files), s.Models, s.Queries, *out)
}

// generate returns the fixture as relative slash path -> gofmt'd source.
func generate(s spec) (map[string][]byte, error) {
	if s.Models < 3 {
		return nil, fmt.Errorf("-models must be at least 3, got %d", s.Models)
	}
	if s.Queries < 1 || s.PerFile < 1 {
		return nil, fmt.Errorf("-queries and -per-file must be positive, got %d and %d", s.Queries, s.PerFile)
	}
	ms := newModels(s.Models)
	files := map[string][]byte{}

	src, err := format.Source(modelsSource(ms))
	if err != nil {
		return nil, fmt.Errorf("models source: %w", err)
	}
	files["models/models.go"] = src

	g := &gen{ms: ms, root: s.Root}
	for first := 0; first < s.Queries; first += s.PerFile {
		last := min(first+s.PerFile, s.Queries)
		body, err := format.Source(g.callerFile(first, last))
		if err != nil {
			return nil, fmt.Errorf("caller source from query %d: %w", first, err)
		}
		files[fmt.Sprintf("caller/q%04d.go", first)] = body
	}
	return files, nil
}
