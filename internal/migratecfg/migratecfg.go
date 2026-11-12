// SPDX-License-Identifier: Apache-2.0

// Package migratecfg is the one reader and validator of the part of momo.yaml
// that configures the migration engine: the migrate: section and
// introspect.only. The momo command (through gen/config) and
// migrate.ConfigFromFile both use it, so a typo or a bad value is rejected the
// same way on both paths. It does not import migrate; Build turns a Resolved
// into engine options through constructors the caller passes in.
package migratecfg

import (
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/azrtydxb/momo/dialect"
)

// Migrate is the migrate: section of momo.yaml.
type Migrate struct {
	Dir                map[string]string `yaml:"dir"`                 // dialect -> directory
	URLEnv             map[string]string `yaml:"url_env"`             // dialect -> environment variable name
	Schemas            []string          `yaml:"schemas"`             // default: the connection's current_schema()
	LockTimeout        string            `yaml:"lock_timeout"`        // Go duration string, default "5s"
	StatementTimeout   string            `yaml:"statement_timeout"`   // Go duration string, default unset
	Exclude            []string          `yaml:"exclude"`             // glob patterns for unmanaged objects
	ExternalExtensions []string          `yaml:"external_extensions"` // extensions created outside momo: never emitted, never reported
}

// GenKeys are the top-level momo.yaml keys that only code generation reads. A
// reader of the migrate part ignores them; gen/config checks the list against
// its File type.
var GenKeys = []string{"dialects", "naming", "packages", "plugins", "tags", "types"}

// IntrospectKeys are the keys of the introspect: section. Only `only` is read
// here; the others belong to `momo generate --from-db`. gen/config checks the
// list against fromdb.Config.
var IntrospectKeys = []string{"columns", "dbdefault", "functions", "inverse", "only", "out", "package", "relations", "skip", "tables", "types", "uuid"}

// extensionName is the form of an entry in migrate.external_extensions: the
// name as pg_extension spells it, lower case, with no schema qualifier.
var extensionName = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)

// Validate checks the migrate: section: the external extension names (shape
// and duplicates), the durations, and the dialect names of dir and url_env.
func (m *Migrate) Validate() error {
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

// ValidateOnly checks the globs of introspect.only: none may be empty (it
// would match no table, so no model would be written) or malformed.
func ValidateOnly(only []string) error {
	for i, g := range only {
		if strings.TrimSpace(g) == "" {
			return fmt.Errorf("introspect.only[%d]: empty glob: it would match no table, so no model would be written", i)
		}
		if _, err := path.Match(g, ""); err != nil {
			return fmt.Errorf("introspect.only[%d]: %q: %w", i, g, err)
		}
	}
	return nil
}

// EngineConfig has exactly the fields of migrate.Config, in the same order, so
// the caller converts it with migrate.Config(c); the compiler rejects the
// conversion if the two ever drift apart.
type EngineConfig struct {
	Schemas            []string
	LockTimeout        time.Duration
	StatementTimeout   time.Duration
	Exclude            []string
	OnlyTables         []string
	ExternalExtensions []string
}

// Resolved is the validated engine configuration of a momo.yaml.
type Resolved struct {
	Config EngineConfig
	// LockTimeout and StatementTimeout are non-nil when the key is present,
	// including 0s, which turns the timeout off.
	LockTimeout, StatementTimeout *time.Duration
}

// Resolve validates the migrate: section and introspect.only and maps them to
// the engine configuration.
func Resolve(m Migrate, only []string) (Resolved, error) {
	if err := m.Validate(); err != nil {
		return Resolved{}, err
	}
	if err := ValidateOnly(only); err != nil {
		return Resolved{}, err
	}
	r := Resolved{Config: EngineConfig{Schemas: m.Schemas, Exclude: m.Exclude, OnlyTables: only, ExternalExtensions: m.ExternalExtensions}}
	if m.LockTimeout != "" {
		d, err := time.ParseDuration(m.LockTimeout)
		if err != nil {
			return Resolved{}, fmt.Errorf("migrate.lock_timeout: %w", err)
		}
		r.Config.LockTimeout, r.LockTimeout = d, &d
	}
	if m.StatementTimeout != "" {
		d, err := time.ParseDuration(m.StatementTimeout)
		if err != nil {
			return Resolved{}, fmt.Errorf("migrate.statement_timeout: %w", err)
		}
		r.Config.StatementTimeout, r.StatementTimeout = d, &d
	}
	return r, nil
}

// Builders are the constructors Build calls; the caller passes the engine's
// WithConfig, WithLockTimeout and WithStatementTimeout.
type Builders[O any] struct {
	Config           func(EngineConfig) O
	LockTimeout      func(time.Duration) O
	StatementTimeout func(time.Duration) O
}

// Build maps r to engine options: the config, then one option for each
// timeout that is present, so an explicit 0s wins over the default.
func Build[O any](r Resolved, b Builders[O]) []O {
	opts := []O{b.Config(r.Config)}
	if r.LockTimeout != nil {
		opts = append(opts, b.LockTimeout(*r.LockTimeout))
	}
	if r.StatementTimeout != nil {
		opts = append(opts, b.StatementTimeout(*r.StatementTimeout))
	}
	return opts
}

// Read decodes and resolves a momo.yaml. It rejects an empty file, several
// documents, a version other than 1, and any key it does not know: an unknown
// key at the top level (the GenKeys are ignored), in migrate or in
// introspect. name prefixes the errors.
func Read(r io.Reader, name string) (Resolved, error) {
	var f struct {
		Version int     `yaml:"version"`
		Migrate Migrate `yaml:"migrate"`
		// Introspect keeps its keys other than only unread but checked.
		Introspect struct {
			Only []string             `yaml:"only"`
			Rest map[string]yaml.Node `yaml:",inline"`
		} `yaml:"introspect"`
		Rest map[string]yaml.Node `yaml:",inline"`
	}
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return Resolved{}, fmt.Errorf("%s: momo.yaml is empty", name)
		}
		return Resolved{}, fmt.Errorf("%s: %w", name, err)
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return Resolved{}, fmt.Errorf("%s: multiple YAML documents are not supported", name)
	}
	if f.Version == 0 {
		return Resolved{}, fmt.Errorf("%s: version is required (want 1)", name)
	}
	if f.Version != 1 {
		return Resolved{}, fmt.Errorf("%s: unsupported momo.yaml version %d", name, f.Version)
	}
	if k := unknown(f.Rest, GenKeys); k != "" {
		return Resolved{}, fmt.Errorf("%s: unknown key %q at the top level", name, k)
	}
	if k := unknown(f.Introspect.Rest, IntrospectKeys); k != "" {
		return Resolved{}, fmt.Errorf("%s: unknown key %q in introspect", name, k)
	}
	res, err := Resolve(f.Migrate, f.Introspect.Only)
	if err != nil {
		return Resolved{}, fmt.Errorf("%s: %w", name, err)
	}
	return res, nil
}

// unknown returns the first key of m (in sorted order) that is not in known.
func unknown(m map[string]yaml.Node, known []string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !slices.Contains(known, k) {
			return k
		}
	}
	return ""
}
