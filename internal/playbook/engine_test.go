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

func TestEvaluateKeepsGeneratingSignalsAfter0946(t *testing.T) {
	loc := mustNY(t)
	set := SettingsFromConfig(config.Defaults())
	set.ChartDate = "2026-07-02"
	item := watchlist.Item{Symbol: "TEST", Order: 0}
	bars := first15Bars(loc, 10, 10.2, 9.8, 50000)
	bars = append(bars,
		bar(loc, 9, 45, 10.0, 10.40, 10.00, 10.10, 10000),
		bar(loc, 9, 46, 10.1, 10.45, 10.05, 10.15, 10000),
		bar(loc, 9, 47, 10.15, 10.70, 10.25, 10.30, 10000),
	)

	evAt946 := Evaluate(item, bars, at(loc, 9, 46), loc, set, nil)
	if evAt946.Signal {
		t.Fatalf("did not expect signal at 09:46, got branch=%q ratio=%.4f", evAt946.Branch, evAt946.Ratio)
	}

	ev := Evaluate(item, bars, at(loc, 9, 47), loc, set, nil)

	if !ev.Signal {
		t.Fatalf("expected signal after 09:46, got status=%s reason=%s ratio=%.4f", ev.Status, ev.Reason, ev.Ratio)
	}
	if ev.Branch != "B1 VWAP FADE" {
		t.Fatalf("branch = %q, want B1 VWAP FADE", ev.Branch)
	}
	if ev.Entry != 10.30 {
		t.Fatalf("entry = %.2f, want 10.30", ev.Entry)
	}
	if ev.Avg15 < 10.036 || ev.Avg15 > 10.037 {
		t.Fatalf("avg15 = %.4f, want rolling 15-bar average around 10.0367", ev.Avg15)
	}
	if ev.Ratio < 1.026 || ev.Ratio > 1.027 {
		t.Fatalf("ratio = %.4f, want close/rolling avg15 around 1.0263", ev.Ratio)
	}
	if ev.DayHigh != 10.70 {
		t.Fatalf("day high = %.2f, want updated HOD 10.70", ev.DayHigh)
	}
	if ev.VWAP <= 0 || ev.VWAP >= ev.Entry {
		t.Fatalf("vwap = %.4f, want updated VWAP below entry %.2f", ev.VWAP, ev.Entry)
	}
	if ev.HODRiskPct < set.ModUpperMinHODRiskPct {
		t.Fatalf("HOD risk = %.4f, want at least %.4f", ev.HODRiskPct, set.ModUpperMinHODRiskPct)
	}

	wantURL := "http://localhost:8081/api/open-chart/TEST/2026-07-02/0947?resolution=1m&signal=sell"
	if ev.ChartURL != wantURL {
		t.Fatalf("chart URL = %q, want %q", ev.ChartURL, wantURL)
	}
}

func TestEvaluateLikelyUpperBefore0945(t *testing.T) {
	loc := mustNY(t)
	set := SettingsFromConfig(config.Defaults())
	set.ChartDate = "2026-07-02"
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
	wantURL := "http://localhost:8081/api/open-chart/TEST/2026-07-02/0941?resolution=1m&signal=sell"
	if ev.ChartURL != wantURL {
		t.Fatalf("chart URL = %q, want %q", ev.ChartURL, wantURL)
	}
}

func TestEvaluateRTHIndicatorsIgnorePremarket(t *testing.T) {
	loc := mustNY(t)
	set := SettingsFromConfig(config.Defaults())
	item := watchlist.Item{Symbol: "TEST", Order: 0}
	bars := []data.Bar{
		{
			Time:   time.Date(2026, 7, 2, 9, 0, 0, 0, loc).UTC(),
			Open:   90,
			High:   100,
			Low:    1,
			Close:  80,
			Volume: 10000,
			VWAP:   75,
		},
		{
			Time:   time.Date(2026, 7, 2, 9, 30, 0, 0, loc).UTC(),
			Open:   10,
			High:   11,
			Low:    9,
			Close:  10,
			Volume: 100,
			VWAP:   10,
		},
		{
			Time:   time.Date(2026, 7, 2, 9, 31, 0, 0, loc).UTC(),
			Open:   10,
			High:   12,
			Low:    8,
			Close:  11,
			Volume: 100,
			VWAP:   11,
		},
	}

	ev := Evaluate(item, bars, at(loc, 9, 31), loc, set, nil)

	if ev.DayHigh != 12 {
		t.Fatalf("day high = %.2f, want RTH HOD 12.00", ev.DayHigh)
	}
	if ev.DayLow != 8 {
		t.Fatalf("day low = %.2f, want RTH LOD 8.00", ev.DayLow)
	}
	if ev.VWAP != 10.5 {
		t.Fatalf("vwap = %.2f, want RTH VWAP 10.50", ev.VWAP)
	}
	if ev.Avg15 != 10.5 {
		t.Fatalf("avg15 = %.2f, want RTH partial avg close 10.50", ev.Avg15)
	}
	if ev.First15Bars != 2 {
		t.Fatalf("first15 bars = %d, want 2 RTH bars", ev.First15Bars)
	}
}

func TestEvaluateUsesRollingAvg15ForLateDayRatio(t *testing.T) {
	loc := mustNY(t)
	set := SettingsFromConfig(config.Defaults())
	item := watchlist.Item{Symbol: "TEST", Order: 0}
	bars := first15Bars(loc, 10, 10.2, 9.8, 50000)

	for i := 0; i < 15; i++ {
		bars = append(bars, bar(loc, 15, 21+i, 11, 11.1, 10.9, 11, 10000))
	}

	ev := Evaluate(item, bars, at(loc, 15, 35), loc, set, nil)

	if ev.Avg15 != 11 {
		t.Fatalf("avg15 = %.4f, want rolling late-day average 11.0000", ev.Avg15)
	}
	if ev.Ratio != 1 {
		t.Fatalf("ratio = %.4f, want close/rolling avg15 to be 1.0000", ev.Ratio)
	}
}

func TestEvaluateReplayChartURLIncludesDateTimeAndSignalSide(t *testing.T) {
	loc := mustNY(t)
	set := SettingsFromConfig(config.Defaults())
	set.ChartDate = "2026-07-02"
	set.ChartTime = "0933"
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
