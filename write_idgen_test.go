// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// idRow is a model with a text primary key that the client generates.
type idRow struct {
	ID   string
	Name string
}

var idRowMeta = momo.Meta[idRow]{
	Table:    "id_rows",
	Columns:  []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "name"}},
	Ptr:      func(m *idRow, i int) any { return [...]any{&m.ID, &m.Name}[i] },
	Value:    func(m *idRow, i int) any { return [...]any{m.ID, m.Name}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK:     []int{0},
	IDGens: []momo.IDGen{{Column: 0, Name: "cuid"}},
}

func (*idRow) MomoMeta() *momo.Meta[idRow] { return &idRowMeta }

// idKind is a named string id type, to prove the generator keeps the field type.
type idKind string

type idKindRow struct {
	ID   idKind
	Name string
}

var idKindRowMeta = momo.Meta[idKindRow]{
	Table:    "id_kind_rows",
	Columns:  []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "name"}},
	Ptr:      func(m *idKindRow, i int) any { return [...]any{&m.ID, &m.Name}[i] },
	Value:    func(m *idKindRow, i int) any { return [...]any{m.ID, m.Name}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK:     []int{0},
	IDGens: []momo.IDGen{{Column: 0, Name: "uuid"}},
}

func (*idKindRow) MomoMeta() *momo.Meta[idKindRow] { return &idKindRowMeta }

// idPtrRow has a nullable id.
type idPtrRow struct {
	ID   *string
	Name string
}

var idPtrRowMeta = momo.Meta[idPtrRow]{
	Table:    "id_ptr_rows",
	Columns:  []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "name"}},
	Ptr:      func(m *idPtrRow, i int) any { return [...]any{&m.ID, &m.Name}[i] },
	Value:    func(m *idPtrRow, i int) any { return [...]any{m.ID, m.Name}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK:     []int{0},
	IDGens: []momo.IDGen{{Column: 0, Name: "ulid"}},
}

func (*idPtrRow) MomoMeta() *momo.Meta[idPtrRow] { return &idPtrRowMeta }

// idBadRow declares a generator on a non-string field.
type idBadRow struct {
	ID   int64
	Name string
}

var idBadRowMeta = momo.Meta[idBadRow]{
	Table:    "id_bad_rows",
	Columns:  []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "name"}},
	Ptr:      func(m *idBadRow, i int) any { return [...]any{&m.ID, &m.Name}[i] },
	Value:    func(m *idBadRow, i int) any { return [...]any{m.ID, m.Name}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK:     []int{0},
	IDGens: []momo.IDGen{{Column: 0, Name: "cuid"}},
}

func (*idBadRow) MomoMeta() *momo.Meta[idBadRow] { return &idBadRowMeta }

func idDB(opts ...momo.Option) *momo.DB {
	return momo.New(fakedriver.New(dialect.Postgres), opts...)
}

var (
	reCUID = regexp.MustCompile(`^c[0-9a-z]{24}$`)
	reUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	reULID = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)
)

// insertCUIDs inserts n zero-id rows and returns the ids written back into
// the models.
func insertCUIDs(t *testing.T, n int) []string {
	t.Helper()
	rows := make([]idRow, n)
	if _, err := momo.Insert[idRow](idDB()).Models(rows).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, n)
	for i := range rows {
		ids[i] = rows[i].ID
	}
	return ids
}

func TestIDGenCUIDFormatAndUniqueness(t *testing.T) {
	ids := insertCUIDs(t, 5000)
	seen := map[string]bool{}
	for _, id := range ids {
		if !reCUID.MatchString(id) {
			t.Fatalf("cuid %q does not match %s", id, reCUID)
		}
		if seen[id] {
			t.Fatalf("duplicate cuid %q", id)
		}
		seen[id] = true
	}
}

func TestIDGenUUIDFormatAndUniqueness(t *testing.T) {
	db := idDB()
	rows := make([]idKindRow, 2000)
	if _, err := momo.Insert[idKindRow](db).Models(rows).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		id := string(r.ID) // the named type survives
		if !reUUID.MatchString(id) {
			t.Fatalf("uuid %q does not match %s", id, reUUID)
		}
		if seen[id] {
			t.Fatalf("duplicate uuid %q", id)
		}
		seen[id] = true
	}
}

func TestIDGenULIDFormatUniquenessAndOrder(t *testing.T) {
	db := idDB()
	rows := make([]idPtrRow, 3000)
	if _, err := momo.Insert[idPtrRow](db).Models(rows).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	prev := ""
	for i, r := range rows {
		if r.ID == nil {
			t.Fatalf("row %d: id not written back", i)
		}
		id := *r.ID
		if !reULID.MatchString(id) {
			t.Fatalf("ulid %q does not match %s", id, reULID)
		}
		if seen[id] {
			t.Fatalf("duplicate ulid %q", id)
		}
		if id <= prev {
			t.Fatalf("ulid %q does not sort after %q", id, prev)
		}
		seen[id], prev = true, id
	}
}

