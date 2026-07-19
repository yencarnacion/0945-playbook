package runner

import (
	"math"
	"testing"
	"time"

	"0945-playbook/internal/config"
	"0945-playbook/internal/data"
	"0945-playbook/internal/market"
	"0945-playbook/internal/playbook"
	"0945-playbook/internal/watchlist"
)

func TestStreamingStatePlaybookAndCAVGParity(t *testing.T) {
	loc := mustNY(t)
	item := watchlist.Item{Symbol: "TEST", Name: "Test", ATR14: 2}
	cfg := config.Defaults()
	set := playbook.SettingsFromConfig(cfg)
	day := time.Date(2026, 7, 2, 0, 0, 0, 0, loc)
	set.ChartDate = day.Format("2006-01-02")
	bars := make([]data.Bar, 0, 16)
	for i := 0; i < 16; i++ {
		c := 10 + float64(i)*.03
		bars = append(bars, data.Bar{Time: time.Date(2026, 7, 2, 9, 30+i, 0, 0, loc).UTC(), Open: c, High: c + .1, Low: c - .1, Close: c, Volume: 50000, VWAP: c})
	}
	at := time.Date(2026, 7, 2, 9, 45, 30, 0, loc)
	legacy := playbook.Evaluate(item, append([]data.Bar(nil), bars...), at, loc, set, nil)
	engine := market.New([]string{item.Symbol}, loc, 64, nil)
	engine.Seed(item.Symbol, bars)
	snap, _ := engine.Snapshot(item.Symbol, 15)
	streamed := playbook.Evaluate(item, snap.Bars, at, loc, set, nil)
	if legacy.Ratio != streamed.Ratio || legacy.Avg15 != streamed.Avg15 || legacy.Signal != streamed.Signal || legacy.Branch != streamed.Branch || legacy.Entry != streamed.Entry {
		t.Fatalf("parity failed\nlegacy=%+v\nstream=%+v", legacy, streamed)
	}
	wantSum := 0.0
	for _, b := range bars[len(bars)-15:] {
		wantSum += b.Close
	}
	if math.Abs(streamed.Avg15-wantSum/15) > 1e-12 {
		t.Fatalf("developing-minute avg=%v want %v", streamed.Avg15, wantSum/15)
	}
}

func TestStreamingStateKaneCutoffParity(t *testing.T) {
	loc := mustNY(t)
	item := watchlist.Item{Symbol: "DOWN", ATR14: 8}
	bars := []data.Bar{{Time: time.Date(2026, 7, 16, 9, 30, 0, 0, loc), Open: 100, High: 105, Low: 95, Close: 100}, {Time: time.Date(2026, 7, 16, 15, 59, 0, 0, loc), Open: 100, High: 101, Low: 99, Close: 100}, {Time: time.Date(2026, 7, 17, 9, 30, 0, 0, loc), Open: 84, High: 85, Low: 83, Close: 84}}
	at := time.Date(2026, 7, 17, 9, 31, 0, 0, loc)
	legacy := kaneState([]watchlist.Item{item}, map[string][]data.Bar{"DOWN": bars}, at, loc, "")
	engine := market.New([]string{"DOWN"}, loc, 64, nil)
	engine.Seed("DOWN", bars)
	snap, _ := engine.Snapshot("DOWN", 15)
	streamed := kaneState([]watchlist.Item{item}, map[string][]data.Bar{"DOWN": snap.Bars}, at, loc, "")
	if len(legacy.Rows) != len(streamed.Rows) || len(legacy.Rows) == 0 || legacy.Rows[0].GapPct != streamed.Rows[0].GapPct || len(legacy.History) != len(streamed.History) {
		t.Fatalf("kane parity legacy=%+v stream=%+v", legacy, streamed)
	}
}
