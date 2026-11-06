// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// An existing table that has no rows is still refused: Diff never queries
// data, so the message must not claim the table is populated.
func TestStagingRefusalDoesNotClaimTheTableIsPopulated(t *testing.T) {
	from := base()
	to := base()
	to.Tables[0].Columns = append(to.Tables[0].Columns, schema.Column{Name: "tenant", Type: "bigint"})
	_, _, err := Diff(from, to, DiffOptions{})
	if !errors.Is(err, ErrStagedRequired) {
		t.Fatalf("want ErrStagedRequired, got %v", err)
	}
	if strings.Contains(err.Error(), "populated") {
		t.Fatalf("the message claims a populated table: %v", err)
	}
	if !strings.Contains(err.Error(), "existing table") || !strings.Contains(err.Error(), "users.tenant") {
		t.Fatalf("the message must say existing table and name the column: %v", err)
	}
}

func TestStagingRefusalOfARenameCandidate(t *testing.T) {
	from := base()
	to := base()
	to.Tables[0].Columns[1] = schema.Column{Name: "mail", Type: "text"}

	for _, opts := range []DiffOptions{{}, {AllowDestructive: true}} {
		_, _, err := Diff(from, to, opts)
		if !errors.Is(err, ErrStagedRequired) {
			t.Fatalf("a NOT NULL rename candidate must be refused (%+v), got %v", opts, err)
		}
		if !strings.Contains(err.Error(), "--rename-column") {
			t.Fatalf("the refusal must name the rename flag: %v", err)
		}
	}

	changes, _, err := Diff(from, to, DiffOptions{AllowDestructive: true, Stage: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		if c.Kind == AddColumn && !c.After.(*schema.Column).Nullable {
			t.Fatalf("a NOT NULL add on an existing table must never be emitted plain: %+v", c)
		}
	}

	// The explicit rename still works and needs no staging.
	if _, _, err := Diff(from, to, DiffOptions{RenameColumns: map[string]string{"users.email": "mail"}}); err != nil {
		t.Fatalf("explicit rename: %v", err)
	}
}

func TestVolatileDefaultOnAddColumnWarns(t *testing.T) {
	for _, def := range []string{"random()", "clock_timestamp()", "gen_random_uuid()", "GEN_RANDOM_UUID ()", "(random() * 10)::int", "nextval('s')"} {
		from := base()
		to := base()
		to.Tables[0].Columns = append(to.Tables[0].Columns, schema.Column{Name: "x", Type: "text", Default: def})
		changes, hints, err := Diff(from, to, DiffOptions{})
		if err != nil || len(changes) != 1 {
			t.Fatalf("%s: %v %v", def, changes, err)
		}
		if !hasHint(hints, "risk: users.x") || !hasHint(hints, "ACCESS EXCLUSIVE") {
			t.Fatalf("%s: want a rewrite risk hint, got %v", def, hints)
		}
	}
}

func TestConstantDefaultOnAddColumnDoesNotWarn(t *testing.T) {
	for _, def := range []string{"''", "0", "now()", "current_timestamp", "'random()'", "coalesce(1, 2)", "false"} {
		from := base()
		to := base()
		to.Tables[0].Columns = append(to.Tables[0].Columns, schema.Column{Name: "x", Type: "text", Default: def})
		_, hints, err := Diff(from, to, DiffOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if hasHint(hints, "risk:") {
			t.Fatalf("%s: constant default must not warn: %v", def, hints)
		}
	}
}

func TestVolatileDefaultOnNewTableDoesNotWarn(t *testing.T) {
	from := base()
	to := base()
	to.Tables = append(to.Tables, schema.Table{Name: "t2", Columns: []schema.Column{{Name: "id", Type: "uuid", Default: "gen_random_uuid()"}}})
	_, hints, err := Diff(from, to, DiffOptions{})
	if err != nil || hasHint(hints, "risk:") {
		t.Fatalf("a new table has no rewrite: %v %v", hints, err)
	}
}

func hasHint(hints []string, sub string) bool {
	for _, h := range hints {
		if strings.Contains(h, sub) {
			return true
		}
	}
	return false
}
