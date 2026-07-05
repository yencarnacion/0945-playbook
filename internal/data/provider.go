package data

import (
	"context"
	"math"
	"sort"
	"time"
)

type Bar struct {
	Time   time.Time `json:"time"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume"`
	VWAP   float64   `json:"vwap"`
}

type Provider interface {
	FetchBars(ctx context.Context, symbol string, start, end time.Time) ([]Bar, error)
}

func NormalizeToMinutes(bars []Bar, loc *time.Location) []Bar {
	if len(bars) == 0 {
		return nil
	}

	sort.Slice(bars, func(i, j int) bool {
		return bars[i].Time.Before(bars[j].Time)
	})

	type bucket struct {
		bar       Bar
		seen      bool
		vwapPV    float64
		vwapVol   float64
		lastStamp time.Time
	}

	buckets := make(map[time.Time]*bucket)
	keys := make([]time.Time, 0, len(bars))

	for _, b := range bars {
		if b.Close <= 0 || b.High <= 0 || b.Low <= 0 || b.Open <= 0 {
			continue
		}
		minute := b.Time.In(loc).Truncate(time.Minute)
		utcMinute := time.Date(minute.Year(), minute.Month(), minute.Day(), minute.Hour(), minute.Minute(), 0, 0, loc).UTC()
		bk := buckets[utcMinute]
		if bk == nil {
			bk = &bucket{}
			buckets[utcMinute] = bk
			keys = append(keys, utcMinute)
		}
		if !bk.seen {
			bk.bar = Bar{
				Time:  utcMinute,
				Open:  b.Open,
				High:  b.High,
				Low:   b.Low,
				Close: b.Close,
			}
			bk.seen = true
		}
		bk.bar.High = math.Max(bk.bar.High, b.High)
		bk.bar.Low = math.Min(bk.bar.Low, b.Low)
		if b.Time.After(bk.lastStamp) || bk.lastStamp.IsZero() {
			bk.bar.Close = b.Close
			bk.lastStamp = b.Time
		}
		bk.bar.Volume += b.Volume
		vwap := b.VWAP
		if vwap <= 0 {
			vwap = (b.High + b.Low + b.Close) / 3
		}
		bk.vwapPV += vwap * b.Volume
		bk.vwapVol += b.Volume
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })
	out := make([]Bar, 0, len(keys))
	for _, k := range keys {
		bk := buckets[k]
		if bk == nil || !bk.seen {
			continue
		}
		if bk.vwapVol > 0 {
			bk.bar.VWAP = bk.vwapPV / bk.vwapVol
		}
		out = append(out, bk.bar)
	}
	return out
}
