// SPDX-License-Identifier: Apache-2.0

package migratecfg_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/internal/migratecfg"
)

// A malformed second document must surface its YAML syntax error, not the
// "multiple documents" message that would hide the real cause.
func TestReadSecondDocumentSyntaxError(t *testing.T) {
	_, err := migratecfg.Read(strings.NewReader("version: 1\n---\n: : bad [\n"), "momo.yaml")
	if err == nil {
		t.Fatal("Read accepted a malformed second document")
	}
	if strings.Contains(err.Error(), "multiple YAML documents") {
		t.Fatalf("syntax error reported as multiple documents: %v", err)
	}
	if !strings.HasPrefix(err.Error(), "momo.yaml: ") || !strings.Contains(err.Error(), "yaml: ") {
		t.Fatalf("error = %q, want the name prefix and the YAML syntax detail", err)
	}
}

// A well-formed second document is still rejected as multiple documents.
func TestReadValidSecondDocumentIsMultiple(t *testing.T) {
	_, err := migratecfg.Read(strings.NewReader("version: 1\n---\nversion: 1\n"), "momo.yaml")
	if err == nil || !strings.Contains(err.Error(), "multiple YAML documents are not supported") {
		t.Fatalf("error = %v, want multiple documents", err)
	}
}
