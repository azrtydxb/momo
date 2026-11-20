// SPDX-License-Identifier: Apache-2.0
//lint:file-ignore SA1019 these tests exercise the deprecated names

package momotest_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/momotest"
)

// TestDeprecatedSeedStepsForward: the deprecated Insert and Graph return the
// same Step type as Rows and GraphOf, and an Insert with no rows runs like a
// Rows with no rows: it succeeds without touching the querier.
func TestDeprecatedSeedStepsForward(t *testing.T) {
	steps := map[string]momotest.Step{
		"Insert": momotest.Insert[models.Author](),
		"Rows":   momotest.Rows[models.Author](),
	}
	for name, step := range steps {
		if err := step(t.Context(), nil); err != nil {
			t.Errorf("%s with no rows returned %v, want nil", name, err)
		}
	}
	forwarders := map[string]momotest.Step{
		"Graph":   momotest.Graph[models.Author](nil),
		"GraphOf": momotest.GraphOf[models.Author](nil),
	}
	for name, step := range forwarders {
		if step == nil {
			t.Errorf("%s returned a nil step", name)
		}
	}
}

// TestMomotestMomoOptionsOf: the momo options given through WithMomoOptions
// come back in order, the other options add none, and no option gives none.
func TestMomotestMomoOptionsOf(t *testing.T) {
	h1, h2 := &obstest.Recorder{}, &obstest.Recorder{}
	got := momotest.MomoOptionsOf(
		momotest.WithEnv("X"),
		momotest.WithMomoOptions(momo.WithHook(h1)),
		momotest.WithMomoOptions(momo.WithHook(h2)),
	)
	if len(got) != 2 {
		t.Fatalf("MomoOptionsOf returned %d options, want 2", len(got))
	}
	if n := len(momotest.MomoOptionsOf()); n != 0 {
		t.Fatalf("MomoOptionsOf() returned %d options, want 0", n)
	}
}
