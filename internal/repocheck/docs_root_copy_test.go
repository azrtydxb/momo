// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDocsRootCopyOnRelease pins that a release deploy also puts llms.txt and
// llms-full.txt at the gh-pages root, so https://azrtydxb.github.io/momo/llms.txt
// answers (mike publishes only under /dev/, /vX.Y/ and /latest/). The step
// runs for a release tag only, never for /dev/, and the links of the root
// llms.txt point into latest/ because the file's own links are relative to a
// version directory.
func TestDocsRootCopyOnRelease(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "docs.yml")) // #nosec G304 -- the repository's own workflow
	if err != nil {
		t.Fatal(err)
	}
	docs := string(b)
	const name = "- name: Copy llms.txt and llms-full.txt to the site root"
	i := strings.Index(docs, name)
	if i < 0 {
		t.Fatalf("docs.yml has no step %q", name)
	}
	step := docs[i:]
	if j := strings.Index(step[len(name):], "\n      - "); j >= 0 {
		step = step[:len(name)+j]
	}
	for _, want := range []string{
		"if: startsWith(github.ref, 'refs/tags/v') || inputs.tag != ''",
		"gh-pages",
		"llms.txt",
		"llms-full.txt",
		"latest/",
		"git push",
	} {
		if !strings.Contains(step, want) {
			t.Errorf("the root copy step lacks %q:\n%s", want, step)
		}
	}
	if strings.Contains(step, "refs/heads/main") {
		t.Error("the root copy step must not run for /dev/ deploys")
	}
	// It must run after the release is published, or it would copy the
	// previous release.
	if pub := strings.Index(docs, "mike set-default --push latest"); pub < 0 || pub > i {
		t.Error("the root copy step must come after mike publishes the release")
	}
}
