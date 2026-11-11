// SPDX-License-Identifier: Apache-2.0

package config

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen/fromdb"
	"github.com/azrtydxb/momo/internal/migratecfg"
)

// yamlKeys lists the yaml keys of the fields of struct type t, sorted.
func yamlKeys(t reflect.Type) []string {
	var keys []string
	for i := 0; i < t.NumField(); i++ {
		if k, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ","); k != "" && k != "-" {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// Catches: migratecfg's key lists (which migrate.ConfigFromFile uses to tell a
// typo from a key that belongs to code generation) falling out of step with
// the File and fromdb.Config types the command decodes.
func TestMigratecfgKeyListsMatchTheTypes(t *testing.T) {
	var gen []string
	for _, k := range yamlKeys(reflect.TypeOf(File{})) {
		if k != "version" && k != "migrate" && k != "introspect" {
			gen = append(gen, k)
		}
	}
	if !slices.Equal(gen, migratecfg.GenKeys) {
		t.Fatalf("migratecfg.GenKeys = %v, want the generation keys of File: %v", migratecfg.GenKeys, gen)
	}
	if got := yamlKeys(reflect.TypeOf(fromdb.Config{})); !slices.Equal(got, migratecfg.IntrospectKeys) {
		t.Fatalf("migratecfg.IntrospectKeys = %v, want the keys of fromdb.Config: %v", migratecfg.IntrospectKeys, got)
	}
	if got, want := yamlKeys(reflect.TypeOf(MigrateConfig{})), []string{"dir", "exclude", "external_extensions", "lock_timeout", "schemas", "statement_timeout", "url_env"}; !slices.Equal(got, want) {
		t.Fatalf("migrate keys = %v, want %v", got, want)
	}
}
