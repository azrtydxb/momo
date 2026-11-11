// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// seenDialect records the session and inspect options an engine hands its
// dialect.
type seenDialect struct {
	recDialect
	inspect *migrate.InspectOptions
}

func (d seenDialect) Inspect(_ context.Context, _ momo.PinnedConn, o migrate.InspectOptions) (*schema.Schema, error) {
	*d.inspect = o
	return &schema.Schema{}, nil
}

// observe runs an engine built from opts, so its dialect records what it was handed.
func observe(t *testing.T, opts []migrate.Option) {
	t.Helper()
	e, err := migrate.NewEngine(recDriver{}, fstest.MapFS{}, append(opts, migrate.WithoutSum(), migrate.WithLockWait(0))...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Apply(context.Background(), migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Inspect(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// Catches: the CLI and migrate.ConfigFromFile reading the same momo.yaml
// differently (a copied mapping drifting), which makes an in-process migration
// manage other tables or use other timeouts than `momo migrate`.
func TestConfigFromFileEqualsCLIOptions(t *testing.T) {
	body := "migrate:\n  schemas: [app]\n  exclude: [\"audit_*\"]\n  external_extensions: [postgis]\n  lock_timeout: 0s\n  statement_timeout: 45s\n" +
		"introspect:\n  only: [accounts, \"app.*\"]\n"
	dir := t.TempDir()
	file, _, err := loadConfigFile(yamlPath(t, dir, body), dir)
	if err != nil {
		t.Fatal(err)
	}

	var cliSess, libSess migrate.SessionOptions
	var cliInsp, libInsp migrate.InspectOptions
	cliDialect := seenDialect{recDialect{&cliSess}, &cliInsp}
	libDialect := seenDialect{recDialect{&libSess}, &libInsp}

	cliOpts, err := (&cli{file: file}).engineOptions(cliDialect)
	if err != nil {
		t.Fatal(err)
	}
	libOpts, err := migrate.ConfigFromFile(fstest.MapFS{"momo.yaml": {Data: []byte("version: 1\n" + body)}}, "momo.yaml")
	if err != nil {
		t.Fatal(err)
	}
	observe(t, cliOpts)
	observe(t, append([]migrate.Option{migrate.WithDialect(libDialect)}, libOpts...))

	if libSess != cliSess {
		t.Fatalf("session options: library %+v, CLI %+v", libSess, cliSess)
	}
	if !reflect.DeepEqual(libInsp, cliInsp) {
		t.Fatalf("inspect options: library %+v, CLI %+v", libInsp, cliInsp)
	}
	if libSess.LockTimeout != 0 || !libSess.LockTimeoutSet || libSess.StatementTimeout != 45*time.Second {
		t.Fatalf("session options = %+v: want an explicit 0s lock timeout and a 45s statement timeout", libSess)
	}
	if !slices.Equal(libInsp.Only, []string{"accounts", "app.*"}) || !slices.Equal(libInsp.Exclude, []string{"audit_*"}) || !slices.Equal(libInsp.Schemas, []string{"app"}) {
		t.Fatalf("inspect options = %+v", libInsp)
	}
}

// Catches: ConfigFromFile accepting a momo.yaml without a version, or one that
// momo itself would reject on a bad duration or an empty only glob.
func TestConfigFromFileRejectsBadFiles(t *testing.T) {
	for name, body := range map[string]string{
		"no version":    "migrate:\n  lock_timeout: 1s\n",
		"bad duration":  "version: 1\nmigrate:\n  lock_timeout: soon\n",
		"empty only":    "version: 1\nintrospect:\n  only: [\"\"]\n",
		"empty file":    "",
		"wrong version": "version: 2\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := migrate.ConfigFromFile(fstest.MapFS{"momo.yaml": {Data: []byte(body)}}, "momo.yaml"); err == nil {
				t.Fatal("want an error")
			}
		})
	}
	if _, err := migrate.ConfigFromFile(fstest.MapFS{}, "momo.yaml"); err == nil {
		t.Fatal("a missing file must be an error")
	}
}

// Catches: the State constants drifting from the strings Status returns.
func TestStateConstants(t *testing.T) {
	for c, want := range map[migrate.State]string{
		migrate.StatePending: "pending", migrate.StateApplied: "applied", migrate.StateDirty: "dirty",
		migrate.StateMismatch: "mismatch", migrate.StateMissing: "missing",
	} {
		if string(c) != want {
			t.Fatalf("%q != %q", c, want)
		}
	}
}
