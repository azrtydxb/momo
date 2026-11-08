// SPDX-License-Identifier: Apache-2.0

// Package config reads momo.yaml and converts it into a gen.Config.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/naming"
)

// File is the decoded momo.yaml.
type File struct {
	Version  int                       `yaml:"version"`
	Packages []string                  `yaml:"packages"`
	Naming   string                    `yaml:"naming"`
	Dialects []string                  `yaml:"dialects"`
	Tags     []string                  `yaml:"tags"`
	Types    map[string]string         `yaml:"types"`
	Migrate  MigrateConfig             `yaml:"migrate"`
	Plugins  map[string]map[string]any `yaml:"plugins"`
}

// MigrateConfig is the single schema of the migrate: section. Its semantics
// are owned by the migrations plan; this package validates the shape only.
type MigrateConfig struct {
	Dir                map[string]string `yaml:"dir"`                 // dialect -> directory
	URLEnv             map[string]string `yaml:"url_env"`             // dialect -> environment variable name
	Schemas            []string          `yaml:"schemas"`             // default: the connection's current_schema()
	LockTimeout        string            `yaml:"lock_timeout"`        // Go duration string, default "5s"
	StatementTimeout   string            `yaml:"statement_timeout"`   // Go duration string, default unset
	Exclude            []string          `yaml:"exclude"`             // glob patterns for unmanaged objects
	ExternalExtensions []string          `yaml:"external_extensions"` // extensions created outside momo: never emitted, never reported
}

// Load reads and validates the momo.yaml at path.
func Load(path string) (*File, error) {
	fh, err := os.Open(path) // #nosec G304 -- the momo.yaml path is the user's own CLI argument or default: trusted local input
	if err != nil {
		return nil, fmt.Errorf("momo.yaml: %w", err)
	}
	defer func() { _ = fh.Close() }()

	dec := yaml.NewDecoder(fh)
	dec.KnownFields(true)
	var f File
	if err := dec.Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s: momo.yaml is empty", path)
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: multiple YAML documents are not supported", path)
	}
	if f.Version == 0 {
		return nil, fmt.Errorf("%s: version is required (want 1)", path)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("%s: unsupported momo.yaml version %d", path, f.Version)
	}
	if f.Naming != "" {
		if _, err := naming.ByName(f.Naming); err != nil {
			return nil, fmt.Errorf("%s: naming: %w", path, err)
		}
	}
	if err := validateTypes(f.Types); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, d := range f.Dialects {
		if dialect.For(dialect.Name(d)) == nil {
			return nil, fmt.Errorf("%s: dialects: unknown dialect %q", path, d)
		}
	}
	if err := f.Migrate.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &f, nil
}

// validateTypes checks the types: section. Only the default of time.Time can
// be overridden, and the replacement must be a date/time SQL type that
// gen.ValidTimeType accepts, because it is written into DDL.
func validateTypes(types map[string]string) error {
	for k, v := range types {
		if k != "time.Time" {
			return fmt.Errorf("types: %q cannot be overridden; only time.Time is supported", k)
		}
		if !gen.ValidTimeType(v) {
			return fmt.Errorf("types: %s: %q is not a timestamp type; use timestamp, timestamptz, time or timetz with an optional precision 0-6 and an optional with/without time zone", k, v)
		}
	}
	return nil
}

// extensionName is the form of an entry in migrate.external_extensions: the
// name as pg_extension spells it, lower case, with no schema qualifier.
var extensionName = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)

func (m *MigrateConfig) validate() error {
	seen := map[string]bool{}
	for _, x := range m.ExternalExtensions {
		if !extensionName.MatchString(x) {
			return fmt.Errorf("migrate.external_extensions: %q is not an extension name: use the lower-case name without spaces, quotes or a schema qualifier", x)
		}
		if seen[x] {
			return fmt.Errorf("migrate.external_extensions: %q is listed twice", x)
		}
		seen[x] = true
	}
	for _, k := range []struct{ key, val string }{
		{"migrate.lock_timeout", m.LockTimeout},
		{"migrate.statement_timeout", m.StatementTimeout},
	} {
		if k.val == "" {
			continue
		}
		if _, err := time.ParseDuration(k.val); err != nil {
			return fmt.Errorf("%s: %w", k.key, err)
		}
	}
	for _, s := range []struct {
		key string
		m   map[string]string
	}{{"migrate.dir", m.Dir}, {"migrate.url_env", m.URLEnv}} {
		keys := make([]string, 0, len(s.m))
		for k := range s.m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if dialect.For(dialect.Name(k)) == nil {
				return fmt.Errorf("%s: unknown dialect %q", s.key, k)
			}
		}
	}
	return nil
}

// ToConfig converts f into a gen.Config rooted at dir. Plugins are left to
// the caller.
func ToConfig(f *File, dir string) (gen.Config, error) {
	name := f.Naming
	if name == "" {
		name = "momo"
	}
	namer, err := naming.ByName(name)
	if err != nil {
		return gen.Config{}, fmt.Errorf("naming: %w", err)
	}
	var dialects []dialect.Name
	for _, d := range f.Dialects {
		if dialect.For(dialect.Name(d)) == nil {
			return gen.Config{}, fmt.Errorf("dialects: unknown dialect %q", d)
		}
		dialects = append(dialects, dialect.Name(d))
	}
	return gen.Config{Dir: dir, Naming: namer, Dialects: dialects, Tags: f.Tags, TypeDefaults: f.Types}, nil
}
