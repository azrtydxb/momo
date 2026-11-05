// SPDX-License-Identifier: Apache-2.0

//go:build integration

package types_test

import (
	"context"
	"reflect"
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
	txt, err := momo.Scalars[string](ctx, db, "SELECT CAST(v AS text) FROM "+tbl+" WHERE id = 1")
	if err != nil || len(txt) != 1 {
		t.Fatalf("%s: select text: %v", name, err)
	}
	return got[0], txt[0]
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
	})
}
