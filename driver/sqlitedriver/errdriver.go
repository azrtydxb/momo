// SPDX-License-Identifier: Apache-2.0

package sqlitedriver

import (
	"context"
	"fmt"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// errDriver is a driver whose every call returns err. New returns one when it
// is given options it cannot apply, and the build without cgo returns one for
// every New.
type errDriver struct{ err error }

func (errDriver) Dialect() *dialect.Dialect { return dialect.For(dialect.SQLite) }

func (d errDriver) Query(context.Context, string, []any) (momo.Rows, error) { return nil, d.err }

func (d errDriver) Exec(context.Context, string, []any) (int64, error) { return 0, d.err }

func (d errDriver) Begin(context.Context, momo.TxOptions) (momo.DriverTx, error) {
	return nil, d.err
}

// refusedOptions returns a momo.ErrValidation naming the settings cfg changes
// from their zero value, or nil when it changes none. New applies no options:
// the *sql.DB it is given is already open, so a silent no-op would hide a busy
// timeout or a connection limit the caller thinks is set.
func refusedOptions(cfg Config) error {
	var names []string
	if cfg.BusyTimeout != 0 {
		names = append(names, "WithBusyTimeout")
	}
	if cfg.Synchronous != "" {
		names = append(names, "WithSynchronous")
	}
	if cfg.TxLock != "" {
		names = append(names, "WithTxLock")
	}
	if cfg.MaxOpenConns != 0 {
		names = append(names, "WithMaxOpenConns")
	}
	if cfg.ReadOnly {
		names = append(names, "Config.ReadOnly")
	}
	if len(names) == 0 {
		return nil
	}
	return &momo.Error{Kind: momo.ErrValidation, Op: "build", Err: fmt.Errorf(
		"sqlitedriver: New cannot apply %s: the *sql.DB is already open, so set the pragmas and the connection limit on your own *sql.DB, or use Open",
		strings.Join(names, ", "))}
}
