// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// onlyDialect records the InspectOptions the engine hands the dialect.
type onlyDialect struct {
	recDialect
	got *migrate.InspectOptions
}

func (d onlyDialect) Inspect(_ context.Context, _ momo.PinnedConn, o migrate.InspectOptions) (*schema.Schema, error) {
	*d.got = o
	return &schema.Schema{}, nil
}

// Catches: introspect.only not reaching the dialect's inspection (the wiring
// momo.yaml -> engineOptions -> Engine -> InspectOptions.Only), which would
// make every table readable again and let one the model cannot represent fail
// the run.
func TestIntrospectOnlyReachesInspection(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "introspect:\n  only: [accounts, \"app.*\"]\n")
	file, _, err := loadConfigFile(cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	l := &cli{file: file}
	var got migrate.InspectOptions
	opts, err := l.engineOptions(onlyDialect{recDialect{new(migrate.SessionOptions)}, &got})
	if err != nil {
		t.Fatal(err)
	}
	e, err := migrate.NewEngine(recDriver{}, fstest.MapFS{}, append(opts, migrate.WithoutSum(), migrate.WithLockWait(0))...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Inspect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Only, []string{"accounts", "app.*"}) {
		t.Fatalf("InspectOptions.Only = %v, want [accounts app.*]", got.Only)
	}
}

// Catches: an empty introspect.only entry reaching migrate, drift and baseline
// unvalidated, where it would hide every table.
func TestIntrospectOnlyEmptyEntryIsRejected(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "introspect:\n  only: [\"\"]\n")
	if _, _, err := loadConfigFile(cfg, dir); err == nil || !strings.Contains(err.Error(), "introspect.only[0]") {
		t.Fatalf("err = %v, want an introspect.only[0] error", err)
	}
}
