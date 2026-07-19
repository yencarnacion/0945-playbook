package runner

import (
	"fmt"
	"time"
)

type alertInputs struct {
	Scan, Symbol, Session, Condition, Direction string
	Health                                      string
	EventTime, Now                              time.Time
	MaxAge                                      time.Duration
	HistoryComplete, PriorComplete              bool
	Gap, Connected, Subscribed                  bool
}

// alertEligibility is the single safety boundary used by every live scan.
func alertEligibility(in alertInputs) (bool, string) {
	if in.Health != "READY" || !in.HistoryComplete || !in.PriorComplete || in.Gap || !in.Connected || !in.Subscribed || in.EventTime.IsZero() || in.Now.Sub(in.EventTime) > in.MaxAge {
		return false, ""
	}
	return true, fmt.Sprintf("%s:%s:%s:%s:%s:%d", in.Scan, in.Symbol, in.Session, in.Condition, in.Direction, in.EventTime.UnixMilli())
}
