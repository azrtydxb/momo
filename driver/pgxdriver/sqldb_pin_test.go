// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

func TestFromSQLDBRejectsNilAndForeignDB(t *testing.T) {
	ctx := context.Background()
	for name, db := range map[string]*sql.DB{"nil": nil, "foreign": sql.OpenDB(fakeConnector{})} {
		d := FromSQLDB(db)
		if _, err := d.Query(ctx, "SELECT 1", nil); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: Query = %v, want ErrUnsupported", name, err)
		}
		if _, err := d.Exec(ctx, "SELECT 1", nil); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: Exec = %v, want ErrUnsupported", name, err)
		}
		if _, err := d.Begin(ctx, momo.TxOptions{}); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: Begin = %v, want ErrUnsupported", name, err)
		}
		if _, err := d.(momo.Pinner).Pin(ctx); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: Pin = %v, want ErrUnsupported", name, err)
		}
		_ = d.(momo.PoolStater).PoolStats()
	}
}

func TestSQLPinnedDiscardNonPgxConnErrors(t *testing.T) {
	db := sql.OpenDB(fakeConnector{})
	t.Cleanup(func() { _ = db.Close() })
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p := &sqlPinned{conn: conn}
	if err := p.Discard(); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Discard on a foreign conn = %v, want ErrUnsupported", err)
	}
}

// fakeConnector is a database/sql driver that is not pgx.
type fakeConnector struct{}

func (fakeConnector) Connect(context.Context) (sqldriver.Conn, error) { return fakeConn{}, nil }
func (fakeConnector) Driver() sqldriver.Driver                        { return fakeDriver{} }

type fakeDriver struct{}

func (fakeDriver) Open(string) (sqldriver.Conn, error) { return fakeConn{}, nil }

type fakeConn struct{}

func (fakeConn) Prepare(string) (sqldriver.Stmt, error) { return nil, errors.New("fake") }
func (fakeConn) Close() error                           { return nil }
func (fakeConn) Begin() (sqldriver.Tx, error)           { return nil, errors.New("fake") }
