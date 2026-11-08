// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen/naming"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "momo.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestTypesSection catches a types: section that is dropped on the way to the
// generator config, or that accepts a key other than time.Time.
func TestTypesSection(t *testing.T) {
	f, err := Load(writeConfig(t, "version: 1\nnaming: prisma\ntypes:\n  time.Time: timestamp(3)\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ToConfig(f, ".")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Naming.(naming.Prisma); !ok {
		t.Fatalf("naming = %T, want naming.Prisma", cfg.Naming)
	}
	if cfg.TypeDefaults["time.Time"] != "timestamp(3)" {
		t.Fatalf("TypeDefaults = %v", cfg.TypeDefaults)
	}
	for _, body := range []string{
		"version: 1\ntypes:\n  string: citext\n",
		"version: 1\ntypes:\n  time.Time: \"\"\n",
		"version: 1\ntypes:\n  time.Time: \"timestamp(3); DROP TABLE x\"\n",
		"version: 1\ntypes:\n  time.Time: \"timestamp(3)'\"\n",
		"version: 1\ntypes:\n  time.Time: \"\\\"timestamp\\\"\"\n",
		"version: 1\ntypes:\n  time.Time: text\n",
		"version: 1\ntypes:\n  time.Time: timestamp(7)\n",
	} {
		if _, err := Load(writeConfig(t, body)); err == nil || !strings.Contains(err.Error(), "types:") {
			t.Errorf("Load(%q) err = %v, want a types: error", body, err)
		}
	}
}
