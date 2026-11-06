// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"testing"

	"github.com/azrtydxb/momo/schema"
)

func TestDefaultNeedsNoRewrite(t *testing.T) {
	for d, want := range map[string]bool{
		"":                       true,
		"NULL":                   true,
		"0":                      true,
		"-1.5":                   true,
		"true":                   true,
		"'x'":                    true,
		"'it''s'":                true,
		"'x'::text":              true,
		"'{}'::jsonb":            true,
		"now()":                  true,
		"CURRENT_TIMESTAMP":      true,
		"current_timestamp":      true,
		"random()":               false,
		"gen_random_uuid()":      false,
		"nextval('s')":           false,
		"1 + random()":           false,
		"'a' || random()::text":  false,
		"(SELECT 1)":             false,
		"clock_timestamp()":      false,
		"now() + interval '1 d'": false,
	} {
		if got := defaultNeedsNoRewrite(d); got != want {
			t.Errorf("%q: got %v want %v", d, got, want)
		}
	}
}

func TestAddColumnRewrites(t *testing.T) {
	if addColumnRewriteReason(&schema.Column{Name: "a", Type: "int", Nullable: true}) != "" {
		t.Fatal("plain nullable column must not rewrite")
	}
	for _, c := range []schema.Column{
		{Name: "a", Type: "int", Generated: "1"},
		{Name: "a", Type: "int", Identity: "always"},
		{Name: "a", Type: "int", Default: "random()"},
	} {
		if addColumnRewriteReason(&c) == "" {
			t.Errorf("%+v must be skipped", c)
		}
	}
}
