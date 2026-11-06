// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

// recConn is a PinnedConn that records the statements it is asked to run.
type recConn struct{ execs []string }

func (c *recConn) Query(context.Context, string, []any) (momo.Rows, error) { return nil, nil }
func (c *recConn) Exec(_ context.Context, sql string, _ []any) (int64, error) {
	c.execs = append(c.execs, sql)
	return 0, nil
}
func (c *recConn) Begin(context.Context, momo.TxOptions) (momo.DriverTx, error) { return nil, nil }
func (c *recConn) Release() error                                               { return nil }

func TestSetupTimeouts(t *testing.T) {
	for name, tc := range map[string]struct {
		opts migrate.SessionOptions
		want []string
	}{
		"unset leaves both alone": {migrate.SessionOptions{}, nil},
		"positive values are applied": {
			migrate.SessionOptions{LockTimeout: 5 * time.Second, StatementTimeout: 30 * time.Second},
			[]string{"SET lock_timeout = 5000", "SET statement_timeout = 30000"},
		},
		"explicit zero statement_timeout is sent": {
			migrate.SessionOptions{LockTimeout: 5 * time.Second, StatementTimeoutSet: true},
			[]string{"SET lock_timeout = 5000", "SET statement_timeout = 0"},
		},
		"explicit zero lock_timeout is sent": {
			migrate.SessionOptions{LockTimeoutSet: true, StatementTimeout: time.Second},
			[]string{"SET lock_timeout = 0", "SET statement_timeout = 1000"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := &recConn{}
			if _, err := mpg.New().Setup(context.Background(), c, tc.opts); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(c.execs, tc.want) {
				t.Fatalf("statements = %q, want %q", c.execs, tc.want)
			}
		})
	}
}
