// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// seenDialect records the session and inspect options an engine hands its
// dialect, and reads two extensions so a hidden one shows in Inspect.
type seenDialect struct {
	recDialect
	inspect *migrate.InspectOptions
}

func (d seenDialect) Inspect(_ context.Context, _ momo.PinnedConn, o migrate.InspectOptions) (*schema.Schema, error) {
	*d.inspect = o
	return &schema.Schema{Extensions: []schema.Extension{{Name: "postgis"}, {Name: "hstore"}}}, nil
}

// observe runs an engine built from opts and returns the extensions Inspect
// reports, so its dialect records what it was handed.
func observe(t *testing.T, opts []migrate.Option) []string {
	t.Helper()
	e, err := migrate.NewEngine(recDriver{}, fstest.MapFS{}, append(opts, migrate.WithoutSum(), migrate.WithLockWait(0))...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Apply(context.Background(), migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	s, err := e.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, x := range s.Extensions {
		names = append(names, x.Name)
	}
	return names
}

func libFS(body string) fstest.MapFS {
	return fstest.MapFS{"momo.yaml": {Data: []byte("version: 1\n" + body)}}
}

// Catches: the CLI and migrate.ConfigFromFile reading the same momo.yaml
// differently (a copied mapping drifting), which makes an in-process migration
// manage other tables, hide other extensions or use other timeouts than
// `momo migrate`.
func TestConfigFromFileEqualsCLIOptions(t *testing.T) {
	for name, body := range map[string]string{
		"all keys": "migrate:\n  schemas: [app]\n  exclude: [\"audit_*\"]\n  external_extensions: [postgis]\n  lock_timeout: 0s\n  statement_timeout: 45s\n" +
			"introspect:\n  only: [accounts, \"app.*\"]\n",
		"no timeout keys": "migrate:\n  external_extensions: [hstore]\n  exclude: [\"tmp_*\"]\n",
		"empty":           "",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			file, _, err := loadConfigFile(yamlPath(t, dir, body), dir)
			if err != nil {
				t.Fatal(err)
			}
			var cliSess, libSess migrate.SessionOptions
			var cliInsp, libInsp migrate.InspectOptions
			cliOpts, err := (&cli{file: file}).engineOptions(seenDialect{recDialect{&cliSess}, &cliInsp})
			if err != nil {
				t.Fatal(err)
			}
			libOpts, err := migrate.ConfigFromFile(libFS(body), "momo.yaml")
			if err != nil {
				t.Fatal(err)
			}
			cliExt := observe(t, cliOpts)
			libExt := observe(t, append([]migrate.Option{migrate.WithDialect(seenDialect{recDialect{&libSess}, &libInsp})}, libOpts...))

			if libSess != cliSess {
				t.Fatalf("session options: library %+v, CLI %+v", libSess, cliSess)
			}
			if !reflect.DeepEqual(libInsp, cliInsp) {
				t.Fatalf("inspect options: library %+v, CLI %+v", libInsp, cliInsp)
			}
			if !slices.Equal(libExt, cliExt) {
				t.Fatalf("extensions reported: library %v, CLI %v", libExt, cliExt)
			}
			switch name {
			case "all keys":
				if libSess.LockTimeout != 0 || !libSess.LockTimeoutSet || libSess.StatementTimeout != 45*time.Second {
					t.Fatalf("session options = %+v: want an explicit 0s lock timeout and a 45s statement timeout", libSess)
				}
				if !slices.Equal(libInsp.Only, []string{"accounts", "app.*"}) || !slices.Equal(libInsp.Exclude, []string{"audit_*"}) || !slices.Equal(libInsp.Schemas, []string{"app"}) {
					t.Fatalf("inspect options = %+v", libInsp)
				}
				if !slices.Equal(libExt, []string{"hstore"}) {
					t.Fatalf("extensions = %v, want postgis hidden", libExt)
				}
			case "no timeout keys":
				if libSess.LockTimeoutSet || libSess.StatementTimeoutSet {
					t.Fatalf("session options = %+v: no timeout key was given", libSess)
				}
				if !slices.Equal(libExt, []string{"postgis"}) {
					t.Fatalf("extensions = %v, want hstore hidden", libExt)
				}
			}
		})
	}
}

// Catches: ConfigFromFile accepting what `momo` rejects, so a typo that stops
// the CLI is silently ignored by an in-process migration: unknown keys in
// migrate, introspect or the top level, several documents, a missing or wrong
// version, a bad duration, a bad or repeated extension name, a bad only glob.
func TestConfigFromFileRejectsBadFiles(t *testing.T) {
	for name, tc := range map[string]struct{ body, want string }{
		"migrate key typo":       {"version: 1\nmigrate:\n  exclud: [a]\n", "exclud"},
		"lock_timeout typo":      {"version: 1\nmigrate:\n  lock_timout: 1s\n", "lock_timout"},
		"introspect key typo":    {"version: 1\nintrospect:\n  onlly: [a]\n", "onlly"},
		"top-level key typo":     {"version: 1\nmigrat:\n  exclude: [a]\n", "migrat"},
		"syntax error":           {"version: 1\nmigrate: [\n", ""},
		"multiple documents":     {"version: 1\n---\nversion: 1\n", "multiple"},
		"no version":             {"migrate:\n  lock_timeout: 1s\n", "version"},
		"wrong version":          {"version: 2\n", "version"},
		"empty file":             {"", "empty"},
		"bad duration":           {"version: 1\nmigrate:\n  lock_timeout: soon\n", "lock_timeout"},
		"empty only glob":        {"version: 1\nintrospect:\n  only: [\"\"]\n", "introspect.only[0]"},
		"unknown dialect":        {"version: 1\nmigrate:\n  dir:\n    nosuch: x\n", "nosuch"},
		"extension upper case":   {"version: 1\nmigrate:\n  external_extensions: [PostGIS]\n", "external_extensions"},
		"extension qualified":    {"version: 1\nmigrate:\n  external_extensions: [public.postgis]\n", "external_extensions"},
		"extension listed twice": {"version: 1\nmigrate:\n  external_extensions: [postgis, postgis]\n", "listed twice"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := migrate.ConfigFromFile(fstest.MapFS{"momo.yaml": {Data: []byte(tc.body)}}, "momo.yaml")
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
	if _, err := migrate.ConfigFromFile(fstest.MapFS{}, "momo.yaml"); err == nil {
		t.Fatal("a missing file must be an error")
	}
}

// Catches: ConfigFromFile rejecting the keys of a real momo.yaml that only
// code generation reads, which would stop an app from embedding the one file.
func TestConfigFromFileIgnoresGenerationKeys(t *testing.T) {
	body := "packages: [./models]\nnaming: gorm\ndialects: [postgres]\ntags: [x]\ntypes:\n  time.Time: timestamptz\nplugins:\n  openapi:\n    out: api.json\n" +
		"introspect:\n  package: models\n  skip: [tmp_*]\n  uuid: google\n  only: [accounts]\nmigrate:\n  dir:\n    postgres: migrations/pg\n  url_env:\n    postgres: DB_URL\n  lock_timeout: 2s\n"
	if _, err := migrate.ConfigFromFile(libFS(body), "momo.yaml"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, _, err := loadConfigFile(yamlPath(t, dir, body), dir); err != nil {
		t.Fatalf("the CLI must accept the same file: %v", err)
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
