package market

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func event(symbol string, t time.Time, close float64) Event {
	return Event{Symbol: symbol, Start: t, End: t.Add(time.Second), Open: close, High: close + .1, Low: close - .1, Close: close, Volume: 100, VWAP: close, Received: t.Add(150 * time.Millisecond)}
}

func TestIncrementalCAVGIncludesDevelopingMinuteAndMatchesReference(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	e := New([]string{"X"}, loc, 32, nil)
	start := time.Date(2026, 7, 2, 9, 30, 0, 0, loc)
	sum := 0.0
	for i := 0; i < 15; i++ {
		c := 10 + float64(i)/10
		sum += c
		e.Apply(event("X", start.Add(time.Duration(i)*time.Minute), c))
	}
	got, _ := e.Snapshot("X", 15)
	want := got.Price / (sum / 15)
	if got.Ratio != want {
		t.Fatalf("ratio %.12f want %.12f", got.Ratio, want)
	}
}

func TestDuplicateAndOutOfOrderAreIdempotent(t *testing.T) {
	loc := time.UTC
	e := New([]string{"X"}, loc, 8, nil)
	base := time.Date(2026, 7, 2, 13, 30, 0, 0, loc)
	newer := event("X", base.Add(2*time.Second), 12)
	older := event("X", base, 10)
	e.Apply(newer)
	e.Apply(older)
	e.Apply(older)
	s, _ := e.Snapshot("X", 1)
	if s.Price != 12 {
		t.Fatalf("current price regressed to out-of-order event: %v", s.Price)
	}
	if s.Bars[0].Close != 12 {
		t.Fatalf("minute close=%v want timestamp-latest 12", s.Bars[0].Close)
	}
	m := e.Metrics()
	if m.Duplicates != 1 || m.OutOfOrder != 1 {
		t.Fatalf("metrics %+v", m)
	}
}

func TestMissingMinuteIsNotFabricated(t *testing.T) {
	e := New([]string{"X"}, time.UTC, 8, nil)
	base := time.Date(2026, 7, 2, 13, 30, 0, 0, time.UTC)
	e.Apply(event("X", base, 10))
	e.Apply(event("X", base.Add(2*time.Minute), 11))
	s, _ := e.Snapshot("X", 2)
	if len(s.Bars) != 2 {
		t.Fatalf("bars=%d want 2", len(s.Bars))
	}
	if s.Bars[1].Time.Sub(s.Bars[0].Time) != 2*time.Minute {
		t.Fatal("silent minute was fabricated")
	}
}

func TestOpeningBurst2000SymbolsBoundedAndFast(t *testing.T) {
	if testing.Short() {
		t.Skip("load test")
	}
	const symbols = 2000
	names := make([]string, symbols)
	for i := range names {
		names[i] = fmt.Sprintf("S%04d", i)
	}
	e := New(names, time.UTC, 8192, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.Run(ctx)
	base := time.Now().Add(-time.Second)
	started := time.Now()
	for round := 0; round < 3; round++ {
		for i, s := range names {
			ev := event(s, base.Add(time.Duration(round)*time.Second), 10+float64(i%20)/10)
			if !e.Offer(ev) {
				t.Fatal("bounded queue rejected opening record")
			}
			if i%97 == 0 {
				if !e.Offer(ev) {
					t.Fatal("bounded queue rejected duplicate")
				}
			}
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for e.Metrics().Processed+e.Metrics().Duplicates < 6063 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	elapsed := time.Since(started)
	m := e.Metrics()
	if m.Dropped != 0 || m.QueueDepth > m.QueueCapacity {
		t.Fatalf("unbounded/lost: %+v", m)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("6000-record burst processing=%s", elapsed)
	}
	for _, s := range names {
		snap, _ := e.Snapshot(s, 3)
		if snap.LastEvent.IsZero() {
			t.Fatalf("%s missing", s)
		}
	}
	t.Logf("records=%d duplicates=%d elapsed=%s rate=%.0f/s", m.Received, m.Duplicates, elapsed, float64(m.Received)/elapsed.Seconds())
}

func BenchmarkOpeningBurst2000(b *testing.B) {
	names := make([]string, 2000)
	for i := range names {
		names[i] = fmt.Sprintf("S%04d", i)
	}
	base := time.Now()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		e := New(names, time.UTC, 32768, nil)
		for round := 0; round < 3; round++ {
			for _, s := range names {
				e.Apply(event(s, base.Add(time.Duration(round)*time.Second), 10))
			}
		}
	}
}
