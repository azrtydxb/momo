// SPDX-License-Identifier: Apache-2.0

package docscheck

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every ```go block under docs/ is one of three kinds, chosen by the marker
// comment on the line before the fence (an HTML comment, so it does not render):
//
//	no marker                     a complete package main program; it is compiled
//	<!-- docscheck:run -->        a complete program that is also executed (integration tests)
//	<!-- docscheck:fragment -->   a snippet that is not a program; it must still parse as Go
//
// Fragments are the visible, bounded exception: they cannot be compiled alone
// because they use the caller's generated code. maxFragmentBlocks is a ceiling
// that is lowered when fragments become programs and raised only with a
// reviewer's eyes on it: write a complete program instead of adding a fragment.
const (
	markerRun         = "<!-- docscheck:run -->"
	markerFragment    = "<!-- docscheck:fragment -->"
	maxFragmentBlocks = 69
)

var goFence = regexp.MustCompile("^```go\\s*$")

type docBlock struct {
	file     string
	line     int
	run      bool
	fragment bool
	src      string
}

func (b docBlock) where(root string) string {
	rel, err := filepath.Rel(root, b.file)
	if err != nil {
		rel = b.file
	}
	return fmt.Sprintf("%s:%d", filepath.ToSlash(rel), b.line)
}

func docFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	for _, pattern := range []string{"*.md", "*/*.md"} {
		m, err := filepath.Glob(filepath.Join(root, "docs", pattern))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	return files
}

// docBlocks returns every ```go block of every page under docs/.
func docBlocks(t *testing.T, root string) []docBlock {
	t.Helper()
	var out []docBlock
	for _, f := range docFiles(t, root) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(b), "\n")
		for i := 0; i < len(lines); i++ {
			if !goFence.MatchString(lines[i]) {
				continue
			}
			blk := docBlock{file: f, line: i + 1}
			for k := i - 1; k >= 0; k-- {
				prev := strings.TrimSpace(lines[k])
				if prev == "" {
					continue
				}
				blk.run = prev == markerRun
				blk.fragment = prev == markerFragment
				break
			}
			var body []string
			for i++; i < len(lines) && strings.TrimSpace(lines[i]) != "```"; i++ {
				body = append(body, lines[i])
			}
			blk.src = strings.Join(body, "\n") + "\n"
			out = append(out, blk)
		}
	}
	return out
}

var declStart = regexp.MustCompile(`^(import|type|func|var|const)\b`)

// splitDecls separates the blank-line-delimited paragraphs of src that start
// with a declaration keyword from the rest, so a snippet that shows
// declarations followed by statements parses as a file plus a function body.
func splitDecls(src string) (decls, body string) {
	var d, b []string
	for _, para := range strings.Split(src, "\n\n") {
		if declStart.MatchString(strings.TrimSpace(para)) {
			d = append(d, para)
		} else {
			b = append(b, para)
		}
	}
	return strings.Join(d, "\n\n"), strings.Join(b, "\n\n")
}

// parsesAsGo reports whether src parses as Go in one of the shapes a
// documentation snippet takes: a file body, a statement list, declarations
// followed by statements, or the fields of a struct.
func parsesAsGo(src string) error {
	decls, body := splitDecls(src)
	shapes := []string{
		"package p\n" + src,
		"package p\nfunc _() {\n" + src + "\n}\n",
		"package p\n" + decls + "\nfunc _() {\n" + body + "\n}\n",
		"package p\ntype _ struct {\n" + src + "\n}\n",
	}
	fset := token.NewFileSet()
	var first error
	for _, s := range shapes {
		_, err := parser.ParseFile(fset, "block.go", s, parser.SkipObjectResolution)
		if err == nil {
			return nil
		}
		if first == nil {
			first = err
		}
	}
	return first
}

func isProgram(src string) bool {
	return regexp.MustCompile(`(?m)^package main\s*$`).MatchString(src)
}

// blockModule creates a scratch module outside the repository that requires
// this module through a replace directive, so block programs are built against
// the current source without leaving files in the tree for other tests to find.
func blockModule(t *testing.T, root string) string {
	t.Helper()
	gomod, err := os.ReadFile(filepath.Join(root, "go.mod")) // #nosec G304 -- the repository's own file
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^go (\S+)\s*$`).FindSubmatch(gomod)
	if m == nil {
		t.Fatal("go.mod has no go directive")
	}
	sum, err := os.ReadFile(filepath.Join(root, "go.sum")) // #nosec G304 -- the repository's own file
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	mod := fmt.Sprintf("module docblocks\n\ngo %s\n\nrequire github.com/azrtydxb/momo v0.0.0\n\nreplace github.com/azrtydxb/momo => %s\n", m[1], filepath.ToSlash(root))
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), sum, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// goIn runs the go command in the scratch module. -mod=mod lets go add the
// requirements of this module's dependencies to the scratch go.mod.
func goIn(dir string, extraEnv []string, args ...string) ([]byte, error) {
	cmd := exec.Command("go", args...) //nolint:gosec // fixed go subcommands, package paths generated by writeBlock
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "GOFLAGS=-mod=mod"), extraEnv...)
	return cmd.CombinedOutput()
}

// writeBlock places block n in its own package of the scratch module and
// returns its package path relative to it.
func writeBlock(t *testing.T, mod string, n int, src string) string {
	t.Helper()
	name := fmt.Sprintf("b%03d", n)
	dir := filepath.Join(mod, name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return "./" + name
}

func TestDocsCodeBlocks(t *testing.T) {
	root := repoRoot(t)
	mod := blockModule(t, root)
	blocks := docBlocks(t, root)
	if len(blocks) == 0 {
		t.Fatal("no go code blocks found under docs/")
	}
	programs, fragments := 0, 0
	for n, b := range blocks {
		switch {
		case b.fragment:
			fragments++
			if isProgram(b.src) {
				t.Errorf("%s is marked as a fragment but is a complete program: remove the marker", b.where(root))
				continue
			}
			if err := parsesAsGo(b.src); err != nil {
				t.Errorf("%s: fragment does not parse as Go: %v", b.where(root), err)
			}
		default:
			programs++
			if !isProgram(b.src) {
				t.Errorf("%s: a go block must be a complete package main program (it is compiled); if it is only a snippet, put %s on the line before the fence (maxFragmentBlocks in codeblocks_test.go caps those)", b.where(root), markerFragment)
				continue
			}
			pkg := writeBlock(t, mod, n, b.src)
			if out, err := goIn(mod, nil, "build", "-o", os.DevNull, pkg); err != nil {
				t.Errorf("%s does not compile:\n%s", b.where(root), out)
			}
		}
	}
	t.Logf("docs go blocks: %d complete programs (compiled), %d fragments (parsed only, ceiling %d)", programs, fragments, maxFragmentBlocks)
	if fragments > maxFragmentBlocks {
		t.Errorf("%d fragment blocks exceed the ceiling of %d: turn fragments into complete package main programs, or raise maxFragmentBlocks by exactly the number you could not", fragments, maxFragmentBlocks)
	}
	if programs == 0 {
		t.Error("no complete program blocks: the compile check proves nothing")
	}
}
