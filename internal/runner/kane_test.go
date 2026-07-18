package runner

import (
	"0945-playbook/internal/data"
	"0945-playbook/internal/watchlist"
	"testing"
	"time"
)

func TestKaneStateRanksPrimaryGapDownAheadOfGapUp(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	now := time.Date(2026, 7, 17, 9, 31, 0, 0, loc)
	bar := func(day int, hour, minute int, o, h, l, c float64) data.Bar {
		return data.Bar{Time: time.Date(2026, 7, day, hour, minute, 0, 0, loc), Open: o, High: h, Low: l, Close: c}
	}
	items := []watchlist.Item{{Symbol: "DOWN", ATR14: 8}, {Symbol: "UP", ATR14: 2}}
	bars := map[string][]data.Bar{
		"DOWN": {bar(16, 9, 30, 100, 105, 95, 100), bar(16, 15, 59, 100, 101, 99, 100), bar(17, 9, 30, 84, 85, 83, 84)},
		"UP":   {bar(16, 9, 30, 20, 21, 19, 20), bar(16, 15, 59, 20, 20, 20, 20), bar(17, 9, 30, 24, 25, 24, 24)},
	}
	got := kaneState(items, bars, now, loc, "http://localhost:8081")
	if len(got.Rows) != 2 || got.Rows[0].Symbol != "DOWN" || !got.Rows[0].Preferred || !got.Rows[1].Preferred || got.Rows[1].Rank != 1 {
		t.Fatalf("unexpected ranking: %+v", got.Rows)
	}
}

func TestKaneStateReturnsAllCandidatesAndPrefersOnePerStrategy(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	now := time.Date(2026, 7, 17, 9, 31, 0, 0, loc)
	bar := func(day int, hour, minute int, o, h, l, c float64) data.Bar {
		return data.Bar{Time: time.Date(2026, 7, day, hour, minute, 0, 0, loc), Open: o, High: h, Low: l, Close: c}
	}
	items := []watchlist.Item{{Symbol: "D1", ATR14: 8}, {Symbol: "D2", ATR14: 8}}
	bars := map[string][]data.Bar{
		"D1": {bar(16, 9, 30, 100, 105, 95, 100), bar(16, 15, 59, 100, 101, 99, 100), bar(17, 9, 30, 82, 83, 81, 82)},
		"D2": {bar(16, 9, 30, 100, 105, 95, 100), bar(16, 15, 59, 100, 101, 99, 100), bar(17, 9, 30, 85, 86, 84, 85)},
	}
	got := kaneState(items, bars, now, loc, "http://localhost:8081")
	if len(got.Rows) != 2 || got.Rows[0].Symbol != "D1" || !got.Rows[0].Preferred || got.Rows[1].Preferred || got.Rows[1].Rank != 2 {
		t.Fatalf("unexpected candidates: %+v", got.Rows)
	}
}
