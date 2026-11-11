// SPDX-License-Identifier: Apache-2.0

//go:build integration

package types_test

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/types"
)

type doc struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

func sp(s string) *string { return &s }

// roundTrip stores in through a column whose type is in.SQLType(d), reads it
// back as a fresh C and returns the value and the column's text rendering.
func roundTrip[C types.Codec](t *testing.T, db *momo.DB, d dialect.Name, name string, in C) (C, string) {
	t.Helper()
	ctx := context.Background()
	tbl := "codec_" + name
	if _, err := momo.Exec(ctx, db, "CREATE TABLE "+tbl+" (id int PRIMARY KEY, v "+in.SQLType(d)+" NOT NULL)"); err != nil {
		t.Fatalf("%s: create table: %v", name, err)
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO "+tbl+" (id, v) VALUES (1, ?)", in); err != nil {
		t.Fatalf("%s: insert: %v", name, err)
	}
	got, err := momo.Scalars[C](ctx, db, "SELECT v FROM "+tbl+" WHERE id = 1")
	if err != nil || len(got) != 1 {
		t.Fatalf("%s: select: %v (%d rows)", name, err, len(got))
	}
	castText := "text"
	if d == dialect.MySQL {
		castText = "CHAR"
	}
	txt, err := momo.Scalars[string](ctx, db, "SELECT CAST(v AS "+castText+") FROM "+tbl+" WHERE id = 1")
	if err != nil || len(txt) != 1 {
		t.Fatalf("%s: select text: %v", name, err)
	}
	return got[0], txt[0]
}

// rejectSpecial binds in (a special Date or Numeric) on a dialect that has no
// such value, on the pool and inside a transaction, and requires ErrValidation
// with nothing stored.
func rejectSpecial(t *testing.T, db *momo.DB, d dialect.Name, name, sqlType string, in any) {
	t.Helper()
	ctx := context.Background()
	tbl := "codec_" + name
	if _, err := momo.Exec(ctx, db, "CREATE TABLE "+tbl+" (id int PRIMARY KEY, v "+sqlType+")"); err != nil {
		t.Fatalf("%s: create table: %v", name, err)
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO "+tbl+" (id, v) VALUES (1, ?)", in); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("%s on %s: insert %+v: err = %v, want ErrValidation", name, d, in, err)
	}
	if err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		_, err := momo.Exec(ctx, tx, "INSERT INTO "+tbl+" (id, v) VALUES (2, ?)", in)
		return err
	}); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("%s on %s: insert %+v in a transaction: err = %v, want ErrValidation", name, d, in, err)
	}
	n, err := momo.Scalars[int64](ctx, db, "SELECT count(*) FROM "+tbl)
	if err != nil || len(n) != 1 || n[0] != 0 {
		t.Fatalf("%s on %s: rows after the rejected inserts = %v, %v; want 0", name, d, n, err)
	}
}

// TestCodecsRoundTripLive stores and reads every codec through a column
// declared with its own SQLType on each registered dialect, so a codec whose
// SQL type and encoding disagree fails here (S-73).
func TestCodecsRoundTripLive(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		if d == dialect.Postgres {
			// hstore is a database-wide extension and the codec tables are
			// not schema-qualified: a scratch database keeps both out of the
			// shared test database, which migrate's drift check inspects.
			db = testopen.PostgresIn(t, testdb.ScratchDatabase(t, "momo_codec_"))
			if _, err := momo.Exec(ctx, db, "CREATE EXTENSION IF NOT EXISTS hstore SCHEMA public"); err != nil {
				t.Fatalf("create hstore extension: %v", err)
			}
		}

		codecRoundTrips(t, db, d)
	})
}

// mysqlServers lists the opener of every supported MySQL-family server.
var mysqlServers = []struct {
	name string
	open func(*testing.T, ...momo.Option) *momo.DB
}{
	{"mysql84", testopen.MySQL84},
	{"mysql97", testopen.MySQL97},
	{"mariadb118", testopen.MariaDB},
}

// TestCodecsRoundTripLiveMySQL runs the same round trip on MySQL 8.4, 9.7 and
// MariaDB: every codec's SQLType there is TEXT, and the text must survive.
func TestCodecsRoundTripLiveMySQL(t *testing.T) {
	for _, s := range mysqlServers {
		t.Run(s.name, func(t *testing.T) {
			db := s.open(t)
			codecRoundTrips(t, db, db.Dialect().Name)
		})
	}
}

