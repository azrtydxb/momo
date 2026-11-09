// SPDX-License-Identifier: Apache-2.0

// Package vecsearch applies pgvector search settings (hnsw and ivfflat) to one
// transaction. Apply sets them on a transaction you already have; With opens
// one and runs a callback inside it. The settings end with the transaction.
package vecsearch

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo"
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

// Upper bounds of the pgvector settings, as enforced by pgvector itself.
const (
	maxHNSWEfSearch      = 1000
	maxHNSWMaxScanTuples = 2147483647 // INT_MAX
	maxIVFFlatProbes     = 32768

	minHNSWScanMemMultiplier = 1
	maxHNSWScanMemMultiplier = 1000
)

// Settings applies pgvector session settings for one transaction only.
// Zero values mean "do not change the server default".
//
// HNSWIterativeScan, HNSWMaxScanTuples, HNSWScanMemMultiplier, IVFFlatIterativeScan and
// IVFFlatMaxProbes need pgvector 0.8.0 or newer. Apply checks the
// installed extension version when any of them is set and returns
// momo.ErrUnsupported on an older one, because an older server may accept the unknown setting as a
// placeholder or fail with a generic error instead of a typed one.
type Settings struct {
	// ExactSearch disables index scans and bitmap scans for the rest of the
	// transaction (enable_indexscan = off, enable_bitmapscan = off) so vector
	// ORDER BY queries scan the table and return exact nearest neighbours.
	// The switches are not specific to vector indexes: primary-key, join and
	// every other index lookup in the same transaction also fall back to
	// sequential scans. Keep the transaction short and dedicated to the exact
	// query.
	ExactSearch       bool
	HNSWEfSearch      int           // hnsw.ef_search, 1..1000
	HNSWIterativeScan IterativeScan // hnsw.iterative_scan (pgvector >= 0.8)
	HNSWMaxScanTuples int           // hnsw.max_scan_tuples, 1..2147483647 (pgvector >= 0.8; server default 20000)
	// HNSWScanMemMultiplier sets hnsw.scan_mem_multiplier: the memory an
	// iterative HNSW scan may use, as a multiple of work_mem. The unit is a
	// plain multiplier (not bytes), the accepted range is 1 to 1000 and zero
	// leaves the server default alone. It needs pgvector 0.8.0 or newer;
	// Apply returns momo.ErrUnsupported on an older extension.
	HNSWScanMemMultiplier float64
	IVFFlatProbes         int           // ivfflat.probes, 1..32768
	IVFFlatIterativeScan  IterativeScan // ivfflat.iterative_scan: off or relaxed_order only (pgvector >= 0.8)
	IVFFlatMaxProbes      int           // ivfflat.max_probes, 1..32768 (pgvector >= 0.8)
}

// hnsw.scan_mem_multiplier arrived with the iterative scans in pgvector 0.8.0
// (CHANGELOG "0.8.0: Added support for iterative index scans"; src/hnsw.c at
// tag v0.8.0 defines it with range 1..1000, and v0.7.4 does not), so the same
// 0.8.0 gate covers it.

// usesIterativeGUCs reports whether s sets a parameter that exists only in
// pgvector 0.8.0 and newer.
func (s Settings) usesIterativeGUCs() bool {
	return s.HNSWIterativeScan != "" || s.HNSWMaxScanTuples != 0 || s.HNSWScanMemMultiplier != 0 ||
		s.IVFFlatIterativeScan != "" || s.IVFFlatMaxProbes != 0
}

// pgvectorAtLeast08 reports whether an extension version string such as
// "0.8.1" is 0.8.0 or newer.
func pgvectorAtLeast08(version string) (bool, error) {
	parts := strings.SplitN(strings.TrimSpace(version), ".", 3)
	if len(parts) < 2 {
		return false, fmt.Errorf("unrecognized pgvector version %q", version)
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false, fmt.Errorf("unrecognized pgvector version %q", version)
	}
	return major > 0 || minor >= 8, nil
}

// requireIterativeSupport returns momo.ErrUnsupported when the installed pgvector
// is older than 0.8.0 or missing.
func requireIterativeSupport(ctx context.Context, tx *momo.Tx) error {
	vs, err := momo.Scalars[string](ctx, tx, "SELECT extversion FROM pg_extension WHERE extname = 'vector'")
	if err != nil {
		return err
	}
	if len(vs) == 0 {
		return &momo.Error{Kind: momo.ErrUnsupported, Op: "search settings", Err: errors.New("the pgvector extension is not installed")}
	}
	ok, err := pgvectorAtLeast08(vs[0])
	if err != nil {
		return &momo.Error{Kind: momo.ErrUnsupported, Op: "search settings", Err: err}
	}
	if !ok {
		return &momo.Error{Kind: momo.ErrUnsupported, Op: "search settings", Err: fmt.Errorf("iterative scan, max_scan_tuples, scan_mem_multiplier and max_probes need pgvector 0.8.0 or newer, found %s (after upgrading the package, run ALTER EXTENSION vector UPDATE)", vs[0])}
	}
	return nil
}

