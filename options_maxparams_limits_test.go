// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/suite"
)

func mustPanicMaxParams(t *testing.T, n int) {
	t.Helper()
	defer func() {
		r := recover()
		msg, _ := r.(string)
		if !strings.Contains(msg, "WithMaxParams") {
			t.Fatalf("WithMaxParams(%d): recovered %v, want a panic naming WithMaxParams", n, r)
		}
	}()
	momo.New(fakedriver.New(dialect.Postgres), momo.WithMaxParams(n))
}

// A zero or negative limit would read as "no limit" in every guard, so it is a
// programmer error and panics at New.
func TestWithMaxParamsRejectsNonPositive(t *testing.T) {
	for _, n := range []int{0, -1, -65535} {
		mustPanicMaxParams(t, n)
	}
}

// A value above the dialect limit would exceed the wire limit, so it is
// clamped to the dialect value.
func TestWithMaxParamsClampsToDialectLimit(t *testing.T) {
	limit := dialect.For(dialect.Postgres).MaxParams
	for _, n := range []int{limit + 1, limit * 10} {
		db := momo.New(fakedriver.New(dialect.Postgres), momo.WithMaxParams(n))
		if got := db.Dialect().MaxParams; got != limit {
			t.Fatalf("WithMaxParams(%d): MaxParams = %d, want clamp to %d", n, got, limit)
		}
	}
	if got := momo.New(fakedriver.New(dialect.Postgres), momo.WithMaxParams(limit)).Dialect().MaxParams; got != limit {
		t.Fatalf("WithMaxParams(dialect max) = %d, want %d", got, limit)
	}
}

// The copy must not alias the shared dialect's slices.
func TestWithMaxParamsDeepCopiesVectorCaps(t *testing.T) {
	shared := dialect.For(dialect.Postgres)
	wantM := slices.Clone(shared.Vector.Metrics)
	wantI := slices.Clone(shared.Vector.Indexes)
	cp := momo.New(fakedriver.New(dialect.Postgres), momo.WithMaxParams(7)).Dialect()
	cp.Vector.Metrics[0] = "mutated"
	cp.Vector.Indexes[0] = "mutated"
	if !slices.Equal(shared.Vector.Metrics, wantM) || !slices.Equal(shared.Vector.Indexes, wantI) {
		t.Fatalf("mutating the copy changed the shared dialect: %v %v", shared.Vector.Metrics, shared.Vector.Indexes)
	}
}

// New builds the execer before the options run so WithMaxParams can adjust
// its dialect; WithFieldHooks must keep working with it in either order.
func TestWithMaxParamsComposesWithFieldHooks(t *testing.T) {
	hooks := []momo.FieldHook[suite.Document]{{Column: 4, Default: func(*suite.Document) any { return int64(7) }}}
	for name, opts := range map[string][]momo.Option{
		"limit first": {momo.WithMaxParams(50), momo.WithFieldHooks[suite.Document](hooks)},
		"hooks first": {momo.WithFieldHooks[suite.Document](hooks), momo.WithMaxParams(50)},
	} {
		t.Run(name, func(t *testing.T) {
			db := momo.New(fakedriver.New(dialect.Postgres), opts...)
			if got := db.Dialect().MaxParams; got != 50 {
				t.Fatalf("MaxParams = %d, want 50", got)
			}
			c, err := momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("a")).SQL()
			if err != nil {
				t.Fatal(err)
			}
			if len(c.Args) < 3 || c.Args[2] != int64(7) {
				t.Fatalf("field hook default was not applied: %q %v", c.SQL, c.Args)
			}
		})
	}
}

func authorsN(n int) []models.Author {
	as := make([]models.Author, n)
	for i := range as {
		as[i].ID = int64(i + 1)
	}
	return as
}

// assertChunks checks that n statements ran and each stayed within limit.
func assertChunks(t *testing.T, drv *fakedriver.Driver, limit, n int) {
	t.Helper()
	calls := drv.Calls()
	if len(calls) != n {
		t.Fatalf("statements = %d, want %d", len(calls), n)
	}
	for i, c := range calls {
		if len(c.Args) > limit {
			t.Fatalf("statement %d binds %d > limit %d", i, len(c.Args), limit)
		}
	}
}

