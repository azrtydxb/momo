// SPDX-License-Identifier: Apache-2.0

// Command nmreport reads `go tool nm -size` output on standard input and
// prints a markdown report of the instantiated generic functions in it: the
// top functions by number of instantiations and by total code size, grouped by
// the generic function's name with its type arguments removed. It is the
// "which generics fan out" half of the compile-memory measurement.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// sym is one nm line.
type sym struct {
	typ  byte
	size int64
	name string
}

// agg is one generic function and its instantiations.
type agg struct {
	name  string
	count int
	size  int64
}

func main() {
	top := flag.Int("top", 30, "rows per table")
	title := flag.String("title", "", "heading of the report")
	flag.Parse()
	syms, err := parse(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "nmreport:", err)
		os.Exit(1)
	}
	fmt.Print(report(syms, *top, *title))
}

// nextField cuts the first space-separated token off s.
func nextField(s string) (tok, rest string) {
	s = strings.TrimLeft(s, " \t")
	tok, rest, _ = strings.Cut(s, " ")
	return tok, strings.TrimLeft(rest, " \t")
}

// parse reads "addr size type name" lines. The name is the rest of the line,
// because the shape names of generic instantiations contain spaces
// (go.shape.struct { ... }). Lines without a size and a type (undefined
// symbols) are skipped.
func parse(r io.Reader) ([]sym, error) {
	var out []sym
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for sc.Scan() {
		_, rest := nextField(sc.Text())
		sizeTok, rest := nextField(rest)
		typTok, name := nextField(rest)
		size, err := strconv.ParseInt(sizeTok, 10, 64)
		if err != nil || len(typTok) != 1 || name == "" {
			continue
		}
		out = append(out, sym{typ: typTok[0], size: size, name: name})
	}
	return out, sc.Err()
}

// baseName removes every bracketed type-argument list from a symbol name and
// reports whether it removed any. "pkg.Select[go.shape.struct { A int }].All"
// becomes "pkg.Select.All".
func baseName(s string) (string, bool) {
	var b strings.Builder
	depth, removed := 0, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '[':
			depth++
		case c == ']' && depth > 0:
			depth--
			removed = true
		case depth == 0:
			b.WriteByte(c)
		}
	}
	if depth != 0 {
		return s, false
	}
	return b.String(), removed && b.String() != s
}

func isText(t byte) bool { return t == 'T' || t == 't' }

// group totals the text symbols that are generic instantiations.
func group(syms []sym) (aggs []agg, instances int, instBytes, textBytes int64, dicts int) {
	m := map[string]*agg{}
	for _, s := range syms {
		if strings.Contains(s.name, "..dict.") {
			dicts++
		}
		if !isText(s.typ) {
			continue
		}
		textBytes += s.size
		base, ok := baseName(s.name)
		if !ok {
			continue
		}
		instances++
		instBytes += s.size
		a := m[base]
		if a == nil {
			a = &agg{name: base}
			m[base] = a
		}
		a.count++
		a.size += s.size
	}
	for _, a := range m {
		aggs = append(aggs, *a)
	}
	return aggs, instances, instBytes, textBytes, dicts
}

func table(b *strings.Builder, aggs []agg, top int, less func(x, y agg) bool) {
	sort.Slice(aggs, func(i, j int) bool {
		if less(aggs[i], aggs[j]) {
			return true
		}
		if less(aggs[j], aggs[i]) {
			return false
		}
		return aggs[i].name < aggs[j].name
	})
	b.WriteString("| # | generic function | instantiations | bytes |\n| --- | --- | ---: | ---: |\n")
	for i, a := range aggs {
		if i == top {
			break
		}
		fmt.Fprintf(b, "| %d | `%s` | %d | %d |\n", i+1, a.name, a.count, a.size)
	}
	b.WriteString("\n")
}

func report(syms []sym, top int, title string) string {
	aggs, inst, instBytes, textBytes, dicts := group(syms)
	var b strings.Builder
	if title != "" {
		fmt.Fprintf(&b, "#### %s\n\n", title)
	}
	fmt.Fprintf(&b, "%d text symbols (%d bytes) of which %d instantiations of %d generic functions (%d bytes); %d dictionaries.\n\n",
		countText(syms), textBytes, inst, len(aggs), instBytes, dicts)
	b.WriteString("Top by number of instantiations:\n\n")
	table(&b, append([]agg(nil), aggs...), top, func(x, y agg) bool { return x.count > y.count })
	b.WriteString("Top by total size:\n\n")
	table(&b, append([]agg(nil), aggs...), top, func(x, y agg) bool { return x.size > y.size })
	return b.String()
}

func countText(syms []sym) int {
	n := 0
	for _, s := range syms {
		if isText(s.typ) {
			n++
		}
	}
	return n
}
