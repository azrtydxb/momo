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
	// Git reads the .gitattributes of every directory from the work tree root
	// down to the file's, and the last matching line that sets merge wins, so
	// walk from the root down and let a later (closer, lower) line override.
	var dirs []string
	for d := abs; ; d = filepath.Dir(d) {
		dirs = append(dirs, d)
		if d == root || d == filepath.Dir(d) {
			break
		}
	}
	union := false
	for i := len(dirs) - 1; i >= 0; i-- {
		rel, err := filepath.Rel(dirs[i], sum)
		if err != nil {
			continue
		}
		if u, set := attributesMerge(filepath.Join(dirs[i], ".gitattributes"), filepath.ToSlash(rel)); set {
			union = u
		}
	}
	if union {
		return
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

// attributesMerge reads the merge attribute that the .gitattributes file sets
// for the slash-separated path rel, relative to the file's directory: union
// reports merge=union, and set reports that some line set merge at all, to
// union or to anything else (merge=text, merge, -merge, !merge). The last
// matching line wins, as in git. [attr] macros are not supported: a macro that
// expands to merge=union is not seen, and the warning is then printed anyway.
func attributesMerge(file, rel string) (union, set bool) {
	f, err := os.Open(file) // #nosec G304 -- a .gitattributes next to the migration directory
	if err != nil {
		return false, false
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") || !attributePatternMatches(fields[0], rel) {
			continue
		}
		for _, a := range fields[1:] {
			switch {
			case a == "merge=union":
				union, set = true, true
			case a == "merge" || a == "-merge" || a == "!merge" || strings.HasPrefix(a, "merge="):
				union, set = false, true
			}
		}
	}
	return union, set
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
