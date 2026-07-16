package runner

import (
	"context"
	"testing"
	"time"

	"0945-playbook/internal/config"
	"0945-playbook/internal/data"
	"0945-playbook/internal/watchlist"
)

func TestExtendedScanKeepsEveryMinuteAndFindsRollingRatio(t *testing.T) {
	loc := mustNY(t)
	cfg := config.Defaults()
	item := watchlist.Item{Symbol: "TEST", Name: "Test Corp", Order: 1}
	started := time.Date(2026, 7, 2, 4, 0, 0, 0, loc)
	r := NewLive("test", cfg, loc, []watchlist.Item{item}, nil)
	r.extendedStarted = started
	r.barsBySymbol[item.Symbol] = extendedBars(loc, started, 16, func(i int) float64 {
		if i == 15 {
			return 10.30
		}
		return 10
	})

	r.mu.Lock()
	r.recordExtendedSnapshots(time.Date(2026, 7, 2, 4, 15, 30, 0, loc), started)
	r.mu.Unlock()

	got := r.ExtendedSnapshot(context.Background(), 0)
	if len(got.History) != 16 {
		t.Fatalf("history minutes = %d, want 16", len(got.History))
	}
	if got.Selected.Clock != "04:15" || len(got.Selected.Rows) != 1 {
		t.Fatalf("latest snapshot = %s with %d rows, want 04:15 with one row", got.Selected.Clock, len(got.Selected.Rows))
	}
	row := got.Selected.Rows[0]
	if row.Symbol != "TEST" || row.Ratio <= cfg.Extended.UpperSignalRatio {
		t.Fatalf("row = %+v, want TEST above %.2f", row, cfg.Extended.UpperSignalRatio)
	}
	if got.History[14].Count != 0 || got.History[15].Count != 1 {
		t.Fatalf("history counts at 04:14/04:15 = %d/%d, want 0/1", got.History[14].Count, got.History[15].Count)
	}
}

func TestExtendedScanStartsItsAverageAtProcessStart(t *testing.T) {
	loc := mustNY(t)
	cfg := config.Defaults()
	item := watchlist.Item{Symbol: "TEST", Order: 1}
	started := time.Date(2026, 7, 2, 10, 0, 0, 0, loc)
	r := NewLive("test", cfg, loc, []watchlist.Item{item}, nil)
	r.extendedStarted = started
	prestart := extendedBars(loc, time.Date(2026, 7, 2, 9, 30, 0, 0, loc), 30, func(int) float64 { return 1 })
	afterStart := extendedBars(loc, started, 15, func(i int) float64 {
		if i == 14 {
			return 10.30
		}
		return 10
	})
	r.barsBySymbol[item.Symbol] = append(prestart, afterStart...)

	r.mu.Lock()
	r.recordExtendedSnapshots(time.Date(2026, 7, 2, 10, 14, 30, 0, loc), started)
	r.mu.Unlock()

	got := r.ExtendedSnapshot(context.Background(), 0)
	if len(got.History) != 15 || len(got.Selected.Rows) != 1 {
		t.Fatalf("history/rows = %d/%d, want 15/1", len(got.History), len(got.Selected.Rows))
	}
	if got.Selected.Rows[0].Average < 10 || got.Selected.Rows[0].Average > 10.03 {
		t.Fatalf("average = %.4f, pre-start bars appear to have leaked in", got.Selected.Rows[0].Average)
	}
}

func extendedBars(loc *time.Location, start time.Time, count int, closeAt func(int) float64) []data.Bar {
	bars := make([]data.Bar, 0, count)
	for i := 0; i < count; i++ {
		close := closeAt(i)
		bars = append(bars, data.Bar{
			Time: start.Add(time.Duration(i) * time.Minute).UTC(), Open: close, High: close, Low: close, Close: close, Volume: 100,
		})
	}
	return bars
}
