// SPDX-License-Identifier: Apache-2.0

package docscheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var agentRules = []string{
	"PostgreSQL with pgvector is the reference database",
	"Run `momo generate` after every model change",
	"Never edit generated files",
	"Pass user input only as arguments",
	"Never bypass `ErrUnfilteredMutation`",
	"Check errors with `errors.Is`",
	"Never edit a migration that has been applied",
	"Run `momo generate --check` in CI",
	"Run `momo migrate validate` in CI",
	"Use only the `tx` handle inside `db.Tx`",
	"Test against a real PostgreSQL",
}

func TestAgentGuideSnippet(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "agents.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	const start, end = "<!-- momo:agents-snippet:start -->", "<!-- momo:agents-snippet:end -->"
	if strings.Count(text, start) != 1 || strings.Count(text, end) != 1 {
		t.Fatal("docs/agents.md must contain exactly one agents-snippet block")
	}
	i, j := strings.Index(text, start), strings.Index(text, end)
	if j < i {
		t.Fatal("docs/agents.md has the agents-snippet markers in the wrong order")
	}
	snippet := text[i+len(start) : j]
	for _, rule := range agentRules {
		if !strings.Contains(snippet, rule) {
			t.Errorf("the AGENTS.md snippet lacks the rule %q", rule)
		}
	}
	if !strings.Contains(text, "ai-guide.md") {
		t.Error("docs/agents.md must point at the full agent guide, ai-guide.md")
	}
}
