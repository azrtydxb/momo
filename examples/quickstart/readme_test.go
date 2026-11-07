// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

const (
	readmePath = "../../README.md"
	modulePath = "github.com/azrtydxb/momo"
)

// fence is one fenced code block of the README.
type fence struct {
	lang string
	body string
}

func readmeFences(t *testing.T) []fence {
	t.Helper()
	raw, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}
	var out []fence
	var cur *fence
	for _, line := range strings.Split(string(raw), "\n") {
		switch {
		case cur == nil && strings.HasPrefix(line, "```"):
			cur = &fence{lang: strings.TrimPrefix(line, "```")}
		case cur != nil && line == "```":
			cur.body = strings.TrimRight(cur.body, "\n")
			out = append(out, *cur)
			cur = nil
		case cur != nil:
			cur.body += line + "\n"
		}
	}
	if cur != nil {
		t.Fatal("README has an unterminated code fence")
	}
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // fixed test paths
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestReadmeSnippetsAreCompiled fails when a Go or YAML snippet in the README
// is not verbatim part of the example that go build and go vet compile.
func TestReadmeSnippetsAreCompiled(t *testing.T) {
	sources := readFile(t, "main.go") + "\n" + readFile(t, "models/models.go")
	config := readFile(t, "momo.yaml")
	goBlocks := 0
	for _, f := range readmeFences(t) {
		switch f.lang {
		case "go":
			goBlocks++
			if !strings.Contains(sources, f.body) {
				t.Errorf("README Go snippet is not in examples/quickstart:\n%s", f.body)
			}
		case "yaml":
			if strings.TrimSpace(f.body) != strings.TrimSpace(config) {
				t.Errorf("README momo.yaml differs from examples/quickstart/momo.yaml:\n%s", f.body)
			}
		}
	}
	if goBlocks < 8 {
		t.Fatalf("found %d Go snippets, want at least 8: the fence parser may be broken", goBlocks)
	}
}

func momoOutput(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", modulePath + "/cmd/momo"}, args...)...) //nolint:gosec // fixed arguments
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("momo %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// TestReadmeCommandsMatchHelp checks every momo command in the README against
// the help the binary prints.
func TestReadmeCommandsMatchHelp(t *testing.T) {
	top := momoOutput(t, "help")
	migrate := momoOutput(t, "migrate", "help")
	generate := momoOutput(t, "help", "generate")
	flagRe := regexp.MustCompile(`^(--?[A-Za-z][A-Za-z-]*)`)

	seen := 0
	for _, f := range readmeFences(t) {
		if f.lang != "sh" {
			continue
		}
		for _, line := range strings.Split(f.body, "\n") {
			words := strings.Fields(line)
			if len(words) < 2 || words[0] != "momo" {
				continue
			}
			seen++
			var help string
			switch words[1] {
			case "generate":
				help = generate
				if !strings.Contains(top, "  generate ") {
					t.Errorf("%q: top-level help has no generate command", line)
				}
			case "migrate":
				help = migrate
				if len(words) < 3 || !regexp.MustCompile(`(?m)^  `+regexp.QuoteMeta(words[2])+`\b`).MatchString(migrate) {
					t.Errorf("%q: not a command in momo migrate help", line)
				}
			case "help":
			default:
				t.Errorf("%q: unknown momo command", line)
			}
			for _, w := range words[2:] {
				if m := flagRe.FindString(w); m != "" && !strings.Contains(help, m) {
					t.Errorf("%q: flag %s is not in the help output", line, m)
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("no momo commands found in README sh blocks")
	}
	// Flags and commands named in prose, in code spans.
	prose := readFile(t, readmePath)
	for _, span := range regexp.MustCompile("`(momo [^`]+)`").FindAllStringSubmatch(prose, -1) {
		words := strings.Fields(span[1])
		switch words[1] {
		case "generate":
			for _, w := range words[2:] {
				if m := flagRe.FindString(w); m != "" && !strings.Contains(generate+top, m) {
					t.Errorf("%q: flag %s is not in the help output", span[1], m)
				}
			}
		case "migrate":
			for _, w := range words[2:] {
				if m := flagRe.FindString(w); m != "" && !strings.Contains(migrate, m) {
					t.Errorf("%q: flag %s is not in momo migrate help", span[1], m)
				}
			}
		}
	}
}

// TestReadmeStatements pins the install lines and the notes the README must keep.
func TestReadmeStatements(t *testing.T) {
	readme := readFile(t, readmePath)
	var manifest map[string]string
	if err := json.Unmarshal([]byte(readFile(t, "../../.release-please-manifest.json")), &manifest); err != nil || manifest["."] == "" {
		t.Fatalf("release-please manifest has no root version: %v", err)
	}
	version := "v" + manifest["."]
	for _, want := range []string{
		"go get " + modulePath + "@" + version,
		"go install " + modulePath + "/cmd/momo@" + version,
		"PgBouncer",
		"*_gen.go text eol=lf",
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README does not contain %q", want)
		}
	}
	goMod := readFile(t, "../../go.mod")
	m := regexp.MustCompile(`(?m)^go (\d+\.\d+)`).FindStringSubmatch(goMod)
	if m == nil {
		t.Fatal("go.mod has no go line")
	}
	if !strings.Contains(readme, "Go "+m[1]+" or newer") {
		t.Errorf("README does not state the go.mod floor %q", "Go "+m[1]+" or newer")
	}
	if !strings.Contains(strings.Join(strings.Fields(readme), " "), "no compatibility promise between releases") {
		t.Error("README does not state that there is no compatibility promise")
	}
	if !strings.Contains(goMod, "module "+modulePath+"\n") {
		t.Errorf("go.mod module is not %s", modulePath)
	}
	if _, err := os.Stat("../../cmd/momo/main.go"); err != nil {
		t.Errorf("install path has no main package: %v", err)
	}
}
