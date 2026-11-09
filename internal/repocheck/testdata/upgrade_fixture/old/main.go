// SPDX-License-Identifier: Apache-2.0

// Command old is the upgrade fixture: a program written against the root
// package before the 0.4.0 split, using every moved identifier through its old
// name. It must NOT compile against the current module. The upgrade fixture
// tests in internal/repocheck check that it fails with an undefined error for
// each moved name, rewrite it with the recipe of docs/upgrading.md and compile
// the result.
//
// Conventions the tests rely on: a variable named tx is a *momo.Tx and a
// variable named db is a *momo.DB, so a method call is found by its receiver.
package main

import (
	"context"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/types"
	"github.com/azrtydxb/momo/vector"
)

type item struct {
	Label string `json:"label"`
}

func main() {}

func arrays() {
	tags := momo.NewColumn[[]string]("docs", "tags", 0)
	title := momo.NewColumn[string]("docs", "title", 0)
	_ = momo.ArrayContains(tags, "go", "sql")
	_ = momo.ArrayContainedBy(tags, "go", "sql")
	_ = momo.ArrayOverlaps(tags, "go", "sql")
	_ = momo.AnyOf(title, "a", "b", "c")
}

func fullText() {
	body := momo.NewColumn[types.TSVector]("articles", "search_tsv", 0)
	text := momo.NewColumn[string]("articles", "body", 0)
	var vec momo.TSExpr[types.TSVector] = momo.ToTSVector("english", text)
	q := momo.WebsearchToTSQuery("english", "red fox")
	_ = momo.PlainToTSQuery("english", "fox")
	_ = momo.PhraseToTSQuery("english", "red fox")
	_ = momo.ToTSQuery("english", "fox")
	_ = momo.TSMatch(body, q)
	_ = momo.TSMatch(vec, q)
	var rank momo.TSExpr[float64] = momo.TSRank(body, q, 1)
	_ = rank
	_ = momo.TSRankCD(body, q, 1)
	_ = momo.TSHeadline("english", text, q, "StartSel=<b>")
}

func scalars() {
	title := momo.NewColumn[string]("docs", "title", 0)
	hits := momo.NewColumn[int64]("docs", "hits", 0)
	_ = momo.Substr(title, 1, 3)
	_ = momo.SubstrFrom(title, 2)
	_ = momo.Lower(title)
	_ = momo.Upper(title)
	var length momo.LengthExpr = momo.Length(title)
	_ = length
	var schema momo.CurrentSchemaExpr = momo.CurrentSchema()
	_ = schema
	var size momo.CaseExpr[string] = momo.Case[string]().When(hits.Gt(100), "big").Else("small")
	_ = size
	_ = momo.Add(hits, 5)
	_ = momo.AddExpr(hits, hits)
	_ = momo.Sub(hits, 5)
	_ = momo.SubExpr(hits, hits)
	_ = momo.Mul(hits, 5)
	_ = momo.MulExpr(hits, hits)
	_ = twice[int64](hits)
	_ = momo.CoalesceExpr[string](title, title)
	var cast momo.CastExpr[float64] = momo.Cast[float64](hits, "numeric(10,2)")
	_ = cast
	var joined momo.ConcatExpr = momo.Concat(title, title)
	_ = joined
}

func twice[T momo.Numeric](e momo.Expression[T]) momo.ArithExpr[T] {
	return momo.AddExpr(e, e)
}

func jsonOperators() {
	after := momo.NewColumn[types.JSON[map[string]any]]("events", "after", 0)
	var via momo.JSONTextExpr = momo.JSONText(after, "via")
	_ = via
	_ = momo.JSONPath(after, "items", "0", "sku")
	_ = momo.JSONContains(after, map[string]any{"via": "api"})
	_ = momo.JSONHasKey(after, "via")
}

func aggregates() {
	title := momo.NewColumn[string]("docs", "title", 0)
	var joined momo.AggExpr[string] = momo.StringAgg(title, ", ")
	_ = joined
	_ = momo.Aggregate[int64]("max", title)
	_ = momo.ArrayAgg[string](title)
	var pair momo.JSONPair = momo.JSONField("label", title)
	var obj momo.JSONObjectExpr[item] = momo.JSONBBuildObject[item](pair)
	_ = momo.JSONBAgg[item](obj)
	_ = momo.PercentileCont(0.5)
	_ = momo.PercentileDisc[float64](0.5)
	var asJSON momo.ToJSONExpr[item] = momo.ToJSONB[item](title)
	_ = asJSON
	_ = momo.EmptyJSONArray[item]()
}

