package runner

import (
	"0945-playbook/internal/config"
	"0945-playbook/internal/data"
	"0945-playbook/internal/market"
	"0945-playbook/internal/watchlist"
	"context"
	"testing"
	"time"
)

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
	now := time.Now().In(loc)
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
}
func TestGapSuppressesCAVGUntilExplicitRecovery(t *testing.T) {
	loc := mustNY(t)
	cfg := config.Defaults()
	item := watchlist.Item{Symbol: "TEST"}
	s := NewStreamingLive("test", cfg, loc, []watchlist.Item{item}, connectedStream{})
	now := time.Now().In(loc)
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
