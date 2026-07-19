package runner

import (
	"testing"
	"time"
)

func TestAlertEligibilityRejectsUnsafeState(t *testing.T) {
	now := time.Date(2026, 7, 20, 9, 30, 5, 0, time.UTC)
	base := alertInputs{Scan: "cavg", Symbol: "X", Session: "2026-07-20", Condition: "threshold", Direction: "above", Health: "READY", EventTime: now.Add(-time.Second), Now: now, MaxAge: 10 * time.Second, HistoryComplete: true, PriorComplete: true, Connected: true, Subscribed: true}
	if ok, id := alertEligibility(base); !ok || id == "" {
		t.Fatalf("safe alert rejected: ok=%v id=%q", ok, id)
	}
	for _, mutate := range []func(*alertInputs){
		func(i *alertInputs) { i.Health = "WARMING" },
		func(i *alertInputs) { i.Health = "RECOVERING" },
		func(i *alertInputs) { i.Health = "DEGRADED" },
		func(i *alertInputs) { i.Health = "STALE" },
		func(i *alertInputs) { i.Health = "DISCONNECTED" },
		func(i *alertInputs) { i.Health = "FAILED" },
		func(i *alertInputs) { i.HistoryComplete = false },
		func(i *alertInputs) { i.PriorComplete = false },
		func(i *alertInputs) { i.Gap = true },
		func(i *alertInputs) { i.Connected = false },
		func(i *alertInputs) { i.Subscribed = false },
		func(i *alertInputs) { i.EventTime = now.Add(-11 * time.Second) },
	} {
		input := base
		mutate(&input)
		if ok, _ := alertEligibility(input); ok {
			t.Fatalf("unsafe alert accepted: %+v", input)
		}
	}
}

func TestAlertDeduplication(t *testing.T) {
	s := &StreamingLive{alertSeen: make(map[string]struct{})}
	if !s.firstAlert("stable") || s.firstAlert("stable") {
		t.Fatal("logical alert was not exactly once")
	}
}