func timeExpressions() {
	created := momo.NewColumn[time.Time]("docs", "created_at", 0)
	var field momo.ExtractField = momo.ExtractYear
	_ = field
	_ = momo.ExtractMonth
	_ = momo.ExtractDay
	_ = momo.ExtractHour
	_ = momo.ExtractMinute
	_ = momo.ExtractSecond
	_ = momo.ExtractQuarter
	_ = momo.ExtractDow
	_ = momo.ExtractISODow
	_ = momo.ExtractDoy
	_ = momo.ExtractEpoch
	var extracted momo.ExtractExpr = momo.Extract(momo.ExtractDay, created)
	_ = extracted
	var zoned momo.TimeExpr = momo.AtTimeZone(created, "Europe/Brussels")
	_ = zoned
	_ = momo.AtTimeZoneExpr(created, momo.Value("UTC"))
	var unit momo.IntervalUnit = momo.IntervalSecond
	_ = unit
	_ = momo.IntervalMinute
	_ = momo.IntervalHour
	_ = momo.IntervalDay
	_ = momo.IntervalWeek
	_ = momo.IntervalMonth
	_ = momo.IntervalYear
	var week momo.Interval = momo.IntervalOf(1, momo.IntervalWeek)
	_ = momo.IntervalExpr(momo.Value(int64(2)), momo.IntervalDay)
	_ = momo.AddInterval(created, week)
	_ = momo.SubInterval(momo.Now(), week)
	_ = momo.CurrentTimestamp()
	var trunc momo.TruncUnit = momo.TruncSecond
	_ = trunc
	_ = momo.TruncMinute
	_ = momo.TruncHour
	_ = momo.TruncDay
	_ = momo.TruncWeek
	_ = momo.TruncMonth
	_ = momo.TruncQuarter
	_ = momo.TruncYear
	_ = momo.DateTrunc(momo.TruncDay, created)
}

func locks(ctx context.Context, db *momo.DB, tx *momo.Tx) error {
	if err := tx.AdvisoryXactLock(ctx, 42); err != nil {
		return err
	}
	if err := tx.AdvisoryXactLockString(ctx, "catalog:7"); err != nil {
		return err
	}
	if _, err := tx.TryAdvisoryXactLock(ctx, 42); err != nil {
		return err
	}
	if _, err := tx.TryAdvisoryXactLockString(ctx, "catalog:7"); err != nil {
		return err
	}
	if err := db.AdvisoryXactLock(ctx, 42); err != nil {
		return err
	}
	if err := db.AdvisoryXactLockString(ctx, "catalog:7"); err != nil {
		return err
	}
	if _, err := db.TryAdvisoryXactLock(ctx, 42); err != nil {
		return err
	}
	_, err := db.TryAdvisoryXactLockString(ctx, "catalog:7")
	return err
}

func search(ctx context.Context, db *momo.DB, tx *momo.Tx) error {
	var settings momo.SearchSettings
	settings.HNSWEfSearch = 100
	settings.HNSWIterativeScan = momo.IterativeRelaxedOrder
	var scan momo.IterativeScan = momo.IterativeOff
	_ = scan
	_ = momo.IterativeStrictOrder
	if err := settings.Validate(); err != nil {
		return err
	}
	if err := tx.SetSearch(ctx, settings); err != nil {
		return err
	}
	return db.WithSearch(ctx, settings, func(inner *momo.Tx) error { return nil })
}

func distances() {
	emb := momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	fingerprint := momo.NewVectorColumn[vector.Bits]("docs", "fp", 8, 0)
	query := vector.Vector{1, 0, 0}
	bits := vector.Bits{Len: 8, Data: []byte{0xff}}
	var dist momo.DistanceExpr = momo.L2Distance(emb, query)
	_ = dist
	_ = momo.InnerProductDistance(emb, query)
	_ = momo.CosineDistance(emb, query)
	_ = momo.L1Distance(emb, query)
	_ = momo.HammingDistance(fingerprint, bits)
	_ = momo.JaccardDistance(fingerprint, bits)
}
