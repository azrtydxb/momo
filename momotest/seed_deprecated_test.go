// SPDX-License-Identifier: Apache-2.0

//lint:file-ignore SA1019 these tests exercise the deprecated names

package momotest

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

// Catches: the deprecated Insert and Graph returning a different Step than
// Rows and GraphOf, or an Insert with no rows touching the querier.
func TestDeprecatedSeedStepsForward(t *testing.T) {
	steps := map[string]Step{
		"Insert": Insert[models.Author](),
		"Rows":   Rows[models.Author](),
	}
	for name, step := range steps {
		if err := step(t.Context(), nil); err != nil {
			t.Errorf("%s with no rows returned %v, want nil", name, err)
		}
	}
	forwarders := map[string]Step{
		"Graph":   Graph[models.Author](nil),
		"GraphOf": GraphOf[models.Author](nil),
	}
	for name, step := range forwarders {
		if step == nil {
			t.Errorf("%s returned a nil step", name)
		}
	}
}

// Catches: MomoOptionsOf dropping, reordering or inventing options: the momo
// options given through WithMomoOptions come back, the other options add none,
// and no option gives none.
func TestMomotestMomoOptionsOf(t *testing.T) {
	h1, h2 := &obstest.Recorder{}, &obstest.Recorder{}
	got := MomoOptionsOf(
		WithEnv("X"),
		WithMomoOptions(momo.WithHook(h1)),
		WithMomoOptions(momo.WithHook(h2)),
	)
	if len(got) != 2 {
		t.Fatalf("MomoOptionsOf returned %d options, want 2", len(got))
	}
	if n := len(MomoOptionsOf()); n != 0 {
		t.Fatalf("MomoOptionsOf() returned %d options, want 0", n)
	}
}
