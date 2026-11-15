// SPDX-License-Identifier: Apache-2.0

package mysql

import "testing"

// Catches: the parser of another index (or none) read for a key, and a key
// name with a backtick not matched.
func TestKeyParser(t *testing.T) {
	ddl := "CREATE TABLE `articles` (\n" +
		"  `id` bigint NOT NULL,\n" +
		"  `title` varchar(255) NOT NULL,\n" +
		"  PRIMARY KEY (`id`),\n" +
		"  FULLTEXT KEY `plain` (`title`),\n" +
		"  FULLTEXT KEY `cjk` (`title`) /*!50100 WITH PARSER `ngram` */ ,\n" +
		"  FULLTEXT KEY `we``ird` (`title`) /*!50100 WITH PARSER `mecab` */ \n" +
		") ENGINE=InnoDB"
	for name, want := range map[string]string{"plain": "", "cjk": "ngram", "we`ird": "mecab", "absent": ""} {
		if got := keyParser(ddl, name); got != want {
			t.Errorf("keyParser(%q) = %q, want %q", name, got, want)
		}
	}
}
