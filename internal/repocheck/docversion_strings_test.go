// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// versionAllowed names the forms in which a vX.Y.Z string may appear in the
// docs. Every other occurrence is a stale or hand-copied version.
const versionAllowed = "a line with the " + versionMarker + " marker, a line inside a " +
	blockStart + " block, a line under an Upgrading, Since, Changelog or History heading, " +
	"the go.mod Go floor, the sqlite-vec pin (SQLiteVecVersion in driver/sqlitedriver), " +
	"the go.mod version of a module the line names, or an entry in internal/repocheck/testdata/version_allow.txt"

var (
	versionRe     = regexp.MustCompile(`\d+\.\d+\.\d+`)
	headingRe     = regexp.MustCompile("^(#{1,6})[ \t]+(.*)$")
	historicalRe  = regexp.MustCompile(`(?i)\b(upgrading|since|changelog|history)\b`)
	goModDepRe    = regexp.MustCompile(`^\s*(?:require\s+)?(\S+\.\S+/\S+)\s+(v\d+\.\d+\.\d+\S*)\s*(?://.*)?$`)
	goModGoRe     = regexp.MustCompile(`(?m)^go (\d+\.\d+\.\d+)\s*$`)
	sqliteVecRe   = regexp.MustCompile(`(?m)^const SQLiteVecVersion = "v?(\d+\.\d+\.\d+)"`)
	allowEntryRe  = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)\s*\|\s*(\S.*)$`)
	fenceMarkerRe = regexp.MustCompile("^\\s*(```|~~~)")
)

// docVersionEnv holds the sources a docs version may legitimately come from.
// Versions are stored without a leading v.
type docVersionEnv struct {
	goFloor string
	sqlVec  string
	deps    map[string]string // module path -> version
	allow   map[string]bool
}