func TestIDGenWritesBackAndBinds(t *testing.T) {
	db := idDB()
	m := idRow{Name: "a"}
	if _, err := momo.Insert[idRow](db).Model(&m).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reCUID.MatchString(m.ID) {
		t.Fatalf("id not written back: %q", m.ID)
	}
	calls := db.Driver().(*fakedriver.Driver).Calls()
	if len(calls) != 1 || calls[0].Args[0] != m.ID {
		t.Fatalf("bound %v, model id %q", calls, m.ID)
	}
}

func TestIDGenKeepsAssignedID(t *testing.T) {
	db := idDB()
	rows := []idRow{{ID: "mine", Name: "a"}, {Name: "b"}}
	if _, err := momo.Insert[idRow](db).Models(rows).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rows[0].ID != "mine" || !reCUID.MatchString(rows[1].ID) {
		t.Fatalf("ids = %q, %q", rows[0].ID, rows[1].ID)
	}
}

func TestIDGenCustomGeneratorOverridesBuiltin(t *testing.T) {
	var mu sync.Mutex
	n := 0
	db := idDB(momo.WithIDGenerator("cuid", func() string {
		mu.Lock()
		defer mu.Unlock()
		n++
		return "id-" + strings.Repeat("x", n)
	}))
	rows := []idRow{{Name: "a"}, {Name: "b"}}
	if _, err := momo.Insert[idRow](db).Models(rows).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rows[0].ID != "id-x" || rows[1].ID != "id-xx" {
		t.Fatalf("ids = %q, %q", rows[0].ID, rows[1].ID)
	}
}

func TestIDGenUnknownGeneratorFails(t *testing.T) {
	prev := idRowMeta.IDGens
	defer func() { idRowMeta.IDGens = prev }()
	idRowMeta.IDGens = []momo.IDGen{{Column: 0, Name: "nope"}}
	m := idRow{Name: "a"}
	_, err := momo.Insert[idRow](idDB()).Model(&m).Exec(context.Background())
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

func TestIDGenEmptyIDFails(t *testing.T) {
	db := idDB(momo.WithIDGenerator("cuid", func() string { return "" }))
	m := idRow{Name: "a"}
	_, err := momo.Insert[idRow](db).Model(&m).Exec(context.Background())
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

func TestIDGenNonStringFieldFails(t *testing.T) {
	m := idBadRow{Name: "a"}
	_, err := momo.Insert[idBadRow](idDB()).Model(&m).Exec(context.Background())
	if err == nil {
		t.Fatal("a generator on an int64 field must fail")
	}
}

func TestIDGenSQLPreviewDoesNotWriteBack(t *testing.T) {
	m := idRow{Name: "a"}
	c, err := momo.Insert[idRow](idDB()).Model(&m).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "" || !reCUID.MatchString(c.Args[0].(string)) {
		t.Fatalf("preview: model id %q, bound %v", m.ID, c.Args)
	}
}

func TestIDGenOmitAndValuesUseDefault(t *testing.T) {
	db := idDB()
	m := idRow{Name: "a"}
	c, err := momo.Insert[idRow](db).Omit(momo.NewColumn[string]("id_rows", "id", momo.FlagPK)).Model(&m).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := c.Args[0].(string); !reCUID.MatchString(id) {
		t.Fatalf("omitted id not generated: %v", c.Args)
	}
}

func TestIDGenChunkedInsertWritesEveryID(t *testing.T) {
	db := idDB(momo.WithMaxParams(10)) // 5 rows of 2 columns per statement
	rows := make([]idRow, 23)
	if _, err := momo.Insert[idRow](db).Models(rows).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	var bound []string
	for _, c := range db.Driver().(*fakedriver.Driver).Calls() {
		for i := 0; i < len(c.Args); i += 2 {
			bound = append(bound, c.Args[i].(string))
		}
	}
	if len(bound) != len(rows) {
		t.Fatalf("bound %d ids, want %d", len(bound), len(rows))
	}
	for i := range rows {
		if rows[i].ID != bound[i] {
			t.Fatalf("row %d: model id %q, bound %q", i, rows[i].ID, bound[i])
		}
	}
}

func TestIDGenUpsertGeneratesForZeroID(t *testing.T) {
	db := idDB()
	m := idRow{Name: "a"}
	if _, err := momo.Upsert[idRow](db).Model(&m).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reCUID.MatchString(m.ID) {
		t.Fatalf("upsert id = %q", m.ID)
	}
}
