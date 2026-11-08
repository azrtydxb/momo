// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// idSecret has a sensitive id column: sensitivity hides it from the generated
// allowlist, not from inserts.
type idSecret struct {
	ID   string
	Name string
}

var idSecretMeta = momo.Meta[idSecret]{
	Table:    "id_secrets",
	Columns:  []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagSensitive}, {Name: "name"}},
	Ptr:      func(m *idSecret, i int) any { return [...]any{&m.ID, &m.Name}[i] },
	Value:    func(m *idSecret, i int) any { return [...]any{m.ID, m.Name}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK:     []int{0},
	IDGens: []momo.IDGen{{Column: 0, Name: "uuid"}},
}

func (*idSecret) MomoMeta() *momo.Meta[idSecret] { return &idSecretMeta }

func TestIDGenSensitiveIDColumnIsGenerated(t *testing.T) {
	m := idSecret{Name: "a"}
	if _, err := momo.Insert[idSecret](idDB()).Model(&m).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reUUID.MatchString(m.ID) {
		t.Fatalf("sensitive id = %q", m.ID)
	}
}

// TestIDGenConflictWithoutReturningLeavesModel covers the dialect that has no
// RETURNING and no ON CONFLICT: the statement fails and nothing is written back.
func TestIDGenConflictWithoutReturningLeavesModel(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.MySQL))
	name := momo.NewColumn[string]("id_rows", "name", 0)
	m := idRow{Name: "a"}
	_, err := momo.Insert[idRow](db).Model(&m).OnConflict(name).DoNothing().Exec(context.Background())
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	if m.ID != "" {
		t.Fatalf("a failed conflict statement must not write the id back: %q", m.ID)
	}
	// Without a conflict clause MySQL writes the id back up front.
	if _, err := momo.Insert[idRow](db).Model(&m).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reCUID.MatchString(m.ID) {
		t.Fatalf("plain insert on MySQL: id = %q", m.ID)
	}
}
