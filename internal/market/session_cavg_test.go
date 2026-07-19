package market

import (
	"0945-playbook/internal/data"
	"testing"
	"time"
)

func cbar(loc *time.Location, day string, h, m int, c float64) data.Bar {
	d, _ := time.ParseInLocation("2006-01-02", day, loc)
	return data.Bar{Time: time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, loc).UTC(), Open: c, High: c, Low: c, Close: c, Volume: 100}
}
func TestCAVGExcludesPriorSessionAndRequires15CurrentValues(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	e := New([]string{"GAP"}, loc, 64, nil)
	bars := []data.Bar{cbar(loc, "2026-07-01", 9, 30, 100), cbar(loc, "2026-07-01", 15, 59, 100)}
	for i := 0; i < 13; i++ {
		bars = append(bars, cbar(loc, "2026-07-02", 4, i, 50))
	}
	e.Seed("GAP", bars)
	now := time.Date(2026, 7, 2, 4, 12, 30, 0, loc)
	e.Apply(Event{Symbol: "GAP", Start: now.Truncate(time.Minute), End: now, Open: 50, High: 50, Low: 50, Close: 50, Volume: 1, VWAP: 50, Received: now})
	s, _ := e.Snapshot("GAP", 15)
	if s.Warm || s.EligibleBarCount != 13 {
		t.Fatalf("prior bars warmed C/Avg: %+v", s)
	}
	e.Seed("GAP", []data.Bar{cbar(loc, "2026-07-02", 4, 13, 50), cbar(loc, "2026-07-02", 4, 14, 55)})
	now = time.Date(2026, 7, 2, 4, 14, 30, 0, loc)
	e.Apply(Event{Symbol: "GAP", Start: now.Truncate(time.Minute), End: now, Open: 55, High: 55, Low: 55, Close: 55, Volume: 1, VWAP: 55, Received: now})
	s, _ = e.Snapshot("GAP", 15)
	if !s.Warm || s.EligibleBarCount != 15 {
		t.Fatalf("15 current bars not warm: %+v", s)
	}
	if s.Average15 < 50 || s.Average15 > 51 {
		t.Fatalf("prior close contaminated average: %v", s.Average15)
	}
}
func TestCAVGSessionResetWeekend(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	e := New([]string{"X"}, loc, 64, nil)
	bars := make([]data.Bar, 15)
	for i := range bars {
		bars[i] = cbar(loc, "2026-07-02", 4, i, 10)
	}
	e.Seed("X", bars)
	monday := time.Date(2026, 7, 6, 4, 0, 30, 0, loc)
	e.Apply(Event{Symbol: "X", Start: monday.Truncate(time.Minute), End: monday, Open: 20, High: 20, Low: 20, Close: 20, Volume: 1, VWAP: 20, Received: monday})
	s, _ := e.Snapshot("X", 15)
	if s.Warm || s.EligibleBarCount != 1 || s.SessionDate != "2026-07-06" {
		t.Fatalf("weekend mixed sessions: %+v", s)
	}
}
