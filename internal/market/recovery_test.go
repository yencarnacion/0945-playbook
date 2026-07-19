package market

import (
	"0945-playbook/internal/data"
	"errors"
	"testing"
	"time"
)

func TestDevelopingMinuteRecoveryNeverFreezesLiveUpdates(t *testing.T) {
	e := New([]string{"X"}, time.UTC, 64, nil)
	minute := time.Date(2026, 7, 2, 13, 30, 0, 0, time.UTC)
	applySecond := func(second int, close, high, low, volume float64) {
		at := minute.Add(time.Duration(second) * time.Second)
		e.Apply(Event{Symbol: "X", Start: at, End: at.Add(time.Second), Open: close, High: high, Low: low, Close: close, Volume: volume, VWAP: close, Received: at.Add(time.Millisecond)})
	}
	applySecond(7, 10, 10, 9.9, 100)
	// The 09:30:08 second is lost.
	e.MarkGap("X", minute.Add(8*time.Second), minute.Add(9*time.Second), "lost_second")
	token, _ := e.BeginGapRecovery("X")
	partial := recoveryBar(minute, 10.1)
	partial.High, partial.Low, partial.Volume = 10.1, 9.8, 150
	resolved, err := e.ApplyAuthoritativeBarsAt(token, []data.Bar{partial}, minute.Add(10*time.Second))
	if resolved || !errors.Is(err, ErrDevelopingGap) || !e.HasGap("X") {
		t.Fatalf("partial current minute resolved=%v err=%v gap=%v", resolved, err, e.HasGap("X"))
	}
	gap, _ := e.GapState("X")
	if !gap.NextAttempt.Equal(minute.Add(time.Minute)) {
		t.Fatalf("next recovery=%s want minute close", gap.NextAttempt)
	}
	applySecond(11, 10.2, 10.3, 9.7, 110)
	applySecond(59, 10.5, 10.8, 9.6, 120)
	snap, _ := e.Snapshot("X", 15)
	if snap.Price != 10.5 || snap.Bars[0].High != 10.8 || snap.Bars[0].Low != 9.6 {
		t.Fatalf("developing minute froze: %+v", snap.Bars[0])
	}
	if snap.Bars[0].Volume != 330 { // partial REST volume was never double-counted
		t.Fatalf("developing volume=%v want=330", snap.Bars[0].Volume)
	}
	if !snap.Gap {
		t.Fatal("developing continuity was falsely verified")
	}

	// Once 09:30 is completed, a newly fetched complete aggregate replaces it.
	finalToken, _ := e.BeginGapRecovery("X")
	complete := recoveryBar(minute, 10.6)
	complete.High, complete.Low, complete.Volume, complete.VWAP = 10.9, 9.5, 600, 10.25
	resolved, err = e.ApplyAuthoritativeBarsAt(finalToken, []data.Bar{complete}, minute.Add(time.Minute+time.Second))
	if err != nil || !resolved {
		t.Fatalf("completed recovery resolved=%v err=%v", resolved, err)
	}
	snap, _ = e.Snapshot("X", 1)
	if snap.Gap || snap.Price != 10.6 || snap.Bars[0].Volume != 600 || snap.Bars[0].High != 10.9 || snap.Bars[0].Low != 9.5 {
		t.Fatalf("completed authoritative replacement: %+v", snap)
	}
	// A late live correction cannot corrupt the finalized authoritative minute.
	applySecond(58, 99, 99, 1, 999)
	snap, _ = e.Snapshot("X", 1)
	if snap.Price != 10.6 || snap.Bars[0].Volume != 600 {
		t.Fatalf("late event corrupted finalized minute: %+v", snap.Bars[0])
	}
}

