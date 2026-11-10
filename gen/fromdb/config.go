// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"errors"
	"fmt"
	"go/token"
	"path"
	"strings"

	"github.com/azrtydxb/momo/gen/tags"
)

// Config is the introspect: section of momo.yaml. It steers `momo generate
// --from-db`: what the database cannot say (Prisma updatedAt, Json shapes,
// Unsupported columns) is written here once and applied on every re-run.
type Config struct {
	// Package is the Go package name of the generated models (default: the
	// base name of the output directory).
	Package string `yaml:"package"`
	// Out is the output directory, relative to momo.yaml (default: the first
	// local entry of packages, else ./models).
	Out string `yaml:"out"`
	// Skip lists table name globs that get no model. A skipped table is also
	// left out of the init snapshot; add the same globs to migrate.exclude so
	// the baseline ignores the live table.
	Skip []string `yaml:"skip"`
	// Only lists table name globs that get a model, the inverse of Skip: a
	// table that matches none gets no model, no place in the init snapshot and
	// no place in migrate (drift, baseline and diff treat it as excluded).
	// Skip applies within Only. Empty means every table.
	Only []string `yaml:"only"`
	// Functions is "" (the default) to write only the functions that a modeled
	// table's triggers, defaults, checks or policies use, each on the model of
	// the table that uses it, or "all" to write every database function on the
	// first model.
	Functions string `yaml:"functions"`
	// DBDefault is "" (the default) to tag only NOT NULL columns with a
	// computed default dbdefault, or "all" to tag every NOT NULL column that
	// has a default.
	DBDefault string `yaml:"dbdefault"`
	// Types overrides the Go type of every column of one SQL type, keyed by
	// the normalized type, for example "inet" or "numeric(10,2)".
	Types map[string]TypeRule `yaml:"types"`
	// Columns are per-column rules, applied in order; a later rule wins.
	Columns []ColumnRule `yaml:"columns"`
	// Tables are per-table rules keyed by table name.
	Tables map[string]TableRule `yaml:"tables"`
}

// TypeRule replaces the Go type chosen for a SQL type.
type TypeRule struct {
	Go     string `yaml:"go"`     // Go type expression, for example "decimal.Decimal"
	Import string `yaml:"import"` // import path the expression needs
}

// ColumnRule matches columns by a "table.column" glob.
type ColumnRule struct {
	Match  string   `yaml:"match"`  // glob over "table.column", for example "*.updated_at"
	Go     string   `yaml:"go"`     // Go type expression
	Import string   `yaml:"import"` // import path the expression needs
	Name   string   `yaml:"name"`   // Go field name
	Tags   []string `yaml:"tags"`   // momo tag options added to the field, for example updated_at
}

// TableRule renames the Go struct of one table and adds tag options to its
// momo.Model marker.
type TableRule struct {
	Name string   `yaml:"name"` // Go struct name
	Tags []string `yaml:"tags"` // marker tag options, for example soft_delete:timestamp
}

// Validate checks the shape of the section. The returned error names the key
// at fault.
func (c *Config) Validate() error {
	if c.Package != "" && !token.IsIdentifier(c.Package) {
		return fmt.Errorf("introspect.package: %q is not a Go identifier", c.Package)
	}
	for i, g := range c.Skip {
		if _, err := path.Match(g, ""); err != nil {
			return fmt.Errorf("introspect.skip[%d]: %q: %w", i, g, err)
		}
	}
	for k, r := range c.Types {
		if strings.TrimSpace(k) == "" {
			return errors.New("introspect.types: empty SQL type key")
		}
		if err := checkGo("introspect.types."+k, r.Go, r.Import, true); err != nil {
			return err
		}
	}
	for i, r := range c.Columns {
		where := fmt.Sprintf("introspect.columns[%d]", i)
		if !strings.Contains(r.Match, ".") {
			return fmt.Errorf("%s.match: %q must be a table.column glob", where, r.Match)
		}
		if _, err := path.Match(r.Match, ""); err != nil {
			return fmt.Errorf("%s.match: %q: %w", where, r.Match, err)
		}
		if err := checkGo(where, r.Go, r.Import, false); err != nil {
			return err
		}
		if r.Name != "" && !token.IsExported(r.Name) {
			return fmt.Errorf("%s.name: %q must be an exported Go identifier", where, r.Name)
		}
		if err := checkTags(where+".tags", r.Tags); err != nil {
			return err
		}
	}
	for k, r := range c.Tables {
		where := "introspect.tables." + k
		if r.Name != "" && !token.IsExported(r.Name) {
			return fmt.Errorf("%s.name: %q must be an exported Go identifier", where, r.Name)
		}
		if err := checkTags(where+".tags", r.Tags); err != nil {
			return err
		}
	}
	return nil
}

func checkGo(where, goType, imp string, required bool) error {
	if strings.TrimSpace(goType) == "" {
		if required {
			return fmt.Errorf("%s.go: a Go type is required", where)
		}
		if imp != "" {
			return fmt.Errorf("%s.import: set without go", where)
		}
		return nil
	}
	if strings.HasPrefix(goType, "*") {
		return fmt.Errorf("%s.go: %q must not start with *: nullable columns get the pointer automatically", where, goType)
	}
	return nil
}

// checkTags parses the options through the real tag grammar, so a typo is
// reported when the config is loaded and not when the models are compiled.
func checkTags(where string, opts []string) error {
	for _, o := range opts {
		if _, probs := tags.Parse(o); len(probs) > 0 {
			return fmt.Errorf("%s: %q: %s", where, o, probs[0].Msg)
		}
	}
	return nil
}

// skipped reports whether the table name matches a skip glob.
func (c *Config) skipped(table string) bool {
	for _, g := range c.Skip {
		if ok, _ := path.Match(g, table); ok {
			return true
		}
	}
	return false
}

// matchGlob reports whether name matches the glob. A malformed glob never
// matches; Validate has already rejected it.
func matchGlob(glob, name string) bool {
	ok, err := path.Match(glob, name)
	return err == nil && ok
}
