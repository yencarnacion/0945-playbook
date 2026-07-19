package runner

import (
	"0945-playbook/internal/config"
	"0945-playbook/internal/data"
	"0945-playbook/internal/market"
	"0945-playbook/internal/watchlist"
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type mutableStream struct{ connected atomic.Bool }

func (m *mutableStream) Connect(context.Context) error   { m.connected.Store(true); return nil }
func (m *mutableStream) Subscribe([]string) error        { return nil }
func (m *mutableStream) Unsubscribe([]string) error      { return nil }
func (m *mutableStream) Events() <-chan data.StreamEvent { return make(chan data.StreamEvent) }
func (m *mutableStream) Health() data.StreamHealth {
	return data.StreamHealth{Connected: m.connected.Load(), Subscriptions: 1}
}
func (m *mutableStream) Close() error { return nil }

type connectedStream struct{}

func (connectedStream) Connect(context.Context) error   { return nil }
func (connectedStream) Subscribe([]string) error        { return nil }
func (connectedStream) Unsubscribe([]string) error      { return nil }
func (connectedStream) Events() <-chan data.StreamEvent { return make(chan data.StreamEvent) }
func (connectedStream) Health() data.StreamHealth {
	return data.StreamHealth{Connected: true, Subscriptions: 1}
}
func (connectedStream) Close() error { return nil }
func TestStreamingPublishPopulatesCAVGAndKaneDelta(t *testing.T) {
	loc := mustNY(t)
	cfg := config.Defaults()
	cfg.Scan.MinFirst15VolumeFilter = 0
	item := watchlist.Item{Symbol: "TEST", ATR14: 2}
	s := NewStreamingLive("test", cfg, loc, []watchlist.Item{item}, connectedStream{})
	client := s.Hub().Subscribe()
	defer client.Close()
	now := time.Date(2026, 7, 20, 10, 0, 30, 0, loc)
	s.now = func() time.Time { return now }
	start := now.Truncate(time.Minute).Add(-14 * time.Minute)
	for i := 0; i < 15; i++ {
		c := 10.0
		if i == 14 {
			c = 10.5
		}
		at := start.Add(time.Duration(i) * time.Minute)
		end := at.Add(time.Second)
		if i == 14 {
			end = now
		}
		s.engine.Apply(market.Event{Symbol: "TEST", Start: at, End: end, Open: c, High: c, Low: c, Close: c, Volume: 100, VWAP: c, Received: time.Now()})
	}
	s.publish()
	snap := s.Snapshot(context.Background())
	if len(snap.CAVG.Selected.Rows) != 1 || snap.CAVG.Selected.Rows[0].Symbol != "TEST" {
		t.Fatalf("cavg snapshot %+v", snap.CAVG)
	}
	if !snap.CAVG.Selected.Rows[0].AlertEligible || snap.CAVG.Selected.Rows[0].AlertID == "" {
		t.Fatalf("first crossing was not eligible: %+v", snap.CAVG.Selected.Rows[0])
	}
	if snap.Kane.Health != "WARMING" {
		t.Fatalf("Kane health=%s without prior summary", snap.Kane.Health)
	}
	select {
	case d := <-client.C:
		if d.CAVG == nil || len(d.CAVG.Selected.Rows) != 1 || d.Kane == nil {
			t.Fatalf("delta %+v", d)
		}
	case <-time.After(time.Second):
		t.Fatal("missing live delta")
	}
	// Re-publication, duplicate delivery, reconnect, and browser resync all retain
	// the backend crossing identity and cannot make the same side eligible again.
	s.dirty["TEST"] = now
	s.publish()
	if s.cachedCAVG.Selected.Rows[0].AlertEligible {
		t.Fatal("duplicate crossing alert")
	}
}
func TestGapSuppressesCAVGUntilExplicitRecovery(t *testing.T) {
	loc := mustNY(t)
	cfg := config.Defaults()
	item := watchlist.Item{Symbol: "TEST"}
	s := NewStreamingLive("test", cfg, loc, []watchlist.Item{item}, connectedStream{})
	now := time.Date(2026, 7, 20, 10, 0, 30, 0, loc)
	s.now = func() time.Time { return now }
	bars := make([]data.Bar, 15)
	for i := range bars {
		c := 10.0
		if i == 14 {
			c = 10.5
		}
		bars[i] = data.Bar{Time: now.Truncate(time.Minute).Add(time.Duration(i-14) * time.Minute), Open: c, High: c, Low: c, Close: c, Volume: 100}
	}
	s.engine.Seed("TEST", bars)
	s.engine.Apply(market.Event{Symbol: "TEST", Start: now.Truncate(time.Minute), End: now, Open: 10.5, High: 10.5, Low: 10.5, Close: 10.5, Volume: 1, VWAP: 10.5, Received: time.Now()})
	s.engine.MarkAllGap(now.Add(-time.Minute), now)
	s.dirty["TEST"] = now
	s.publish()
	if len(s.cachedCAVG.Selected.Rows) != 0 {
		t.Fatal("gap produced C/Avg alert row")
	}
	if !s.engine.ResolveGap("TEST", bars) {
		t.Fatal("recovery failed")
	}
	s.dirty["TEST"] = now
	s.publish()
	if len(s.cachedCAVG.Selected.Rows) != 1 {
		t.Fatal("resolved gap did not restore eligibility")
	}
}

func TestHealthSweepPublishesSilentStaleAndDisconnect(t *testing.T) {
	loc := mustNY(t)
	cfg := config.Defaults()
	cfg.Scan.MinFirst15VolumeFilter = 0
	stream := &mutableStream{}
	stream.connected.Store(true)
	s := NewStreamingLive("test", cfg, loc, []watchlist.Item{{Symbol: "TEST"}}, stream)
	base := time.Date(2026, 7, 20, 10, 0, 0, 0, loc)
	s.now = func() time.Time { return base }
	for i := 0; i < 15; i++ {
		at := base.Add(time.Duration(i-14) * time.Minute)
		s.engine.Apply(market.Event{Symbol: "TEST", Start: at, End: at.Add(time.Second), Open: 10, High: 10, Low: 10, Close: 10, Volume: 1, Received: base})
	}
	s.healthSweep()
	s.publish()
	if got := s.cachedState.Rows[0].Health; got != "READY" {
		t.Fatalf("initial health=%s", got)
	}
	s.now = func() time.Time { return base.Add(s.maxAge + 2*time.Second) }
	s.healthSweep()
	s.publish()
	if got := s.cachedState.Rows[0].Health; got != "STALE" {
		t.Fatalf("silent transition health=%s", got)
	}
	staleGeneration := s.playbookGeneration.Load()
	stream.connected.Store(false)
	s.healthSweep()
	s.publish()
	if got := s.cachedState.Rows[0].Health; got != "DISCONNECTED" {
		t.Fatalf("disconnect health=%s", got)
	}
	if s.playbookGeneration.Load() <= staleGeneration {
		t.Fatal("health-only transition did not advance generation")
	}
}