func searchInvalid(format string, args ...any) error {
	return &momo.Error{Kind: momo.ErrValidation, Op: "search settings", Err: fmt.Errorf(format, args...)}
}

// Validate reports a setting outside its pgvector range. The error wraps
// momo.ErrValidation.
func (s Settings) Validate() error {
	if s.HNSWEfSearch < 0 || s.HNSWEfSearch > 1000 {
		return searchInvalid("hnsw.ef_search %d is outside 1..1000", s.HNSWEfSearch)
	}
	switch s.HNSWIterativeScan {
	case "", IterativeOff, IterativeRelaxedOrder, IterativeStrictOrder:
	default:
		return searchInvalid("hnsw.iterative_scan %q is not off, relaxed_order or strict_order", s.HNSWIterativeScan)
	}
	if s.HNSWMaxScanTuples < 0 || s.HNSWMaxScanTuples > maxHNSWMaxScanTuples {
		return searchInvalid("hnsw.max_scan_tuples %d is outside 1..%d", s.HNSWMaxScanTuples, maxHNSWMaxScanTuples)
	}
	if m := s.HNSWScanMemMultiplier; m != 0 && !(m >= minHNSWScanMemMultiplier && m <= maxHNSWScanMemMultiplier) {
		return searchInvalid("hnsw.scan_mem_multiplier %v is outside %d..%d", m, minHNSWScanMemMultiplier, maxHNSWScanMemMultiplier)
	}
	if s.IVFFlatProbes < 0 || s.IVFFlatProbes > maxIVFFlatProbes {
		return searchInvalid("ivfflat.probes %d is outside 1..%d", s.IVFFlatProbes, maxIVFFlatProbes)
	}
	switch s.IVFFlatIterativeScan {
	case "", IterativeOff, IterativeRelaxedOrder:
	default:
		return searchInvalid("ivfflat.iterative_scan %q is not off or relaxed_order", s.IVFFlatIterativeScan)
	}
	if s.IVFFlatMaxProbes < 0 || s.IVFFlatMaxProbes > maxIVFFlatProbes {
		return searchInvalid("ivfflat.max_probes %d is outside 1..%d", s.IVFFlatMaxProbes, maxIVFFlatProbes)
	}
	return nil
}

type settingPair struct{ Name, Value string }

// searchSettingPairs returns the non-zero settings in this fixed order: enable_indexscan,
// enable_bitmapscan, hnsw.ef_search, hnsw.iterative_scan, hnsw.max_scan_tuples,
// hnsw.scan_mem_multiplier, ivfflat.probes, ivfflat.iterative_scan, ivfflat.max_probes. Integers are
// formatted with strconv.Itoa and the multiplier without exponent notation.
func searchSettingPairs(s Settings) []settingPair {
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
	if s.HNSWScanMemMultiplier != 0 {
		out = append(out, settingPair{"hnsw.scan_mem_multiplier", strconv.FormatFloat(s.HNSWScanMemMultiplier, 'f', -1, 64)})
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

// Apply applies s to the rest of tx. Settings that need pgvector 0.8 return
// momo.ErrUnsupported on an older extension.
//
// Dialect behavior: on SQLite, ExactSearch alone is a no-op (SQLite has no
// approximate vector index to bypass, so every search is already exact) and
// any other non-zero field returns momo.ErrUnsupported. On MySQL,
// Tx.SetLocal is unsupported, so any non-zero setting including ExactSearch
// alone returns momo.ErrUnsupported; there is no transaction-scoped switch to
// apply.
func Apply(ctx context.Context, tx *momo.Tx, s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	name := tx.Dialect().Name
	if name == dialect.SQLite {
		only := s
		only.ExactSearch = false
		if only != (Settings{}) {
			return &momo.Error{Kind: momo.ErrUnsupported, Op: "search settings", Err: errors.New("only ExactSearch is supported on SQLite")}
		}
		return nil
	}
	if s.usesIterativeGUCs() && name == dialect.Postgres {
		if err := requireIterativeSupport(ctx, tx); err != nil {
			return err
		}
	}
	for _, p := range searchSettingPairs(s) {
		if err := tx.SetLocal(ctx, p.Name, p.Value); err != nil {
			return err
		}
	}
	return nil
}

// With runs fn in a transaction on db with s applied to it. The settings end
// with the transaction.
func With(ctx context.Context, db *momo.DB, s Settings, fn func(*momo.Tx) error) error {
	if err := s.Validate(); err != nil {
		return err
	}
	return db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if err := Apply(ctx, tx, s); err != nil {
			return err
		}
		return fn(tx)
	})
}
