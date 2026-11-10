// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestDocsWorkflow pins the docs workflow: it builds and checks the site on
// pull requests, pushes to main and sprint branches and on worktree-agent
// branches, and publishes with mike only from main (/dev/) and version tags.
func TestDocsWorkflow(t *testing.T) {
	w := loadWorkflow(t, "docs.yml")
	push, ok := w.On["push"].(map[string]any)
	if !ok {
		t.Fatal("docs.yml must run on push")
	}
	for _, b := range []string{"main", "sprint-*", "worktree-agent-*"} {
		if !matrixHas(push["branches"], b) {
			t.Errorf("docs.yml must build on pushes to %s", b)
		}
	}
	if _, ok := push["tags"]; !ok {
		t.Error("docs.yml must deploy releases on tags")
	}
	if _, ok := w.On["pull_request"]; !ok {
		t.Error("docs.yml must build on pull_request")
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "docs.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"mike deploy --push dev",
		"mike deploy --push --update-aliases",
		"mike set-default --push latest",
		"pip install --require-hashes -r docs/requirements.txt",
		"mkdocs build --strict",
		"TestLLMsTxt",
		"TestDocsCodeBlocks",
		"TestDocsRunnableBlocks",
		"timeout-minutes",
		"concurrency:",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("docs.yml lacks %q", want)
		}
	}
	deploy, ok := w.Jobs["deploy"]
	if !ok {
		t.Fatal("docs.yml has no deploy job")
	}
	if deploy.Permissions == nil || !strings.Contains(text, "contents: write") {
		t.Error("only the deploy job may write contents")
	}
	if !strings.Contains(deploy.If, "refs/heads/main") || !strings.Contains(deploy.If, "refs/tags/v") {
		t.Errorf("the deploy job must run only on main and version tags, got if: %q", deploy.If)
	}
	if !strings.Contains(deploy.If, "!contains(github.ref_name, '-')") {
		t.Errorf("the deploy job must skip prerelease tags, got if: %q", deploy.If)
	}
	if !strings.Contains(text, "group: docs-deploy") {
		t.Error("the deploy job needs its own concurrency group docs-deploy")
	}
	if strings.Contains(deploy.If, "worktree-agent") || strings.Contains(deploy.If, "sprint") {
		t.Errorf("the deploy job must not run on work or sprint branches, got if: %q", deploy.If)
	}
	if w.Jobs["build"].Permissions != nil {
		t.Error("the build job must use the read-only top-level permissions")
	}
	ci, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`(?i)setup-python|pip install|python`).Match(ci) {
		t.Error("ci.yml must not install Python; docs tooling runs only in docs.yml")
	}
}

// TestDocsPublishedOnRelease pins how a release reaches /vX.Y/ and latest. The
// tag release-please creates with the workflow token does not trigger a tag
// push, so release-please.yml must dispatch docs.yml with the new tag, and the
// dispatched run may deploy only a release tag, never /dev/.
func TestDocsPublishedOnRelease(t *testing.T) {
	root := repoRoot(t)
	docs, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "docs.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"workflow_dispatch:",
		"tag:",
		"github.event_name == 'workflow_dispatch' && startsWith(inputs.tag, 'v') && !contains(inputs.tag, '-')",
		"github.event_name == 'push' && github.ref == 'refs/heads/main'",
		"ref: ${{ inputs.tag || github.ref }}",
	} {
		if !strings.Contains(string(docs), want) {
			t.Errorf("docs.yml lacks %q", want)
		}
	}
	rp, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release-please.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"actions: write",
		"steps.release.outputs.release_created == 'true'",
		`gh workflow run docs.yml --repo "$GITHUB_REPOSITORY" --ref main -f tag="$TAG"`,
	} {
		if !strings.Contains(string(rp), want) {
			t.Errorf("release-please.yml lacks %q", want)
		}
	}
}
