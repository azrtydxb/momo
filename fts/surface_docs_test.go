// SPDX-License-Identifier: Apache-2.0

package fts_test

import (
	"os"
	"strings"
	"testing"
)

// Catches: the score-comparability statement and the per-dialect comparison
// table being removed from the docs.
func TestFullTextDocsStatements(t *testing.T) {
	doc, err := os.ReadFile("../docs/fulltext.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(doc)
	for _, want := range []string{
		"comparable only within one dialect and one query",
		"| Index type",
		"| Minimum token length",
		"| Transaction visibility",
		"FTS_DOC_ID",
		"'rebuild'",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("docs/fulltext.md lost %q", want)
		}
	}
}
