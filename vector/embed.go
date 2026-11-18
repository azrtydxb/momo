// SPDX-License-Identifier: Apache-2.0

package vector

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strconv"
)

// Embedder turns texts into embeddings. The application implements it for its
// provider: Momo ships no provider and makes no network call of its own. It is
// what momo.SyncEmbeddings calls, with no database statement open and no
// transaction held, so a slow provider blocks nothing.
type Embedder interface {
	// Embed returns one vector per text, in the order of texts. A different
	// count, or a vector of the wrong dimension, makes momo.SyncEmbeddings
	// fail with momo.ErrValidation before it writes the batch.
	Embed(ctx context.Context, texts []string) ([]Vector, error)
}

// SourceHash returns the hash of a row's embedding sources, as the hash column
// of an embed_from field stores it: the SHA-256 of the sources encoded in
// order, as 64 lowercase hexadecimal characters. A NULL source encodes as the
// byte N. Any other source encodes as the byte S, its length in bytes in
// decimal, a colon and its bytes, so the encoding is prefix-free: no two
// different lists of sources encode alike, whatever characters the values
// hold. The hash depends only on the values and their order, never on the
// model, the column names or the database. It does not change between
// releases; a change of the encoding would make every row stale.
func SourceHash(sources ...sql.NullString) string {
	var buf []byte
	for _, s := range sources {
		if !s.Valid {
			buf = append(buf, 'N')
			continue
		}
		buf = append(buf, 'S')
		buf = strconv.AppendInt(buf, int64(len(s.String)), 10)
		buf = append(buf, ':')
		buf = append(buf, s.String...)
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:])
}
