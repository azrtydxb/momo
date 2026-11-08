// SPDX-License-Identifier: Apache-2.0

package docscheck

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestDocsIndexLinksEveryPage(t *testing.T) {
	root := repoRoot(t)
	idx, err := os.ReadFile(filepath.Join(root, "docs", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	pages, err := NavPages(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pages {
		if p == "README.md" {
			continue
		}
		if !strings.Contains(string(idx), "("+p+")") && !strings.Contains(string(idx), "("+p+"#") {
			t.Errorf("docs/README.md does not link %s", p)
		}
	}
}

// TestCapabilityMatrixComplete fails when a capability boolean of
// dialect.Dialect is not named in docs/capabilities.md.
func TestCapabilityMatrixComplete(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "capabilities.md"))
	if err != nil {
		t.Fatal(err)
	}
	rt := reflect.TypeFor[dialect.Dialect]()
	checked := 0
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Type.Kind() != reflect.Bool {
			continue
		}
		checked++
		if !strings.Contains(string(b), f.Name) {
			t.Errorf("docs/capabilities.md does not mention the dialect capability %s", f.Name)
		}
	}
	if checked == 0 {
		t.Fatal("dialect.Dialect has no capability booleans; the test is stale")
	}
}
