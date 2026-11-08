// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateExternalExtensions(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "momo.yaml")
		if err := os.WriteFile(p, []byte("version: 1\n"+body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	f, err := Load(write("migrate:\n  external_extensions: [vector, uuid-ossp]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.Migrate.ExternalExtensions, ","); got != "vector,uuid-ossp" {
		t.Fatalf("external_extensions = %q", got)
	}
	for name, list := range map[string]string{
		"empty":      `[""]`,
		"blank":      `["  "]`,
		"space":      `["pg trgm"]`,
		"qualified":  `["public.vector"]`,
		"upper case": `["Vector"]`,
		"quoted":     `["\"vector\""]`,
		"duplicate":  `[vector, vector]`,
	} {
		if _, err := Load(write("migrate:\n  external_extensions: " + list + "\n")); err == nil || !strings.Contains(err.Error(), "external_extensions") {
			t.Fatalf("%s: want an external_extensions error, got %v", name, err)
		}
	}
}
