// SPDX-License-Identifier: Apache-2.0

package timeop_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/timeop"
)

// Catches: ClockTimestamp or StatementTimestamp rendering now(), which is the
// transaction start and defeats their purpose.
func TestClockTimestampGolden(t *testing.T) {
	const sqliteNow = `strftime('%Y-%m-%d %H:%M:%f', 'now')`
	runTimeGoldens(t, []timeGolden{
		{dialect.Postgres, timeop.ClockTimestamp(), `clock_timestamp()`, nil},
		{dialect.Postgres, timeop.StatementTimestamp(), `statement_timestamp()`, nil},
		{dialect.MySQL, timeop.ClockTimestamp(), `SYSDATE(6)`, nil},
		{dialect.SQLite, timeop.ClockTimestamp(), sqliteNow, nil},
		{dialect.SQLite, timeop.StatementTimestamp(), sqliteNow, nil},
	})
}

// Catches: StatementTimestamp silently rendering something wrong on MySQL.
func TestStatementTimestampMySQLUnsupported(t *testing.T) {
	if _, err := selectOne(dialect.MySQL, timeop.StatementTimestamp()); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("MySQL StatementTimestamp err = %v, want ErrUnsupported", err)
	}
}

// Catches: the clock expressions failing to compare like a timestamp column.
func TestClockTimestampComposes(t *testing.T) {
	got, err := selectOne(dialect.Postgres, timeop.AddInterval(timeop.ClockTimestamp(), timeop.IntervalOf(30, timeop.IntervalSecond)))
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT (clock_timestamp() + INTERVAL '30 second') FROM "documents"`
	if got.SQL != want {
		t.Errorf("got %s\nwant %s", got.SQL, want)
	}
}
