package playbook

import (
	"testing"
	"time"

	"0945-playbook/internal/config"
	"0945-playbook/internal/data"
	"0945-playbook/internal/watchlist"
)

func TestEvaluateExtremeUpperFadeSignal(t *testing.T) {
	loc := mustNY(t)
	set := SettingsFromConfig(config.Defaults())
	item := watchlist.Item{Symbol: "TEST", Order: 0}
	bars := first15Bars(loc, 10, 10.2, 9.8, 50000)
	bars = append(bars, bar(loc, 9, 45, 10.0, 10.65, 10.55, 10.60, 80000))

	ev := Evaluate(item, bars, at(loc, 9, 45), loc, set, nil)

	if !ev.Signal {
		t.Fatalf("expected signal, got status=%s reason=%s", ev.Status, ev.Reason)
	}
	if ev.Branch != "B2 EXT FADE" {
		t.Fatalf("branch = %q, want B2 EXT FADE", ev.Branch)
	}
	if ev.Side != -1 || ev.Action != "SHORT/HOLD" {
		t.Fatalf("side/action = %d/%q, want active short hold", ev.Side, ev.Action)
	}
	if ev.Shares <= 0 {
		t.Fatalf("shares = %d, want positive", ev.Shares)
	}
	if ev.Ratio < 1.05 {
		t.Fatalf("ratio = %.4f, want extreme upper", ev.Ratio)
	}
}

func TestEvaluateLikelyUpperBefore0945(t *testing.T) {
	loc := mustNY(t)
	set := SettingsFromConfig(config.Defaults())
	item := watchlist.Item{Symbol: "TEST", Order: 0}
	bars := first15Bars(loc, 10, 10.1, 9.9, 50000)[:11]
	bars = append(bars, bar(loc, 9, 41, 10.0, 10.35, 10.0, 10.30, 50000))

	ev := Evaluate(item, bars, at(loc, 9, 41), loc, set, nil)

	if !ev.Candidate {
		t.Fatalf("expected likely candidate, got status=%s ratio=%.4f", ev.Status, ev.Ratio)
	}
	if ev.Status != "LIKELY SELL" {
		t.Fatalf("status = %q, want LIKELY SELL", ev.Status)
	}
	if ev.Signal {
		t.Fatalf("did not expect actual signal before 09:45")
	}
	if ev.Branch == "-" || ev.Entry == 0 || ev.Target == 0 || ev.Stop == 0 || ev.Shares == 0 {
		t.Fatalf("expected estimated trade details, got branch=%q entry=%.2f target=%.2f stop=%.2f shares=%d", ev.Branch, ev.Entry, ev.Target, ev.Stop, ev.Shares)
	}
	if ev.ORBSource != "live 15m estimate" {
		t.Fatalf("ORB source = %q, want live 15m estimate", ev.ORBSource)
	}
}

func TestEvaluateReplayChartURLIncludesDateTimeAndSignalSide(t *testing.T) {
	loc := mustNY(t)
	set := SettingsFromConfig(config.Defaults())
	set.ChartDate = "2026-07-02"
	set.ChartTime = "0945"
	item := watchlist.Item{Symbol: "AVAV", Order: 0}
	bars := first15Bars(loc, 10, 10.2, 9.8, 50000)
	bars = append(bars, bar(loc, 9, 45, 10.0, 10.65, 10.55, 10.60, 80000))

	ev := Evaluate(item, bars, at(loc, 9, 45), loc, set, nil)

	want := "http://localhost:8081/api/open-chart/AVAV/2026-07-02/0945?resolution=1m&signal=sell"
	if ev.ChartURL != want {
		t.Fatalf("chart URL = %q, want %q", ev.ChartURL, want)
	}
}

func first15Bars(loc *time.Location, close float64, high float64, low float64, volume float64) []data.Bar {
	out := make([]data.Bar, 0, 15)
	for i := 0; i < 15; i++ {
		t := time.Date(2026, 7, 2, 9, 30+i, 0, 0, loc)
		out = append(out, data.Bar{
			Time:   t.UTC(),
			Open:   close,
			High:   high,
			Low:    low,
			Close:  close,
			Volume: volume,
			VWAP:   close,
		})
	}
	return out
}

func bar(loc *time.Location, hour int, minute int, open float64, high float64, low float64, close float64, volume float64) data.Bar {
	t := time.Date(2026, 7, 2, hour, minute, 0, 0, loc)
	return data.Bar{
		Time:   t.UTC(),
		Open:   open,
		High:   high,
		Low:    low,
		Close:  close,
		Volume: volume,
		VWAP:   (high + low + close) / 3,
	}
}

func at(loc *time.Location, hour int, minute int) time.Time {
	return time.Date(2026, 7, 2, hour, minute, 0, 0, loc)
}

func mustNY(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