func recoveryBar(at time.Time, c float64) data.Bar {
	return data.Bar{Time: at.UTC().Truncate(time.Minute), Open: c, High: c, Low: c, Close: c, Volume: 100}
}
func TestAuthoritativeRecoveryReplacesIncompleteBar(t *testing.T) {
	e := New([]string{"X"}, time.UTC, 8, nil)
	m := time.Date(2026, 7, 2, 13, 30, 0, 0, time.UTC)
	e.Seed("X", []data.Bar{recoveryBar(m, 1)})
	e.MarkGap("X", m, m.Add(59*time.Second), "partial_minute")
	tok, ok := e.BeginGapRecovery("X")
	if !ok {
		t.Fatal("no token")
	}
	if err := e.ApplyAuthoritativeBars(tok, []data.Bar{recoveryBar(m, 10)}); err != nil {
		t.Fatal(err)
	}
	// A late aggregate for a recovered completed minute cannot double its volume
	// or overwrite the authoritative REST close.
	e.Apply(Event{Symbol: "X", Start: m, End: m.Add(30 * time.Second), Open: 1, High: 20, Low: 1, Close: 20, Volume: 50, Received: m.Add(time.Minute)})
	s, _ := e.Snapshot("X", 1)
	if s.Bars[0].Close != 10 || s.Bars[0].Volume != 100 || s.Gap {
		t.Fatalf("authoritative replacement failed %+v", s)
	}
}
func TestPartialEmptyAndOldVersionCannotClearGap(t *testing.T) {
	e := New([]string{"X"}, time.UTC, 8, nil)
	m := time.Date(2026, 7, 2, 13, 30, 0, 0, time.UTC)
	e.MarkGap("X", m, m.Add(time.Minute), "disconnect")
	old, _ := e.BeginGapRecovery("X")
	if err := e.ApplyAuthoritativeBars(old, nil); err == nil {
		t.Fatal("empty cleared gap")
	}
	e.MarkGap("X", m, m.Add(2*time.Minute), "expanded")
	if err := e.ApplyAuthoritativeBars(old, []data.Bar{recoveryBar(m, 1), recoveryBar(m.Add(time.Minute), 1)}); err == nil {
		t.Fatal("old version cleared expanded gap")
	}
	g, ok := e.GapState("X")
	if !ok || g.Version != 2 {
		t.Fatalf("gap=%+v", g)
	}
	cur, _ := e.BeginGapRecovery("X")
	if err := e.ApplyAuthoritativeBars(cur, []data.Bar{recoveryBar(m, 1), recoveryBar(m.Add(time.Minute), 1)}); err == nil {
		t.Fatal("partial coverage cleared gap")
	}
	if !e.HasGap("X") {
		t.Fatal("partial recovery cleared gap")
	}
}
func TestRepeatedRecoveryIsIdempotent(t *testing.T) {
	e := New([]string{"X"}, time.UTC, 8, nil)
	m := time.Date(2026, 7, 2, 13, 30, 0, 0, time.UTC)
	e.MarkGap("X", m, m, "loss")
	tok, _ := e.BeginGapRecovery("X")
	bars := []data.Bar{recoveryBar(m, 3)}
	if err := e.ApplyAuthoritativeBars(tok, bars); err != nil {
		t.Fatal(err)
	}
	if err := e.ApplyAuthoritativeBars(tok, bars); err == nil {
		t.Fatal("committed token reused")
	}
}

func TestAuthoritativeReplacementRecomputesCAVGWindow(t *testing.T) {
	e := New([]string{"X"}, time.UTC, 64, nil)
	current := time.Date(2026, 7, 2, 13, 30, 0, 0, time.UTC)
	history := make([]data.Bar, 0, 14)
	for i := 14; i > 0; i-- {
		history = append(history, recoveryBar(current.Add(-time.Duration(i)*time.Minute), 10))
	}
	e.Seed("X", history)
	e.Apply(Event{Symbol: "X", Start: current, End: current.Add(10 * time.Second), Open: 11, High: 11, Low: 11, Close: 11, Volume: 10, VWAP: 11, Received: current.Add(10 * time.Second)})
	before, _ := e.Snapshot("X", 15)
	e.MarkGap("X", current, current.Add(10*time.Second), "lost_second")
	token, _ := e.BeginGapRecovery("X")
	complete := recoveryBar(current, 20)
	if resolved, err := e.ApplyAuthoritativeBarsAt(token, []data.Bar{complete}, current.Add(time.Minute)); err != nil || !resolved {
		t.Fatalf("resolved=%v err=%v", resolved, err)
	}
	after, _ := e.Snapshot("X", 15)
	want := (14*10.0 + 20) / 15
	if !after.Warm || after.Average15 != want || after.Ratio != 20/want || before.Average15 == after.Average15 {
		t.Fatalf("before avg=%v after=%+v want=%v", before.Average15, after, want)
	}
}
