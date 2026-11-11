// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Settings is the part of momo.yaml that configures the migration engine: the
// migrate: section's schemas, exclude, external_extensions, lock_timeout and
// statement_timeout, and introspect.only. The `momo` command and
// ConfigFromFile both map it to engine options through Settings.Options, so a
// program that migrates in-process and the CLI cannot read momo.yaml
// differently.
type Settings struct {
	Schemas            []string // migrate.schemas
	Exclude            []string // migrate.exclude
	ExternalExtensions []string // migrate.external_extensions
	Only               []string // introspect.only
	LockTimeout        string   // migrate.lock_timeout, a Go duration; empty: unset
	StatementTimeout   string   // migrate.statement_timeout, a Go duration; empty: unset
}

// Options maps the settings to engine options: WithConfig, plus
// WithLockTimeout and WithStatementTimeout for a timeout that is present, so
// an explicit 0s turns the timeout off. It does not set the dialect or the
// clock.
func (s Settings) Options() ([]Option, error) {
	for i, g := range s.Only {
		if strings.TrimSpace(g) == "" {
			return nil, fmt.Errorf("introspect.only[%d]: empty glob: it would match no table", i)
		}
		if _, err := path.Match(g, ""); err != nil {
			return nil, fmt.Errorf("introspect.only[%d]: %q: %w", i, g, err)
		}
	}
	cfg := Config{Schemas: s.Schemas, Exclude: s.Exclude, OnlyTables: s.Only, ExternalExtensions: s.ExternalExtensions}
	var timeouts []Option
	var err error
	if s.LockTimeout != "" {
		if cfg.LockTimeout, err = time.ParseDuration(s.LockTimeout); err != nil {
			return nil, fmt.Errorf("migrate.lock_timeout: %w", err)
		}
		// A key that is present wins, including 0s, which turns the timeout off.
		timeouts = append(timeouts, WithLockTimeout(cfg.LockTimeout))
	}
	if s.StatementTimeout != "" {
		if cfg.StatementTimeout, err = time.ParseDuration(s.StatementTimeout); err != nil {
			return nil, fmt.Errorf("migrate.statement_timeout: %w", err)
		}
		timeouts = append(timeouts, WithStatementTimeout(cfg.StatementTimeout))
	}
	return append([]Option{WithConfig(cfg)}, timeouts...), nil
}

// yamlFile is the slice of momo.yaml that ConfigFromFile reads. Other keys
// belong to code generation and are ignored here.
type yamlFile struct {
	Version int `yaml:"version"`
	Migrate struct {
		Schemas            []string `yaml:"schemas"`
		LockTimeout        string   `yaml:"lock_timeout"`
		StatementTimeout   string   `yaml:"statement_timeout"`
		Exclude            []string `yaml:"exclude"`
		ExternalExtensions []string `yaml:"external_extensions"`
	} `yaml:"migrate"`
	Introspect struct {
		Only []string `yaml:"only"`
	} `yaml:"introspect"`
}

// ConfigFromFile reads the momo.yaml at path in fsys, for example an embedded
// one, and returns the engine options its migrate: section and
// introspect.only describe: schemas, exclude, external_extensions,
// lock_timeout, statement_timeout and only. Pass them to NewEngine together
// with the dialect-bearing driver and the migration files. It is the same
// mapping `momo migrate` uses. Keys that only code generation reads are
// ignored.
func ConfigFromFile(fsys fs.FS, path string) ([]Option, error) {
	fh, err := fsys.Open(path)
	if err != nil {
		return nil, fmt.Errorf("momo.yaml: %w", err)
	}
	defer func() { _ = fh.Close() }()
	var f yamlFile
	if err := yaml.NewDecoder(fh).Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s: momo.yaml is empty", path)
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("%s: unsupported or missing momo.yaml version %d (want 1)", path, f.Version)
	}
	opts, err := Settings{
		Schemas: f.Migrate.Schemas, Exclude: f.Migrate.Exclude, ExternalExtensions: f.Migrate.ExternalExtensions,
		Only: f.Introspect.Only, LockTimeout: f.Migrate.LockTimeout, StatementTimeout: f.Migrate.StatementTimeout,
	}.Options()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return opts, nil
}
