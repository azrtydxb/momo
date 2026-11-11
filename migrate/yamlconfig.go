// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"fmt"
	"io/fs"

	"github.com/azrtydxb/momo/internal/migratecfg"
)

// ConfigFromFile reads the momo.yaml at path in fsys, for example an embedded
// one, and returns the engine options its migrate: section and
// introspect.only describe: schemas, exclude, external_extensions,
// lock_timeout, statement_timeout and only. Pass them to NewEngine with the
// driver and the migration files. It reads and checks the file exactly as
// `momo migrate` does: it needs version 1, rejects an empty file, several
// documents, an unknown key in migrate or introspect, an unknown top-level key,
// a bad duration, a bad external extension name and a bad introspect.only glob.
// The only keys it ignores are the top-level ones that code generation alone
// reads (packages, naming, dialects, tags, types and plugins) and the other
// introspect keys. It does not set the dialect: the driver supplies that, or
// pass WithDialect.
func ConfigFromFile(fsys fs.FS, path string) ([]Option, error) {
	fh, err := fsys.Open(path)
	if err != nil {
		return nil, fmt.Errorf("momo.yaml: %w", err)
	}
	defer func() { _ = fh.Close() }()
	r, err := migratecfg.Read(fh, path)
	if err != nil {
		return nil, err
	}
	return optionsFrom(r), nil
}

// optionsFrom builds the engine options of a resolved momo.yaml. The `momo`
// command calls migratecfg.Build with the same three constructors.
func optionsFrom(r migratecfg.Resolved) []Option {
	return migratecfg.Build(r, migratecfg.Builders[Option]{
		Config:           func(c migratecfg.EngineConfig) Option { return WithConfig(Config(c)) },
		LockTimeout:      WithLockTimeout,
		StatementTimeout: WithStatementTimeout,
	})
}