// codecRoundTrips stores and reads every codec on db through a column of its
// own SQLType for d.
func codecRoundTrips(t *testing.T, db *momo.DB, d dialect.Name) {
	t.Run("JSON", func(t *testing.T) {
		in := &types.JSON[doc]{V: doc{Name: `q"uote\`, Tags: []string{"a", "b"}}}
		got, _ := roundTrip(t, db, d, "json", in)
		if !reflect.DeepEqual(got, in) {
			t.Fatalf("got %+v, want %+v", got, in)
		}
	})
	t.Run("Date", func(t *testing.T) {
		in := &types.Date{Time: time.Date(1969, 12, 31, 0, 0, 0, 0, time.UTC)}
		got, txt := roundTrip(t, db, d, "date", in)
		if txt != "1969-12-31" || !got.Time.Equal(in.Time) {
			t.Fatalf("got %v stored as %q", got.Time, txt)
		}
	})
	t.Run("DateInfinity", func(t *testing.T) {
		for i, c := range []struct {
			in   types.Date
			text string
		}{
			{types.Date{Infinity: true}, "infinity"},
			{types.Date{NegInfinity: true}, "-infinity"},
		} {
			if d != dialect.Postgres {
				rejectSpecial(t, db, d, "dateinf"+strconv.Itoa(i), c.in.SQLType(d), c.in)
				continue
			}
			in := c.in
			got, txt := roundTrip(t, db, d, "dateinf"+strconv.Itoa(i), &in)
			if txt != c.text || got.Infinity != in.Infinity || got.NegInfinity != in.NegInfinity || got.IsFinite() {
				t.Fatalf("got %+v stored as %q, want %+v as %q", got, txt, in, c.text)
			}
		}
	})
	t.Run("Numeric", func(t *testing.T) {
		for i, dec := range []string{"12345678901234567890.123456789", "-0.000000000000000001", "0", "-12.50"} {
			in := &types.Numeric{Decimal: dec}
			got, txt := roundTrip(t, db, d, "numeric"+strconv.Itoa(i), in)
			if got.Decimal != dec || !got.IsFinite() || txt != dec {
				t.Fatalf("%s: got %+v stored as %q", dec, got, txt)
			}
		}
	})
	t.Run("NumericSpecial", func(t *testing.T) {
		for i, c := range []struct {
			in   types.Numeric
			text string
		}{
			{types.Numeric{NaN: true}, "NaN"},
			{types.Numeric{Infinity: true}, "Infinity"},
			{types.Numeric{NegInfinity: true}, "-Infinity"},
		} {
			if d != dialect.Postgres {
				rejectSpecial(t, db, d, "numspecial"+strconv.Itoa(i), c.in.SQLType(d), c.in)
				continue
			}
			in := c.in
			got, txt := roundTrip(t, db, d, "numspecial"+strconv.Itoa(i), &in)
			if txt != c.text || *got != in || got.IsFinite() {
				t.Fatalf("got %+v stored as %q, want %+v as %q", got, txt, in, c.text)
			}
		}
	})
	t.Run("TimeOfDay", func(t *testing.T) {
		// Last microsecond of the day: rounding would turn it into 24:00:00.
		in := &types.TimeOfDay{Nanos: 86399999999000}
		got, txt := roundTrip(t, db, d, "tod", in)
		if txt != "23:59:59.999999" || got.Nanos != in.Nanos {
			t.Fatalf("got %d stored as %q", got.Nanos, txt)
		}
	})
	t.Run("UUID", func(t *testing.T) {
		in := &types.UUID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 0xff}
		got, txt := roundTrip(t, db, d, "uuid", in)
		if txt != "01020304-0506-0708-090a-0b0c0d0e0fff" || *got != *in {
			t.Fatalf("got %v stored as %q", got, txt)
		}
	})
	t.Run("Hstore", func(t *testing.T) {
		in := &types.Hstore{"k": sp("v"), "n": nil, "e": sp(""), `q"\`: sp(`a"b\c => d, `)}
		got, _ := roundTrip(t, db, d, "hstore", in)
		if !reflect.DeepEqual(got, in) {
			t.Fatalf("got %v, want %v", got, in)
		}
	})
}
