// SPDX-License-Identifier: Apache-2.0

package types

import "github.com/jackc/pgx/v5/pgtype"

// UUIDValue implements pgtype.UUIDValuer, so pgx encodes a UUID, and a slice
// of them as a uuid[] parameter, in binary form. Without it pgx cannot encode
// the elements of a []UUID: it only falls back to driver.Valuer for scalars.
func (u UUID) UUIDValue() (pgtype.UUID, error) {
	return pgtype.UUID{Bytes: u, Valid: true}, nil
}
