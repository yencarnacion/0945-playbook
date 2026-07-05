package runner

import (
	"testing"
	"time"

	"0945-playbook/internal/data"
)

func TestLiveFetchStartBackfillsFromOpenWhenCacheEmpty(t *testing.T) {
	loc := mustNY(t)
	now := time.Date(2026, 7, 2, 10, 15, 0, 0, loc)

	got := liveFetchStart(now, loc, "09:30", nil)
	want := time.Date(2026, 7, 2, 9, 30, 0, 0, loc)

	if !got.Equal(want) {
		t.Fatalf("fetch start = %s, want %s", got, want)
	}
}

func TestLiveFetchStartRefetchesPreviousCachedMinute(t *testing.T) {
	loc := mustNY(t)
	now := time.Date(2026, 7, 2, 10, 15, 0, 0, loc)
	cached := []data.Bar{
		liveBar(loc, 9, 30, 10),
		liveBar(loc, 9, 45, 11),
	}

	got := liveFetchStart(now, loc, "09:30", cached)
	want := time.Date(2026, 7, 2, 9, 44, 0, 0, loc)

	if !got.Equal(want) {
		t.Fatalf("fetch start = %s, want %s", got, want)
	}
}

func TestMergeRTHBarsFiltersPremarketAndReplacesOverlappingMinute(t *testing.T) {
	loc := mustNY(t)
	open := time.Date(2026, 7, 2, 9, 30, 0, 0, loc)
	closeTime := time.Date(2026, 7, 2, 16, 0, 0, 0, loc)
	existing := []data.Bar{
		liveBar(loc, 9, 30, 10),
		liveBar(loc, 9, 31, 11),
	}
	incoming := []data.Bar{
		liveBar(loc, 9, 0, 99),
		liveBar(loc, 9, 31, 12),
		liveBar(loc, 9, 32, 13),
	}

	got := mergeRTHBars(existing, incoming, open, closeTime, loc)

	if len(got) != 3 {
		t.Fatalf("merged bars = %d, want 3", len(got))
	}
	for i, wantClose := range []float64{10, 12, 13} {
		if got[i].Close != wantClose {
			t.Fatalf("bar %d close = %.2f, want %.2f", i, got[i].Close, wantClose)
		}
	}
}

func liveBar(loc *time.Location, hour int, minute int, close float64) data.Bar {
	t := time.Date(2026, 7, 2, hour, minute, 0, 0, loc)
	return data.Bar{
		Time:   t.UTC(),
		Open:   close,
		High:   close + 0.1,
		Low:    close - 0.1,
		Close:  close,
		Volume: 100,
		VWAP:   close,
	}
}

func mustNY(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
