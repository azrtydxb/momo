// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/azrtydxb/momo/dialect"
)

// IterativeScan is the value of an hnsw.iterative_scan or ivfflat.iterative_scan
// setting.
type IterativeScan string

const (
	// IterativeOff disables iterative index scans.
	IterativeOff IterativeScan = "off"
	// IterativeRelaxedOrder keeps scanning until enough rows pass the filter,
	// allowing slightly out-of-order results.
	IterativeRelaxedOrder IterativeScan = "relaxed_order"
	// IterativeStrictOrder keeps exact distance order while scanning (hnsw only).
	IterativeStrictOrder IterativeScan = "strict_order"
)

// SearchSettings applies pgvector session settings for one transaction only.
// Zero values mean "do not change the server default".
type SearchSettings struct {
	ExactSearch          bool          // enable_indexscan = off and enable_bitmapscan = off
	HNSWEfSearch         int           // hnsw.ef_search, 1..1000
	HNSWIterativeScan    IterativeScan // hnsw.iterative_scan
	HNSWMaxScanTuples    int           // hnsw.max_scan_tuples, >= 1 (server default 20000)
	IVFFlatProbes        int           // ivfflat.probes, >= 1
	IVFFlatIterativeScan IterativeScan // ivfflat.iterative_scan: off or relaxed_order only
	IVFFlatMaxProbes     int           // ivfflat.max_probes, >= 1
}

func searchInvalid(format string, args ...any) error {
	return &Error{Kind: ErrValidation, Op: "search settings", Err: fmt.Errorf(format, args...)}
}

// Validate reports a setting outside its pgvector range. The error wraps
// ErrValidation.
func (s SearchSettings) Validate() error {
	if s.HNSWEfSearch < 0 || s.HNSWEfSearch > 1000 {
		return searchInvalid("hnsw.ef_search %d is outside 1..1000", s.HNSWEfSearch)
	}
	switch s.HNSWIterativeScan {
	case "", IterativeOff, IterativeRelaxedOrder, IterativeStrictOrder:
	default:
		return searchInvalid("hnsw.iterative_scan %q is not off, relaxed_order or strict_order", s.HNSWIterativeScan)
	}
	if s.HNSWMaxScanTuples < 0 {
		return searchInvalid("hnsw.max_scan_tuples %d must be at least 1", s.HNSWMaxScanTuples)
	}
	if s.IVFFlatProbes < 0 {
		return searchInvalid("ivfflat.probes %d must be at least 1", s.IVFFlatProbes)
	}
	switch s.IVFFlatIterativeScan {
	case "", IterativeOff, IterativeRelaxedOrder:
	default:
		return searchInvalid("ivfflat.iterative_scan %q is not off or relaxed_order", s.IVFFlatIterativeScan)
	}
	if s.IVFFlatMaxProbes < 0 {
		return searchInvalid("ivfflat.max_probes %d must be at least 1", s.IVFFlatMaxProbes)
	}
	return nil
}

type settingPair struct{ Name, Value string }

// searchSettingPairs returns the non-zero settings in this fixed order: enable_indexscan,
// enable_bitmapscan, hnsw.ef_search, hnsw.iterative_scan, hnsw.max_scan_tuples, ivfflat.probes,
// ivfflat.iterative_scan, ivfflat.max_probes. Integers are formatted with strconv.Itoa.
func searchSettingPairs(s SearchSettings) []settingPair {
	var out []settingPair
	if s.ExactSearch {
		out = append(out, settingPair{"enable_indexscan", "off"}, settingPair{"enable_bitmapscan", "off"})
	}
	if s.HNSWEfSearch != 0 {
		out = append(out, settingPair{"hnsw.ef_search", strconv.Itoa(s.HNSWEfSearch)})
	}
	if s.HNSWIterativeScan != "" {
		out = append(out, settingPair{"hnsw.iterative_scan", string(s.HNSWIterativeScan)})
	}
	if s.HNSWMaxScanTuples != 0 {
		out = append(out, settingPair{"hnsw.max_scan_tuples", strconv.Itoa(s.HNSWMaxScanTuples)})
	}
	if s.IVFFlatProbes != 0 {
		out = append(out, settingPair{"ivfflat.probes", strconv.Itoa(s.IVFFlatProbes)})
	}
	if s.IVFFlatIterativeScan != "" {
		out = append(out, settingPair{"ivfflat.iterative_scan", string(s.IVFFlatIterativeScan)})
	}
	if s.IVFFlatMaxProbes != 0 {
		out = append(out, settingPair{"ivfflat.max_probes", strconv.Itoa(s.IVFFlatMaxProbes)})
	}
	return out
}

// SetLocal sets a run-time parameter for the rest of this transaction only
// (set_config with is_local = true). Name and value are bound parameters, so
// the setting reverts at commit or rollback and never reaches a pooled
// connection, which makes it safe behind PgBouncer in transaction mode. It
// returns ErrUnsupported on dialects without transaction-scoped settings.
func (tx *Tx) SetLocal(ctx context.Context, name, value string) error {
	if tx.ex.d.Name != dialect.Postgres {
		return &Error{Kind: ErrUnsupported, Op: "set local", Err: errors.New("transaction-scoped settings need PostgreSQL")}
	}
	_, err := Exec(ctx, tx, "SELECT set_config(?, ?, true)", name, value)
	return err
}

// SetSearch applies s to the rest of this transaction. On SQLite, ExactSearch
// alone is a no-op and any other non-zero field returns ErrUnsupported.
func (tx *Tx) SetSearch(ctx context.Context, s SearchSettings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if tx.ex.d.Name == dialect.SQLite {
		only := s
		only.ExactSearch = false
		if only != (SearchSettings{}) {
			return &Error{Kind: ErrUnsupported, Op: "search settings", Err: errors.New("only ExactSearch is supported on SQLite")}
		}
		return nil
	}
	for _, p := range searchSettingPairs(s) {
		if err := tx.SetLocal(ctx, p.Name, p.Value); err != nil {
			return err
		}
	}
	return nil
}

// WithSearch runs fn in a transaction with s applied to it. The settings end
// with the transaction.
func (db *DB) WithSearch(ctx context.Context, s SearchSettings, fn func(*Tx) error) error {
	if err := s.Validate(); err != nil {
		return err
	}
	return db.Tx(ctx, TxOptions{}, func(tx *Tx) error {
		if err := tx.SetSearch(ctx, s); err != nil {
			return err
		}
		return fn(tx)
	})
}
