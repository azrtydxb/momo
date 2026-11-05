// SPDX-License-Identifier: Apache-2.0

package testdb

import (
	"os"
	"regexp"
	"slices"
	"testing"
)

// pinnedPgvector is the pgvector release every test database must run.
const pinnedPgvector = "0.8.7"

func matches(t *testing.T, path string, re *regexp.Regexp) [][]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return re.FindAllStringSubmatch(string(b), -1)
}

// TestPgvectorPinsAgree keeps compose.yaml and the CI matrix on the same
// pinned pgvector images, so local and CI runs exercise the same extension.
func TestPgvectorPinsAgree(t *testing.T) {
	var compose []string
	for _, m := range matches(t, "compose.yaml", regexp.MustCompile(`image: pgvector/pgvector:(\S+)`)) {
		compose = append(compose, m[1])
	}
	slices.Sort(compose)

	var ci []string
	for _, m := range matches(t, "../../.github/workflows/ci.yml", regexp.MustCompile(`pg: \["([^"]+)", "([^"]+)"\]`)) {
		ci = append(ci, m[1], m[2])
	}
	slices.Sort(ci)

	want := []string{pinnedPgvector + "-pg16", pinnedPgvector + "-pg17"}
	if !slices.Equal(compose, want) {
		t.Errorf("compose.yaml pgvector images = %v, want %v", compose, want)
	}
	if !slices.Equal(ci, want) {
		t.Errorf("ci.yml pgvector matrix = %v, want %v", ci, want)
	}
}
