// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
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

	if err := checkOnlyMatched(nil, nil, nil, say); err != nil || len(said) != 0 {
		t.Fatalf("no only: err=%v said=%v", err, said)
	}
	for _, only := range [][]string{{"usres"}, {"public.users"}} {
		err := checkOnlyMatched(only, nil, nil, say)
		if err == nil || !strings.Contains(err.Error(), "introspect.only matched no table") {
			t.Fatalf("only %v, no tables: err = %v", only, err)
		}
	}
	said = nil
	if err := checkOnlyMatched([]string{"users", "app.*"}, []schema.Table{users, orders}, nil, say); err != nil || len(said) != 0 {
		t.Fatalf("all matched: err=%v said=%v", err, said)
	}
	if err := checkOnlyMatched([]string{"users", "usres", "public.x"}, []schema.Table{users}, nil, say); err != nil {
		t.Fatal(err)
	}
	if len(said) != 1 || !strings.Contains(said[0], "usres, public.x") || !strings.Contains(said[0], "matched no table") {
		t.Fatalf("said = %v, want one warning naming usres and public.x", said)
	}
}

// Catches: an introspect.only entry that names a standalone sequence being
// reported as matching no table, and failing the run when every entry is one.
func TestCheckOnlyMatchedCountsSequences(t *testing.T) {
	users := schema.Table{Name: "users"}
	jobs := schema.Sequence{Name: "jobs_seq"}
	var said []string
	say := func(f string, a ...any) { said = append(said, fmt.Sprintf(f, a...)) }

	if err := checkOnlyMatched([]string{"users", "jobs_*"}, []schema.Table{users}, []schema.Sequence{jobs}, say); err != nil || len(said) != 0 {
		t.Fatalf("table and sequence entry: err=%v said=%v", err, said)
	}
	// A sequence alone cannot be modeled: with no table matched the run is still
	// the usage error, even when every entry names a sequence.
	err := checkOnlyMatched([]string{"jobs_seq"}, nil, []schema.Sequence{jobs}, say)
	if err == nil || !strings.Contains(err.Error(), "introspect.only matched no table") {
		t.Fatalf("only a sequence entry, no table: err = %v, want the matched-no-table error", err)
	}
	if len(said) != 0 {
		t.Fatalf("said = %v, want no warning before the error", said)
	}
	if err := checkOnlyMatched([]string{"users", "nope_seq"}, []schema.Table{users}, []schema.Sequence{jobs}, say); err != nil {
		t.Fatal(err)
	}
	if len(said) != 1 || !strings.Contains(said[0], "nope_seq") {
		t.Fatalf("said = %v, want one warning naming nope_seq", said)
	}
}

// Catches: a sequence that --from-db left out of the models and the snapshot
// still being unmanaged for drift and baseline, with no hint to exclude it.
func TestExcludeHintNamesOmittedSequences(t *testing.T) {
	var out bytes.Buffer
	l := &cli{file: &config.File{}, out: &out}
	l.file.Migrate.Exclude = []string{"seen_seq"}
	omitted := omittedSequences(
		[]schema.Sequence{{Name: "kept_seq"}, {Name: "seen_seq"}, {Name: "gone_seq"}, {Schema: "app", Name: "other_seq"}},
		[]schema.Sequence{{Name: "kept_seq"}})
	l.excludeHint(omitted)
	said := out.String()
	if strings.Count(said, "\n") != 1 || !strings.Contains(said, "gone_seq") || !strings.Contains(said, "app.other_seq") || strings.Contains(said, "seen_seq") || strings.Contains(said, "kept_seq") {
		t.Fatalf("said = %q, want one hint naming gone_seq and app.other_seq only", said)
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
