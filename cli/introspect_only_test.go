// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/config"
	"github.com/azrtydxb/momo/gen/fromdb"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a fromdb.Config key (introspect: section of momo.yaml) that the
// --from-db usage text does not list, so a user cannot discover it.
func TestGenerateUsageListsEveryIntrospectKey(t *testing.T) {
	start := strings.Index(generateUsage, "go in the introspect section of momo.yaml")
	if start < 0 {
		t.Fatal("usage text has no introspect section paragraph")
	}
	para := generateUsage[start:]
	if end := strings.Index(para, "\n\n"); end >= 0 {
		para = para[:end]
	}
	rt := reflect.TypeOf(fromdb.Config{})
	for i := range rt.NumField() {
		key, _, _ := strings.Cut(rt.Field(i).Tag.Get("yaml"), ",")
		if key == "" || key == "-" {
			t.Fatalf("field %s has no yaml key", rt.Field(i).Name)
		}
		if !regexp.MustCompile(`\b` + regexp.QuoteMeta(key) + `\b`).MatchString(para) {
			t.Errorf("--from-db usage does not list the introspect key %q", key)
		}
	}
}

// Catches: introspect.only globs that match nothing going unreported: a typo,
// or public.users in the current schema, would filter every table out and the
// run would end as "the database has no tables to model" with exit 0.
func TestCheckOnlyMatched(t *testing.T) {
	users := schema.Table{Name: "users"}
	orders := schema.Table{Schema: "app", Name: "orders"}
	var said []string
	say := func(f string, a ...any) { said = append(said, fmt.Sprintf(f, a...)) }

	if err := checkOnlyMatched(nil, nil, say); err != nil || len(said) != 0 {
		t.Fatalf("no only: err=%v said=%v", err, said)
	}
	for _, only := range [][]string{{"usres"}, {"public.users"}} {
		err := checkOnlyMatched(only, nil, say)
		if err == nil || !strings.Contains(err.Error(), "introspect.only matched no table") {
			t.Fatalf("only %v, no tables: err = %v", only, err)
		}
	}
	said = nil
	if err := checkOnlyMatched([]string{"users", "app.*"}, []schema.Table{users, orders}, say); err != nil || len(said) != 0 {
		t.Fatalf("all matched: err=%v said=%v", err, said)
	}
	if err := checkOnlyMatched([]string{"users", "usres", "public.x"}, []schema.Table{users}, say); err != nil {
		t.Fatal(err)
	}
	if len(said) != 1 || !strings.Contains(said[0], "usres, public.x") || !strings.Contains(said[0], "matched no table") {
		t.Fatalf("said = %v, want one warning naming usres and public.x", said)
	}
}

// Catches: a bad introspect.only reaching the engine, where the error names
// migrate.Config.OnlyTables instead of the momo.yaml key.
func TestEngineOptionsValidatesIntrospect(t *testing.T) {
	l := &cli{file: &config.File{}, dialectName: string(dialect.Postgres)}
	l.file.Introspect.Only = []string{"["}
	_, err := l.engineOptions(mpg.New())
	if err == nil || !strings.Contains(err.Error(), "introspect.only[0]") {
		t.Fatalf("err = %v, want an introspect.only[0] error", err)
	}
}
