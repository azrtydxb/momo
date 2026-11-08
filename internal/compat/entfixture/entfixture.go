// SPDX-License-Identifier: Apache-2.0

// Package entfixture is a small project written with Ent. The importer reads
// the schema in ent/schema, and its test compares the schema Ent's own
// migration creates in PostgreSQL with the schema Momo derives from the
// imported models. The Ent client in ent/ is generated from that schema; run
// go generate ./ent after changing it.
package entfixture
