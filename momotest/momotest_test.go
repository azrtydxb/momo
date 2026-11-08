// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

// fakeTB records Skip and Fatal and stops the calling goroutine, as the real
// ones do, so the no-database paths of New can be tested without a server.
type fakeTB struct {
	testing.TB
	skipped, failed bool
	msg             string
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Skip(args ...any) {
	f.skipped, f.msg = true, fmt.Sprint(args...)
	runtime.Goexit()
}

func (f *fakeTB) Fatal(args ...any) {
	f.failed, f.msg = true, fmt.Sprint(args...)
	runtime.Goexit()
}

func run(fn func()) {
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	<-done
}

const unsetEnv = "MOMOTEST_SURELY_UNSET_VARIABLE"

func TestResolve(t *testing.T) {
	set := func(string) (string, bool) { return "postgres://x", true }
	unset := func(string) (string, bool) { return "", false }

	if v, err := resolve(DefaultEnv, set, true); err != nil || v != "postgres://x" {
		t.Fatalf("set variable: %q, %v", v, err)
	}
	if _, err := resolve(DefaultEnv, unset, false); err != errSkip {
		t.Fatalf("unset, not required: err=%v, want errSkip", err)
	}
	_, err := resolve(DefaultEnv, unset, true)
	if err == nil || err == errSkip || !strings.Contains(err.Error(), DefaultEnv) {
		t.Fatalf("unset, required: err=%v, want an error naming the variable", err)
	}
}

func TestUnsetURLSkips(t *testing.T) {
	t.Setenv(RequireEnv, "")
	for name, fn := range map[string]func(*fakeTB){
		"New":   func(f *fakeTB) { New(f, WithEnv(unsetEnv)) },
		"NewTx": func(f *fakeTB) { NewTx(f, WithEnv(unsetEnv)) },
	} {
		f := &fakeTB{}
		run(func() { fn(f) })
		if !f.skipped || f.failed || !strings.Contains(f.msg, unsetEnv) {
			t.Errorf("%s: skipped=%v failed=%v msg=%q, want a skip naming %s", name, f.skipped, f.failed, f.msg, unsetEnv)
		}
	}
}

func TestUnsetURLFailsWhenRequired(t *testing.T) {
	t.Setenv(RequireEnv, "1")
	f := &fakeTB{}
	run(func() { New(f, WithEnv(unsetEnv)) })
	if f.skipped || !f.failed || !strings.Contains(f.msg, unsetEnv) {
		t.Fatalf("skipped=%v failed=%v msg=%q, want a failure naming %s", f.skipped, f.failed, f.msg, unsetEnv)
	}
}

func TestRandomNameIsUniqueAndSafe(t *testing.T) {
	a, b := randomName("momotest_"), randomName("momotest_")
	if a == b || len(a) != len("momotest_")+16 || strings.ContainsAny(a, `"'; `) {
		t.Fatalf("randomName gave %q and %q", a, b)
	}
}

func TestWithSearchPath(t *testing.T) {
	got, err := withSearchPath("postgres://u:p@h:5432/db?sslmode=disable", "s1")
	if err != nil || !strings.Contains(got, "search_path=s1%2Cpublic") || !strings.Contains(got, "sslmode=disable") {
		t.Fatalf("valid URL: %q, %v", got, err)
	}
	for _, bad := range []string{"host=localhost user=u dbname=d", "mysql://u@h/db", "%zz", ""} {
		if _, err := withSearchPath(bad, "s1"); err == nil || !strings.Contains(err.Error(), "postgres://") {
			t.Errorf("withSearchPath(%q) err = %v, want an error requiring a postgres:// URL", bad, err)
		}
	}
}

// TestCheckMigrationsRejectsEscapes feeds migration SQL kept in
// testdata/escapes to the checker. The SQL is input data that is only
// scanned, never executed, so it lives in fixture files, which the repository
// scan for tests that create extensions in the shared database skips by
// design, and not in this test's source.
func TestCheckMigrationsRejectsEscapes(t *testing.T) {
	fsys := func(file string) fstest.MapFS {
		sql, err := os.ReadFile(filepath.Join("testdata", "escapes", file))
		if err != nil {
			t.Fatal(err)
		}
		return fstest.MapFS{"20260101000000_a.up.sql": {Data: sql}}
	}
	cases := []struct {
		name string
		file string
		opts []Option
		want string // empty: accepted
	}{
		{"plain table", "plain.sql", nil, ""},
		{"unlisted extension", "extension.sql", nil, "vector"},
		{"unlisted quoted extension", "quoted_extension.sql", nil, "pgcrypto"},
		{"listed extension", "extension.sql", []Option{WithExtensions("vector")}, ""},
		{"schema", "schema.sql", nil, "schema"},
		{"schema if not exists", "schema_if_not_exists.sql", nil, "schema"},
		{"quoted extension if not exists", "quoted_if_not_exists.sql", nil, "uuid-ossp"},
		{"mixed case unlisted", "mixed_case.sql", nil, "foo"},
		{"mixed case listed in other case", "mixed_case.sql", []Option{WithExtensions("FOO")}, ""},
		{"comment between the keywords", "comment_between.sql", nil, "foo"},
		{"comments that mention the words", "comment_mentions.sql", nil, ""},
		{"nested comment", "nested_comment.sql", nil, ""},
		{"string literal", "string_literal.sql", nil, ""},
		{"escaped strings", "escaped_strings.sql", nil, ""},
		// Dollar-quoted bodies are scanned as code, so a DO block is caught.
		{"dollar-quoted body", "dollar_body.sql", nil, "hstore"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := newConfig(append([]Option{WithMigrations(fsys(tc.file))}, tc.opts...)).checkMigrations()
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("err = %v, want none", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("err = %v, want one naming %q", err, tc.want)
			}
		})
	}
}

func TestExtensionNamesAreLowercased(t *testing.T) {
	c := newConfig([]Option{WithExtensions("Vector", "PGCrypto")})
	if got := strings.Join(c.extensions, ","); got != "vector,pgcrypto" {
		t.Fatalf("extensions = %q, want vector,pgcrypto", got)
	}
}

func TestTemplateNameTracksMigrations(t *testing.T) {
	name := func(opts ...Option) string {
		n, err := newConfig(opts).templateName()
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	a := fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x int);")}}
	a2 := fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x int);")}}
	b := fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (y int);")}}

	if name(WithMigrations(a)) != name(WithMigrations(a2)) {
		t.Error("equal migrations must share a template")
	}
	if name(WithMigrations(a)) == name(WithMigrations(b)) {
		t.Error("a changed migration must get a new template")
	}
	if name(WithMigrations(a)) == name(WithMigrations(a), WithExtensions("vector")) {
		t.Error("a different extension list must get a new template")
	}
	if n := name(); !strings.HasPrefix(n, "momotest_tpl_") || len(n) != len("momotest_tpl_")+16 {
		t.Errorf("template name %q has the wrong shape", n)
	}
}
