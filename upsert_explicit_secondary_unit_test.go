// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// Catches: an upsert that targets the secondary unique key UNIQUE(email) while
// Explicit writes the auto-increment primary key being sent to MySQL or
// MariaDB. ON DUPLICATE KEY UPDATE cannot name a key, so a duplicate on the
// primary key would update the row too. The relaxation of the secondary-target
// rule holds only while the insert leaves the key to the database, and Explicit
// puts the key in the column list, so every form (DoUpdate, DoNothing, one row,
// many rows) must fail with ErrUnsupported naming the primary key.
func TestUpsertSecondaryTargetWithExplicitKeyRefusedOnMySQLFamily(t *testing.T) {
	for _, f := range []dialect.Flavor{dialect.FlavorMySQL, dialect.FlavorMariaDB} {
		db := momo.New(fakedriver.NewDialect(dialect.MySQLFlavor(f)))
		one := mrUserT{ID: 7, Email: "a@example.com", Name: "a"}
		many := []mrUserT{{ID: 8, Email: "b@example.com", Name: "b"}, {ID: 9, Email: "c@example.com", Name: "c"}}
		builds := map[string]func() (momo.Compiled, error){
			"DoUpdate": func() (momo.Compiled, error) {
				return momo.Upsert[mrUserT](db).Model(&one).Explicit(mrUserID).
					OnConflict(mrUserEmail).DoUpdate(mrUserName.SetExcluded()).SQL()
			},
			"DoNothing": func() (momo.Compiled, error) {
				return momo.Upsert[mrUserT](db).Model(&one).Explicit(mrUserID).
					OnConflict(mrUserEmail).DoNothing().SQL()
			},
			"Models": func() (momo.Compiled, error) {
				return momo.Upsert[mrUserT](db).Models(many).Explicit(mrUserID).
					OnConflict(mrUserEmail).DoUpdate(mrUserName.SetExcluded()).SQL()
			},
			"Insert OnConflict": func() (momo.Compiled, error) {
				return momo.Insert[mrUserT](db).Model(&one).Explicit(mrUserID).
					OnConflict(mrUserEmail).DoUpdate(mrUserName.SetExcluded()).SQL()
			},
		}
		for name, build := range builds {
			c, err := build()
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s %s: err = %v, sql = %q, want ErrUnsupported", f, name, err, c.SQL)
				continue
			}
			if !strings.Contains(err.Error(), "[id]") || !strings.Contains(err.Error(), "ON DUPLICATE KEY UPDATE") {
				t.Errorf("%s %s: err = %v, want a message naming the primary key [id] and ON DUPLICATE KEY UPDATE", f, name, err)
			}
		}
	}
}

// Catches: the refusal being raised only when the SQL is rendered for
// inspection, while Exec still sends the INSERT. No statement of any kind may
// reach the server, so no row is written.
func TestUpsertSecondaryTargetWithExplicitKeySendsNothing(t *testing.T) {
	for _, f := range []dialect.Flavor{dialect.FlavorMySQL, dialect.FlavorMariaDB} {
		d := newLastIDDriver(f, 2, 1)
		db := momo.New(d)
		u := mrUserT{ID: 7, Email: "a@example.com", Name: "a"}
		_, err := momo.Upsert[mrUserT](db).Model(&u).Explicit(mrUserID).
			OnConflict(mrUserEmail).DoUpdate(mrUserName.SetExcluded()).Exec(t.Context())
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("%s: err = %v, want ErrUnsupported", f, err)
		}
		if n := countCalls(d.Driver, "INSERT"); n != 0 {
			t.Fatalf("%s: %d INSERT statements reached the driver, want 0", f, n)
		}
		if u.ID != 7 {
			t.Fatalf("%s: model id = %d after the refusal, want 7 untouched", f, u.ID)
		}
	}
}

// Catches: PostgreSQL and SQLite refusing, or mis-targeting, the same call.
// Both name the conflict target in ON CONFLICT, so a secondary target with an
// explicit primary key is unambiguous there: the key joins the column list,
// the conflict is on email, and no ON DUPLICATE KEY form is rendered.
func TestUpsertSecondaryTargetWithExplicitKeyRendersOnPostgresAndSQLite(t *testing.T) {
	for _, name := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		db := momo.New(fakedriver.New(name))
		u := mrUserT{ID: 7, Email: "a@example.com", Name: "a"}
		c, err := momo.Upsert[mrUserT](db).Model(&u).Explicit(mrUserID).
			OnConflict(mrUserEmail).DoUpdate(mrUserName.SetExcluded()).SQL()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, want := range []string{`("id", "email", "name")`, `ON CONFLICT ("email") DO UPDATE SET "name" = EXCLUDED."name"`} {
			if !strings.Contains(c.SQL, want) {
				t.Errorf("%s: SQL %s lacks %s", name, c.SQL, want)
			}
		}
		if strings.Contains(c.SQL, "ON DUPLICATE KEY") {
			t.Errorf("%s: SQL %s uses MySQL syntax", name, c.SQL)
		}
	}
}