// 60 keys at 40 - 16 reserved = 24 per statement: three statements.
func TestLoadChunksWithMaxParamsOnFakeDriver(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithMaxParams(40))
	if err := momo.Load(t.Context(), db, authorsN(60), author.FirstDocument); err != nil {
		t.Fatal(err)
	}
	assertChunks(t, drv, 40, 3)
	big := fakedriver.New(dialect.Postgres)
	if err := momo.Load(t.Context(), momo.New(big), authorsN(60), author.FirstDocument); err != nil {
		t.Fatal(err)
	}
	assertChunks(t, big, 65535, 1)
}

func TestLoadInsideTxUsesMaxParams(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithMaxParams(40))
	err := db.Tx(t.Context(), momo.TxOptions{}, func(tx *momo.Tx) error {
		return momo.Load(t.Context(), tx, authorsN(60), author.FirstDocument)
	})
	if err != nil {
		t.Fatal(err)
	}
	assertChunks(t, drv, 40, 3)
}

// pinnedFake pins statements to a fake driver.
type pinnedFake struct{ *fakedriver.Driver }

func (pinnedFake) Release() error { return nil }

func TestOnConnInheritsMaxParams(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithMaxParams(40)).OnConn(pinnedFake{drv})
	if got := db.Dialect().MaxParams; got != 40 {
		t.Fatalf("OnConn MaxParams = %d, want 40", got)
	}
	if err := momo.Load(t.Context(), db, authorsN(60), author.FirstDocument); err != nil {
		t.Fatal(err)
	}
	assertChunks(t, drv, 40, 3)
}

// A scope that leaves no room for even one key is a validation error, not an
// oversized statement.
func TestLoadScopeLeavesNoRoomForKey(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithMaxParams(3))
	// Discriminator (1) + LoadWhere (2) + one key column exceeds 3.
	imgID := momo.NewColumn[int64]("images", "id", 0)
	err := momo.Load(t.Context(), db, authorsN(3), author.Images, momo.LoadWhere[models.Image](imgID.In(1, 2)))
	var me *momo.Error
	if !errors.As(err, &me) || me.Kind != momo.ErrValidation || !strings.Contains(err.Error(), "no room for a key") {
		t.Fatalf("err = %v, want ErrValidation naming the missing key room", err)
	}
	if n := len(drv.Calls()); n != 0 {
		t.Fatalf("%d statements ran, want none", n)
	}
}

func TestRawInRejectedUnderLoweredMaxParams(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	q := func(db *momo.DB, n int) error {
		_, err := momo.Select[suite.Document](db).Where(momo.TypedSQL[bool]("tenant_id IN (?)", momo.In(make([]int, n)))).SQL()
		return err
	}
	low := momo.New(drv, momo.WithMaxParams(5))
	if err := q(low, 6); err == nil || !strings.Contains(err.Error(), "limit of 5") {
		t.Fatalf("err = %v, want a build error naming the limit of 5", err)
	}
	if err := q(low, 5); err != nil {
		t.Fatalf("exactly the limit must pass: %v", err)
	}
	if err := q(momo.New(drv), 6); err != nil {
		t.Fatalf("without the option 6 binds pass: %v", err)
	}
}

// Plain (non-In) raw binds go through checkMaxParams.
func TestRawPlainBindsRejectedUnderLoweredMaxParams(t *testing.T) {
	q := func(db *momo.DB, n int) error {
		args := make([]any, n)
		_, err := momo.Select[suite.Document](db).Where(momo.TypedSQL[bool](strings.Repeat("? = 1 AND ", n-1)+"? = 1", args...)).SQL()
		return err
	}
	low := momo.New(fakedriver.New(dialect.Postgres), momo.WithMaxParams(5))
	if err := q(low, 6); err == nil || !strings.Contains(err.Error(), "limit of 5") {
		t.Fatalf("err = %v, want a build error naming the limit of 5", err)
	}
	if err := q(low, 5); err != nil {
		t.Fatalf("exactly the limit must pass: %v", err)
	}
}
