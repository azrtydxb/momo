// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"strings"
	"testing"
)

// TestHashTemplateDBCoversEveryInput catches a template database name that
// ignores one of its inputs: the schema template name (migrations, extension
// list, momo version), the server version or an extension version. Each would
// let a template built under other conditions be cloned.
func TestHashTemplateDBCoversEveryInput(t *testing.T) {
	base := hashTemplateDB("momotest_tpl_aaaa", "170002", []string{"vector=0.8.0"})
	if !strings.HasPrefix(base, templateDBPrefix) {
		t.Fatalf("name %q lacks the prefix %q", base, templateDBPrefix)
	}
	if again := hashTemplateDB("momotest_tpl_aaaa", "170002", []string{"vector=0.8.0"}); again != base {
		t.Fatalf("the same inputs gave %s and %s", base, again)
	}
	for name, got := range map[string]string{
		"schema template": hashTemplateDB("momotest_tpl_bbbb", "170002", []string{"vector=0.8.0"}),
		"server version":  hashTemplateDB("momotest_tpl_aaaa", "160004", []string{"vector=0.8.0"}),
		"extension":       hashTemplateDB("momotest_tpl_aaaa", "170002", []string{"vector=0.8.1"}),
		"extension set":   hashTemplateDB("momotest_tpl_aaaa", "170002", nil),
	} {
		if got == base {
			t.Errorf("a changed %s kept the name %s", name, base)
		}
	}
}
