// SPDX-License-Identifier: Apache-2.0

package schemautil

// FTS5Table is the name of the FTS5 virtual table that indexes table.
func FTS5Table(table string) string { return table + "_fts" }

// TSVectorColumn is the name of the generated tsvector column of the
// full-text index named index.
func TSVectorColumn(index string) string { return index + "_tsv" }
