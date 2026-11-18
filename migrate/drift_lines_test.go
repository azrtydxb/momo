// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
)

// Catches: a very long list of differing objects flooding the refusal, or the
// cut-off losing the count of what it left out.
func TestBaselineMismatchErrorCapsTheList(t *testing.T) {
	rep := &migrate.DriftReport{}
	for i := range 60 {
		rep.Missing = append(rep.Missing, migrate.ObjectRef{Kind: "check", Name: fmt.Sprintf("t.c%02d", i)})
	}
	err := error(&migrate.BaselineMismatchError{Report: rep})
	if !errors.Is(err, migrate.ErrBaselineMismatch) {
		t.Fatal("the error must wrap ErrBaselineMismatch")
	}
	msg := err.Error()
	if !strings.Contains(msg, "missing: check t.c00") || !strings.Contains(msg, "missing: check t.c49") {
		t.Errorf("the first 50 objects must be named:\n%s", msg)
	}
	if strings.Contains(msg, "t.c50") || !strings.Contains(msg, "and 10 more") {
		t.Errorf("the rest must be counted, not listed:\n%s", msg)
	}
	if got := len(rep.Lines(0)); got != 60 {
		t.Errorf("Lines(0) = %d lines, want all 60", got)
	}
}

// Catches: a changed object named without its definitions, which is what the
// issue asked for, and a schema-qualified name losing its schema.
func TestDriftLinesNameKindObjectAndDefinitions(t *testing.T) {
	rep := &migrate.DriftReport{
		Changed: []migrate.ObjectDiff{{
			Object:   migrate.ObjectRef{Kind: "column", Schema: "app", Name: "items.status"},
			Detail:   "default, nullable differ",
			Snapshot: `default="'new'", nullable=false`,
			Live:     "default=none, nullable=true",
		}},
		Unmanaged: []migrate.ObjectRef{{Kind: "table", Name: "scratch"}},
	}
	got := rep.Lines(0)
	want := []string{
		`changed: column app.items.status: default, nullable differ; live: default=none, nullable=true; snapshot: default="'new'", nullable=false`,
		"unmanaged: table scratch",
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}
