// SPDX-License-Identifier: Apache-2.0

package vectortype

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

func TestMySQLVectorType(t *testing.T) {
	cases := []struct {
		name    string
		flavor  dialect.Flavor
		version string
		dim     int
		want    string
		bad     bool
	}{
		{"mysql 9.7", dialect.FlavorMySQL, "9.7.4", 3, "VECTOR(3)", false},
		{"mysql 9.0", dialect.FlavorMySQL, "9.0.0", 1536, "VECTOR(1536)", false},
		{"mysql 9.7 suffix", dialect.FlavorMySQL, "9.7.4-log", 3, "VECTOR(3)", false},
		{"mysql 8.4", dialect.FlavorMySQL, "8.4.13", 3, "", true},
		{"mysql 8.0", dialect.FlavorMySQL, "8.0.40", 3, "", true},
		{"mariadb 11.8", dialect.FlavorMariaDB, "11.8.9-MariaDB", 8, "VECTOR(8)", false},
		{"mariadb 11.8 replication prefix", dialect.FlavorMariaDB, "5.5.5-11.8.9-MariaDB-ubu2404", 8, "VECTOR(8)", false},
		{"mariadb 11.8 ubuntu suffix", dialect.FlavorMariaDB, "11.8.2-MariaDB-ubu2404", 8, "VECTOR(8)", false},
		{"mariadb 11.7", dialect.FlavorMariaDB, "11.7.2-MariaDB", 8, "VECTOR(8)", false},
		{"mariadb 10.11", dialect.FlavorMariaDB, "10.11.6-MariaDB", 8, "", true},
		{"mariadb 11.6", dialect.FlavorMariaDB, "11.6.2-MariaDB", 8, "", true},
		{"no dimension", dialect.FlavorMariaDB, "11.8.9-MariaDB", 0, "", true},
		{"garbage version", dialect.FlavorMySQL, "abc", 3, "", true},
		{"unknown flavor", dialect.Flavor("oracle"), "9.7.4", 3, "", true},
	}
	for _, c := range cases {
		got, err := MySQL(c.flavor, c.version, c.dim)
		if c.bad {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s: err = %v, want ErrUnsupported", c.name, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: %q, %v; want %q", c.name, got, err, c.want)
		}
	}
	_, err := MySQL(dialect.FlavorMySQL, "8.4.13", 3)
	if err == nil || !strings.Contains(err.Error(), "8.4.13") {
		t.Fatalf("diagnostic %v does not name the server version", err)
	}
	if _, err := MySQL(dialect.FlavorMariaDB, "10.11.6-MariaDB", 3); err == nil || !strings.Contains(err.Error(), "VECTOR requires MySQL 9.0 or MariaDB 11.7") {
		t.Fatalf("MariaDB diagnostic %v lacks the requirement text", err)
	}
}
