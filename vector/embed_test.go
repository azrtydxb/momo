// SPDX-License-Identifier: Apache-2.0

package vector

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"testing"
)

func nullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

// Catches: a change to the documented encoding. Every stored hash would turn
// stale, so the bytes are pinned here from the documented rule (N for NULL,
// S, the byte length, a colon and the bytes for a value), not from the code.
func TestSourceHashEncoding(t *testing.T) {
	sum := sha256.Sum256([]byte("S3:abcNS0:S6:héllo"))
	want := hex.EncodeToString(sum[:])
	if got := SourceHash(nullStr("abc"), sql.NullString{}, nullStr(""), nullStr("héllo")); got != want {
		t.Fatalf("hash = %s, want %s", got, want)
	}
}

// Catches: two different lists of sources hashing alike (a value moving across
// a boundary, NULL standing in for an empty string, a reorder), and a hash that
// is not 64 lowercase hex characters.
func TestSourceHashSeparatesSources(t *testing.T) {
	hashes := map[string]string{
		"ab|c":    SourceHash(nullStr("ab"), nullStr("c")),
		"a|bc":    SourceHash(nullStr("a"), nullStr("bc")),
		"c|ab":    SourceHash(nullStr("c"), nullStr("ab")),
		"abc":     SourceHash(nullStr("abc")),
		"null":    SourceHash(sql.NullString{}),
		"empty":   SourceHash(nullStr("")),
		"none":    SourceHash(),
		"S1:aN":   SourceHash(nullStr("a"), sql.NullString{}),
		"digits":  SourceHash(nullStr("1:a")),
		"digits2": SourceHash(nullStr("1"), nullStr("a")),
	}
	seen := map[string]string{}
	for name, h := range hashes {
		if len(h) != 64 || strings.ToLower(h) != h {
			t.Fatalf("%s: hash %q is not 64 lowercase hex characters", name, h)
		}
		if _, err := hex.DecodeString(h); err != nil {
			t.Fatalf("%s: hash %q is not hex: %v", name, h, err)
		}
		if prev, dup := seen[h]; dup {
			t.Fatalf("%s and %s hash alike", name, prev)
		}
		seen[h] = name
	}
	first, again := SourceHash(nullStr("x"), nullStr("y")), SourceHash(nullStr("x"), nullStr("y"))
	if first != again {
		t.Fatal("hash is not deterministic")
	}
}
