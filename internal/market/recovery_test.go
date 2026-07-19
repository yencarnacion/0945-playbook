package market

import (
	"0945-playbook/internal/data"
	"testing"
	"time"
)

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
