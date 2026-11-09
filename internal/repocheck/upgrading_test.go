// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The moves files under testdata/moves/ record every identifier a split of
// the root package moves. One file per new package, named <package>.txt; each
// line is "<old> <new>" with the old identifier as written after "momo." and
// the new one as written after "<package>.". Blank lines and lines starting
// with "#" are ignored. docs/upgrading.md must list exactly these pairs in
// the table of its "Upgrading to 0.4.0" section: a row is
//
//	| `momo.<old>` | `github.com/azrtydxb/momo/<package>` | `<package>.<new>` |
//
// TestUpgradingCoversMoves fails for a pair missing from the page and for a
// row of the page that no moves file backs.

var (
	identRe    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)
	pkgNameRe  = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	upgradeRow = regexp.MustCompile("^\\|\\s*`([^`]+)`\\s*\\|\\s*`([^`]+)`\\s*\\|\\s*`([^`]+)`\\s*\\|\\s*$")
)

const upgradeHeading = "## Upgrading to 0.4.0"

// movePair is one row of the identifier table.
type movePair struct{ old, path, new string }

func (p movePair) String() string {
	return fmt.Sprintf("| `%s` | `%s` | `%s` |", p.old, p.path, p.new)
}

// parseMoves reads the text of one moves file for package pkg.
func parseMoves(pkg, text string) ([]movePair, error) {
	if !pkgNameRe.MatchString(pkg) {
		return nil, fmt.Errorf("%q is not a package name; name the file <package>.txt", pkg)
	}
	var out []movePair
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 2 || !identRe.MatchString(f[0]) || !identRe.MatchString(f[1]) {
			return nil, fmt.Errorf("line %d: want \"<old> <new>\" with two identifiers, got %q", i+1, line)
		}
		out = append(out, movePair{"momo." + f[0], modPath + "/" + pkg, pkg + "." + f[1]})
	}
	return out, nil
}

// parseUpgradeRows returns the rows of the identifier table in the section
// "Upgrading to 0.4.0" of the page text.
func parseUpgradeRows(text string) []movePair {
	var out []movePair
	in := false
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			in = strings.TrimSpace(line) == upgradeHeading
		case in:
			if m := upgradeRow.FindStringSubmatch(line); m != nil {
				out = append(out, movePair{m[1], m[2], m[3]})
			}
		}
	}
	return out
}

// diffMoves compares the pairs the moves files require with the rows the page
// lists and returns one message per difference, sorted.
func diffMoves(moves, page []movePair) []string {
	have := map[movePair]bool{}
	for _, p := range page {
		have[p] = true
	}
	want := map[movePair]bool{}
	var msgs []string
	for _, p := range moves {
		want[p] = true
		if !have[p] {
			msgs = append(msgs, "docs/upgrading.md lacks the row "+p.String())
		}
	}
	seen := map[movePair]bool{}
	for _, p := range page {
		if seen[p] {
			msgs = append(msgs, "docs/upgrading.md lists the row twice: "+p.String())
		}
		seen[p] = true
		if !want[p] {
			msgs = append(msgs, "docs/upgrading.md lists a move that no testdata/moves file records: "+p.String())
		}
	}
	sort.Strings(msgs)
	return msgs
}

func TestUpgradingCoversMoves(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "internal", "repocheck", "testdata", "moves")
	var moves []movePair
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304 -- files of the repository's own testdata
		if err != nil {
			t.Fatal(err)
		}
		ps, err := parseMoves(strings.TrimSuffix(e.Name(), ".txt"), string(b))
		if err != nil {
			t.Errorf("testdata/moves/%s: %v", e.Name(), err)
			continue
		}
		moves = append(moves, ps...)
	}
	page := readRepoFile(t, "docs", "upgrading.md")
	if !strings.Contains(page, "\n"+upgradeHeading+"\n") {
		t.Fatalf("docs/upgrading.md has no %q heading", upgradeHeading)
	}
	for _, m := range diffMoves(moves, parseUpgradeRows(page)) {
		t.Error(m)
	}
}

// TestUpgradingDiffDetectsDrift pins the comparison itself: a pair missing
// from the page and a row without a moves file are each reported, and matching
// sets are clean.
func TestUpgradingDiffDetectsDrift(t *testing.T) {
	moves, err := parseMoves("arrayop", "# comment\n\nArrayContains Contains\nTx.AdvisoryXactLock XactLock\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(moves) != 2 {
		t.Fatalf("parsed %d pairs, want 2", len(moves))
	}
	page := parseUpgradeRows("# Upgrading\n\n" + upgradeHeading + "\n\n| Old | Path | New |\n| --- | --- | --- |\n" +
		moves[0].String() + "\n" +
		"| `momo.Stale` | `github.com/azrtydxb/momo/agg` | `agg.Stale` |\n\n## Other\n" +
		"| `momo.Outside` | `github.com/azrtydxb/momo/agg` | `agg.Outside` |\n")
	got := diffMoves(moves, page)
	if len(got) != 2 {
		t.Fatalf("want a missing row and a stale row, got %v", got)
	}
	if !strings.Contains(got[0]+got[1], "lacks the row") || !strings.Contains(got[0]+got[1], "momo.Stale") {
		t.Errorf("unexpected messages %v", got)
	}
	dup := diffMoves(moves, []movePair{moves[0], moves[1], moves[1]})
	if len(dup) != 1 || !strings.Contains(dup[0], "twice") {
		t.Errorf("a duplicated row must be reported once as a duplicate, got %v", dup)
	}
	if clean := diffMoves(moves, moves); len(clean) != 0 {
		t.Errorf("identical sets reported %v", clean)
	}
	if _, err := parseMoves("arrayop", "OnlyOneField\n"); err == nil {
		t.Error("a line with one field must be refused")
	}
	if _, err := parseMoves("Bad-Name", "A B\n"); err == nil {
		t.Error("a file name that is not a package name must be refused")
	}
}
