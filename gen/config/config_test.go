// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/naming"
)

func TestConfigLoad(t *testing.T) {
	f, err := Load("testdata/momo.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if f.Naming != "gorm" {
		t.Fatalf("naming = %q, want gorm", f.Naming)
	}
	if f.Plugins["viz"]["format"] != "mermaid" {
		t.Fatalf("viz format = %v, want mermaid", f.Plugins["viz"]["format"])
	}
	if f.Migrate.Dir["postgres"] != "migrations/postgres" || f.Migrate.URLEnv["postgres"] != "MOMO_POSTGRES_URL" {
		t.Fatalf("migrate dir/url_env = %v / %v", f.Migrate.Dir, f.Migrate.URLEnv)
	}
	if f.Migrate.LockTimeout != "5s" || f.Migrate.StatementTimeout != "30s" {
		t.Fatalf("migrate timeouts = %q, %q", f.Migrate.LockTimeout, f.Migrate.StatementTimeout)
	}
	if len(f.Migrate.Schemas) != 1 || f.Migrate.Schemas[0] != "public" || len(f.Migrate.Exclude) != 1 || f.Migrate.Exclude[0] != "pg_*" {
		t.Fatalf("migrate schemas/exclude = %v / %v", f.Migrate.Schemas, f.Migrate.Exclude)
	}
	_, err = Load("testdata/bad.yaml")
	if err == nil || !strings.Contains(err.Error(), "field pakages not found") {
		t.Fatalf("bad.yaml: want unknown-field error, got %v", err)
	}
	cfg, err := ToConfig(f, "testdata")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Naming.(naming.GORM); !ok {
		t.Fatalf("naming = %T, want naming.GORM", cfg.Naming)
	}
}

func TestConfigMigrateValidation(t *testing.T) {
	write := func(t *testing.T, body string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "momo.yaml")
		if err := os.WriteFile(p, []byte("version: 1\n"+body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tests := []struct {
		name, body, want string
	}{
		{"bad lock_timeout", "migrate:\n  lock_timeout: soon\n", "lock_timeout"},
		{"bad statement_timeout", "migrate:\n  statement_timeout: 5 parsecs\n", "statement_timeout"},
		{"unknown dir dialect", "migrate:\n  dir:\n    oracle9: migrations/x\n", "oracle9"},
		{"unknown url_env dialect", "migrate:\n  url_env:\n    mongo: MONGO_URL\n", "mongo"},
		{"unknown migrate field", "migrate:\n  dirs: {}\n", "field dirs not found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(write(t, tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load err = %v, want containing %q", err, tc.want)
			}
		})
	}
	if _, err := Load(write(t, "migrate:\n  lock_timeout: 5s\n  dir:\n    postgres: m\n")); err != nil {
		t.Fatalf("valid migrate section: %v", err)
	}
}

func TestNamingOverridesFromConfig(t *testing.T) {
	dir, err := filepath.Abs("testdata/overrides")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: dir, Naming: naming.GORM{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	if len(g.Models) != 1 {
		t.Fatalf("models = %d, want 1", len(g.Models))
	}
	cols := map[string]bool{}
	for _, f := range g.Models[0].Fields {
		cols[f.Column] = true
	}
	if !cols["custom_name"] {
		t.Fatalf("explicit column tag lost; have %v", cols)
	}
	if want := (naming.GORM{}).ColumnName("ItemCode"); !cols[want] {
		t.Fatalf("preset column %s missing; have %v", want, cols)
	}
	if cols["name"] {
		t.Fatal("the preset overrode an explicit column tag")
	}
}

func TestLoadHostile(t *testing.T) {
	write := func(t *testing.T, body string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "momo.yaml")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tests := []struct {
		name, body, want string
	}{
		{"empty file", "", "empty"},
		{"unknown top-level key", "version: 1\nbogus: 1\n", "field bogus not found"},
		{"version 2", "version: 2\n", "unsupported momo.yaml version 2"},
		{"unknown preset", "version: 1\nnaming: rails\n", "rails"},
		{"unknown dialect", "version: 1\ndialects: [oracle9]\n", "oracle9"},
		{"lock_timeout names key", "version: 1\nmigrate:\n  lock_timeout: 5\n", "migrate.lock_timeout"},
		{"statement_timeout names key", "version: 1\nmigrate:\n  statement_timeout: x\n", "migrate.statement_timeout"},
		{"dir dialect names key", "version: 1\nmigrate:\n  dir:\n    oracle9: x\n", "migrate.dir"},
		{"url_env dialect names key", "version: 1\nmigrate:\n  url_env:\n    mongo: X\n", "migrate.url_env"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(write(t, tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load err = %v, want containing %q", err, tc.want)
			}
		})
	}
	missing := filepath.Join(t.TempDir(), "nope", "momo.yaml")
	if _, err := Load(missing); err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("missing path: err = %v, want naming %s", err, missing)
	}
	f, err := Load(write(t, "version: 1\ndialects: [postgres, mysql]\ntags: [x]\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ToConfig(f, "d")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Dialects) != 2 || cfg.Dialects[1] != dialect.MySQL || cfg.Dir != "d" || cfg.Tags[0] != "x" {
		t.Fatalf("ToConfig = %+v", cfg)
	}
	if _, ok := cfg.Naming.(naming.Momo); !ok {
		t.Fatalf("default naming = %T, want naming.Momo", cfg.Naming)
	}
}
