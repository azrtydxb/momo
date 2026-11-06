// SPDX-License-Identifier: Apache-2.0

package naming

import "testing"

func TestTitleASCIIEmpty(t *testing.T) {
	if got := titleASCII(""); got != "" {
		t.Fatalf("titleASCII(\"\") = %q", got)
	}
	if got := titleASCII("uRL"); got != "Url" {
		t.Fatalf("titleASCII = %q", got)
	}
}

func TestInitialismReplacerSkipsEmpty(t *testing.T) {
	r := newInitialismReplacer([]string{"", "API"})
	if got := r.Replace("userAPIKey"); got != "userApiKey" {
		t.Fatalf("Replace = %q", got)
	}
}