func isWordOrDot(c byte) bool {
	return c == '.' || c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// versionsOnLine returns the X.Y.Z strings on a line without a leading v.
// A match inside a longer dotted number (127.0.0.1) or a word is skipped.
func versionsOnLine(line string) []string {
	var out []string
	for _, loc := range versionRe.FindAllStringIndex(line, -1) {
		s, e := loc[0], loc[1]
		if s > 0 {
			p := line[s-1]
			if p == 'v' {
				if s > 1 && isWordOrDot(line[s-2]) {
					continue
				}
			} else if isWordOrDot(p) {
				continue
			}
		}
		if e+1 < len(line) && line[e] == '.' && line[e+1] >= '0' && line[e+1] <= '9' {
			continue
		}
		out = append(out, line[s:e])
	}
	return out
}

// checkDocVersions returns one problem per version string in text that is not
// one of the allowed forms.
func checkDocVersions(file, text string, env docVersionEnv) []string {
	var problems []string
	inBlock, inFence := false, false
	histLevel := 0 // heading level of the open historical section, 0 if none
	for i, line := range strings.Split(text, "\n") {
		switch {
		case strings.Contains(line, blockStart):
			inBlock = true
			continue
		case strings.Contains(line, blockEnd):
			inBlock = false
			continue
		}
		if fenceMarkerRe.MatchString(line) {
			inFence = !inFence
		} else if !inFence {
			if m := headingRe.FindStringSubmatch(line); m != nil {
				level := len(m[1])
				if histLevel != 0 && level <= histLevel {
					histLevel = 0
				}
				if histLevel == 0 && historicalRe.MatchString(m[2]) {
					histLevel = level
				}
			}
		}
		found := versionsOnLine(line)
		if len(found) == 0 || inBlock || histLevel != 0 || strings.Contains(line, versionMarker) {
			continue
		}
		// A line that names a go.mod module is a dependency row: its
		// versions must be that module's version (or the Go floor).
		var named []string
		for path := range env.deps {
			if strings.Contains(line, path) {
				named = append(named, path)
			}
		}
		for _, v := range found {
			ok := false
			if len(named) > 0 {
				ok = v == env.goFloor
				for _, p := range named {
					ok = ok || env.deps[p] == v
				}
			} else {
				ok = v == env.goFloor || v == env.sqlVec || env.allow[v]
			}
			if !ok {
				problems = append(problems, fmt.Sprintf("%s:%d names %s, which is not a single-source version; allowed forms: %s", file, i+1, v, versionAllowed))
			}
		}
	}
	return problems
}

func loadDocVersionEnv(t *testing.T, root string) docVersionEnv {
	t.Helper()
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	env := docVersionEnv{deps: map[string]string{}, allow: map[string]bool{}}
	mod := read("go.mod")
	m := goModGoRe.FindStringSubmatch(mod)
	if m == nil {
		t.Fatal("go.mod has no go directive with a patch version")
	}
	env.goFloor = m[1]
	for _, line := range strings.Split(mod, "\n") {
		if d := goModDepRe.FindStringSubmatch(line); d != nil {
			if v := versionRe.FindString(d[2]); v != "" {
				env.deps[d[1]] = v
			}
		}
	}
	sv := sqliteVecRe.FindStringSubmatch(read("driver/sqlitedriver/vec.go"))
	if sv == nil {
		t.Fatal("driver/sqlitedriver/vec.go has no SQLiteVecVersion constant")
	}
	env.sqlVec = sv[1]
	for i, line := range strings.Split(read("internal/repocheck/testdata/version_allow.txt"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		a := allowEntryRe.FindStringSubmatch(line)
		if a == nil {
			t.Errorf("version_allow.txt:%d must read '<version> | <reason>': %q", i+1, line)
			continue
		}
		env.allow[a[1]] = true
	}
	return env
}

func scannedDocs(t *testing.T, root string) []string {
	t.Helper()
	files := []string{"README.md", "llms.txt"}
	md, err := filepath.Glob(filepath.Join(root, "docs", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range md {
		files = append(files, "docs/"+filepath.Base(p))
	}
	err = filepath.WalkDir(filepath.Join(root, "examples"), func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "README.md" {
			rel, rerr := filepath.Rel(root, p)
			if rerr != nil {
				return rerr
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

// TestDocsVersionStrings fails on any version string in the user-facing docs
// that does not come from a single source, so a release or a dependency bump
// cannot leave a stale number behind in prose.
func TestDocsVersionStrings(t *testing.T) {
	root := repoRoot(t)
	env := loadDocVersionEnv(t, root)
	carriers := map[string]bool{}
	for _, f := range scannedDocs(t, root) {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		text := string(b)
		if strings.Contains(text, versionMarker) || strings.Contains(text, blockStart) {
			carriers[f] = true
		}
		for _, p := range checkDocVersions(f, text, env) {
			t.Error(p)
		}
	}

	cb, err := os.ReadFile(filepath.Join(root, "release-please-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Packages map[string]struct {
			ExtraFiles []string `json:"extra-files"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(cb, &cfg); err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, f := range cfg.Packages["."].ExtraFiles {
		listed[f] = true
	}
	for f := range carriers {
		if !listed[f] {
			t.Errorf("%s carries a release-please marker but is not in extra-files of release-please-config.json, so release-please would never update it", f)
		}
	}
}

// TestDocsVersionRules pins the rules TestDocsVersionStrings applies, on
// synthetic text, so a loosened rule cannot pass unnoticed.
func TestDocsVersionRules(t *testing.T) {
	env := docVersionEnv{
		goFloor: "1.26.10",
		sqlVec:  "0.1.6",
		deps:    map[string]string{"github.com/jackc/pgx/v5": "5.12.0"},
		allow:   map[string]bool{"0.8.7": true},
	}
	cases := []struct {
		name string
		text string
		bad  int
	}{
		{"unmarked version", "Released as v0.9.9 today.\n", 1},
		{"marked line", "Released as v0.9.9 today. <!-- x-release-please-version -->\n", 0},
		{"marked block", "<!-- x-release-please-start-version -->\nv0.9.9\n<!-- x-release-please-end -->\nv0.9.9\n", 1},
		{"upgrading heading", "## Upgrading: header\nEarlier wrote v0.1.0-dev here.\n", 0},
		{"section ends at next heading", "## Upgrading\nv0.1.0\n## Other\nv0.1.0\n", 1},
		{"subheading stays historical", "## Changelog\n### Notes\nv0.2.0\n", 0},
		{"heading in a code fence is not a section", "```sh\n# Upgrading\n```\nv0.1.0\n", 1},
		{"go floor", "Needs Go 1.26.10 or newer.\n", 0},
		{"other go patch", "Needs Go 1.26.9 or newer.\n", 1},
		{"sqlite-vec pin", "sqlite-vec v0.1.6 is linked.\n", 0},
		{"allow file entry", "pgvector 0.8.7\n", 0},
		{"version not in the allow file", "pgvector 0.8.6\n", 1},
		{"dependency row agrees", "| `github.com/jackc/pgx/v5` | v5.12.0 |\n", 0},
		{"dependency row disagrees", "| `github.com/jackc/pgx/v5` | v5.11.0 |\n", 1},
		{"dependency row with an allowed but wrong version", "| `github.com/jackc/pgx/v5` | 0.8.7 |\n", 1},
		{"ip address is not a version", "host 127.0.0.1 and 10.0.0.12\n", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkDocVersions("doc.md", tc.text, env)
			if len(got) != tc.bad {
				t.Fatalf("got %d problems, want %d: %v", len(got), tc.bad, got)
			}
			for _, p := range got {
				if !strings.HasPrefix(p, "doc.md:") || !strings.Contains(p, "allowed forms") {
					t.Errorf("problem lacks file, line or the allowed forms: %q", p)
				}
			}
		})
	}
}
