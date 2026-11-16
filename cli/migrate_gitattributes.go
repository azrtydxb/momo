// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/azrtydxb/momo/migrate"
)

// warnNoUnionRule prints a warning when dir is inside a git work tree and no
// .gitattributes rule gives its momo.sum the merge=union driver. Without it two
// branches that each add a migration conflict on momo.sum. It never fails the
// command: a project may resolve that conflict by hand.
func (l *cli) warnNoUnionRule(dir string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	root := gitWorkTree(abs)
	if root == "" {
		return
	}
	sum := filepath.Join(abs, migrate.SumFileName)
	for d := abs; ; d = filepath.Dir(d) {
		if rel, err := filepath.Rel(d, sum); err == nil && attributesGiveUnion(filepath.Join(d, ".gitattributes"), filepath.ToSlash(rel)) {
			return
		}
		if d == root || d == filepath.Dir(d) {
			break
		}
	}
	_, _ = fmt.Fprintf(l.errOut, "momo: warning: no .gitattributes rule %q covers %s; two branches that each add a migration will conflict on it (see docs/migrations.md, Parallel branches)\n",
		"<dir>/momo.sum merge=union", l.rel(sum))
}

// gitWorkTree returns the closest ancestor of dir (or dir itself) that holds a
// .git entry, or "" when dir is not inside a git work tree.
func gitWorkTree(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		if d == filepath.Dir(d) {
			return ""
		}
	}
}

// attributesGiveUnion reports whether the .gitattributes file gives merge=union
// to the slash-separated path rel, relative to the file's directory.
func attributesGiveUnion(file, rel string) bool {
	f, err := os.Open(file) // #nosec G304 -- a .gitattributes next to the migration directory
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		union := false
		for _, a := range fields[1:] {
			if a == "merge=union" {
				union = true
			}
		}
		if union && attributePatternMatches(fields[0], rel) {
			return true
		}
	}
	return false
}

// attributePatternMatches matches a .gitattributes pattern against rel: a
// pattern without a slash matches the file name at any depth; one with a slash
// is anchored at the file's directory, and ** matches any number of directories.
func attributePatternMatches(pattern, rel string) bool {
	if !strings.Contains(strings.TrimPrefix(pattern, "/"), "/") {
		ok, _ := path.Match(strings.TrimPrefix(pattern, "/"), path.Base(rel))
		return ok
	}
	return globSegments(strings.Split(strings.TrimPrefix(pattern, "/"), "/"), strings.Split(rel, "/"))
}

func globSegments(pat, name []string) bool {
	if len(pat) == 0 {
		return len(name) == 0
	}
	if pat[0] == "**" {
		for i := 0; i <= len(name); i++ {
			if globSegments(pat[1:], name[i:]) {
				return true
			}
		}
		return false
	}
	if len(name) == 0 {
		return false
	}
	ok, _ := path.Match(pat[0], name[0])
	return ok && globSegments(pat[1:], name[1:])
}
