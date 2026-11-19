// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// apidiffReport is shaped like the output of "apidiff -m" (see objectString in
// golang.org/x/exp/apidiff): a root-package name is bare, a child-package name
// has a "./" prefix, a removed package has the full import path, and a name in
// a package outside the module has its full import path.
const apidiffReport = `Incompatible changes:
- ./gen/graphql.Plugin: removed
- ./gen/grpc.(*Plugin).Generate: removed
- ./gen/grpc.Lock: changed from struct{} to struct{ X int }
- ./gen/openapi.Options.Title: removed
- package github.com/azrtydxb/momo/importer/gorm: removed
- package github.com/azrtydxb/momo/gen/apiquery: removed
- ./importer/history.Raw: removed
- ./gen/graphqlx.Foo: removed
- ./migrate.Engine: removed
- package github.com/azrtydxb/momo/migrate/sqlite: removed
- Select: removed
- (*DB).Begin: removed
- golang.org/x/other.T: changed from int to string
Compatible changes:
- ./gen/grpc.NewThing: added
- ./migrate.NewThing: added
`

var apidiffCovered = []string{
	"./gen/graphqlx.Foo: removed",
	"./migrate.Engine: removed",
	"package github.com/azrtydxb/momo/migrate/sqlite: removed",
	"Select: removed",
	"(*DB).Begin: removed",
	"golang.org/x/other.T: changed from int to string",
}

// TestAPIDiffIncompatibleFilter runs scripts/apidiff-incompatible.sh, the
// filter of the compatibility gate, over real-shaped apidiff output.
func TestAPIDiffIncompatibleFilter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the gate scripts run on Linux and macOS")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "apidiff-incompatible.sh")
	dir := t.TempDir()
	report := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(report, []byte(apidiffReport), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(excluded string) []string {
		t.Helper()
		args := []string{script, report, "github.com/azrtydxb/momo"}
		if excluded != "" {
			args = append(args, excluded)
		}
		out, err := exec.Command(bash, args...).CombinedOutput() // #nosec G204 -- fixed test arguments
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		var lines []string
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if l != "" {
				lines = append(lines, strings.TrimPrefix(l, "- "))
			}
		}
		return lines
	}

	list := filepath.Join(dir, "experimental.txt")
	listing := "# comment\n\ngen/graphql\ngen/grpc\ngen/openapi\ngen/apiquery\nimporter\n"
	if err := os.WriteFile(list, []byte(listing), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := run(list); !reflect.DeepEqual(got, apidiffCovered) {
		t.Errorf("with the real list, got\n%q\nwant\n%q", got, apidiffCovered)
	}

	// The shipped list excludes what the policy page names.
	shipped := run(filepath.Join(root, "scripts", "apidiff-experimental.txt"))
	if !reflect.DeepEqual(shipped, apidiffCovered) {
		t.Errorf("with scripts/apidiff-experimental.txt, got\n%q\nwant\n%q", shipped, apidiffCovered)
	}

	// An empty list, a list of comments and a missing file exclude nothing, and
	// the compatible section is never printed.
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	comments := filepath.Join(dir, "comments.txt")
	if err := os.WriteFile(comments, []byte("# nothing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, file := range map[string]string{"empty": empty, "comments": comments, "missing": filepath.Join(dir, "absent.txt"), "none": ""} {
		got := run(file)
		if len(got) != 13 {
			t.Errorf("%s list: got %d incompatible lines, want 13: %q", name, len(got), got)
		}
		for _, l := range got {
			if strings.Contains(l, "NewThing") {
				t.Errorf("%s list: compatible change printed: %s", name, l)
			}
		}
	}
}
