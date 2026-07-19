package runner

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"0945-playbook/internal/config"
	"0945-playbook/internal/dashboard"
	"0945-playbook/internal/data"
	"0945-playbook/internal/market"
	"0945-playbook/internal/playbook"
	"0945-playbook/internal/watchlist"
)

type latencyWindow struct {
	mu     sync.Mutex
	values []float64
	max    int
}

func (l *latencyWindow) add(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.values = append(l.values, float64(d.Microseconds())/1000)
	if len(l.values) > l.max {
		copy(l.values, l.values[len(l.values)-l.max:])
		l.values = l.values[:l.max]
	}
}
func (l *latencyWindow) stats() (float64, float64, float64, float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.values) == 0 {
		return 0, 0, 0, 0
	}
	v := append([]float64(nil), l.values...)
	sort.Float64s(v)
	q := func(p float64) float64 { return v[int(float64(len(v)-1)*p)] }
	return q(.5), q(.95), q(.99), v[len(v)-1]
}

type StreamingLive struct {
	*LiveRunner
	stream     data.StreamProvider
	engine     *market.Engine
	dirtyMu    sync.Mutex
	dirty      map[string]time.Time
	generation atomic.Uint64
	updates    chan dashboard.Delta
	published  atomic.Int64
	latency    latencyWindow
	maxAge     time.Duration
	lastKane   time.Time
}

func NewStreamingLive(project string, cfg config.Config, loc *time.Location, items []watchlist.Item, stream data.StreamProvider) *StreamingLive {
	base := NewLive(project, cfg, loc, items, nil)
	s := &StreamingLive{LiveRunner: base, stream: stream, dirty: make(map[string]time.Time), updates: make(chan dashboard.Delta, 64), latency: latencyWindow{max: 60000}, maxAge: config.Duration(cfg.Scan.MaxEventAge)}
	syms := make([]string, len(items))
	for i, item := range items {
		syms[i] = item.Symbol
	}
	s.engine = market.New(syms, loc, cfg.Scan.QueueCapacity, func(symbol string, event, receipt time.Time) {
		s.dirtyMu.Lock()
		if old := s.dirty[symbol]; old.IsZero() || event.After(old) {
			s.dirty[symbol] = event
		}
		s.dirtyMu.Unlock()
	})
	return s
}
func (s *StreamingLive) Run(ctx context.Context) {
	go s.engine.Run(ctx)
	symbols := make([]string, len(s.items))
	for i, item := range s.items {
		symbols[i] = item.Symbol
	}
	if err := s.stream.Subscribe(symbols); err != nil {
		return
	}
	if err := s.stream.Connect(ctx); err != nil {
		return
	}
	defer s.stream.Close()
	go s.warmFromCache(time.Now().In(s.loc).Format("2006-01-02"))
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-s.stream.Events():
				s.engine.Offer(market.Event{Symbol: ev.Symbol, Start: ev.Start, End: ev.End, Open: ev.Open, High: ev.High, Low: ev.Low, Close: ev.Close, Volume: ev.Volume, VWAP: ev.VWAP, AccumulatedVolume: ev.AccumulatedVolume, Received: ev.Received})
			}
		}
	}()
	tick := time.NewTicker(config.Duration(s.cfg.Scan.PublishInterval))
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.publish()
		}
	}
}

func (s *StreamingLive) warmFromCache(day string) {
	for _, item := range s.items {
		bars, err := data.LoadBars(s.cfg.Scan.DataDir, day, item.Symbol)
		if err == nil && len(bars) > 0 {
			s.engine.Seed(item.Symbol, bars)
			last := bars[len(bars)-1].Time
			s.dirtyMu.Lock()
			s.dirty[item.Symbol] = last
			s.dirtyMu.Unlock()
		}
	}
}
func (s *StreamingLive) publish() {
	s.dirtyMu.Lock()
	dirty := s.dirty
	s.dirty = make(map[string]time.Time)
	s.dirtyMu.Unlock()
	if len(dirty) == 0 {
		return
	}
	now := time.Now()
	changed := make([]playbook.Evaluation, 0, len(dirty))
	set := s.set
	set.ChartDate = now.In(s.loc).Format("2006-01-02")
	set.ChartTime = chartClock(now.In(s.loc))
	s.mu.Lock()
	bySymbol := make(map[string]int, len(s.rows))
	for i := range s.rows {
		bySymbol[s.rows[i].Symbol] = i
	}
	for symbol, eventTime := range dirty {
		snap, ok := s.engine.Snapshot(symbol, s.cfg.Extended.AvgCloseBars)
		if !ok {
			continue
		}
		item := s.items[bySymbol[symbol]]
		ev := playbook.Evaluate(item, snap.Bars, now, s.loc, set, nil)
		ev.MarketEventTime = snap.LastEvent.Format(time.RFC3339Nano)
		ev.EventAgeMS = float64(now.Sub(snap.LastEvent).Microseconds()) / 1000
		ev.Stale = now.Sub(snap.LastEvent) > s.maxAge
		if ev.Stale && (ev.Signal || ev.Candidate) {
			ev.Signal = false
			ev.Candidate = false
			ev.Active = false
			ev.Status = "STALE"
			ev.Action = "SUPPRESSED"
			ev.Reason = "Market event is older than the configured alert limit."
		}
		s.rows[bySymbol[symbol]] = ev
		s.barsBySymbol[symbol] = snap.Bars
		changed = append(changed, ev)
		s.latency.add(now.Sub(eventTime))
	}
	s.updated = now
	if now.Sub(s.lastKane) >= time.Second {
		s.lastKane = now
		s.precomputeKaneLocked(now)
	}
	state := dashboard.Build(s.project, "live", now.In(s.loc).Format("15:04:05"), s.cfg.Scan.ChartBaseURL, s.cfg.Scan.MinFirst15VolumeFilter, now, s.rows)
	state.Kane = s.cachedKane
	gen := s.generation.Add(1)
	state.Generation = gen
	state.PublishedAt = now.Format(time.RFC3339Nano)
	s.cachedState = state
	s.mu.Unlock()
	s.published.Store(now.UnixNano())
	d := dashboard.Delta{Generation: gen, PublishedAt: state.PublishedAt, Rows: changed, Stats: state.Stats}
	select {
	case s.updates <- d:
	default:
	}
}
func (s *StreamingLive) Snapshot(context.Context) dashboard.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cachedState
}
func (s *StreamingLive) Updates() <-chan dashboard.Delta { return s.updates }
func (s *StreamingLive) LatencyHealth(context.Context) dashboard.LatencyHealth {
	now := time.Now()
	m := s.engine.Metrics()
	ws := s.stream.Health()
	p50, p95, p99, max := s.latency.stats()
	status := "READY"
	age := now.Sub(m.LastEvent)
	if !ws.Connected {
		status = "DISCONNECTED"
	} else if m.Dropped > 0 {
		status = "DEGRADED"
	} else if m.LastEvent.IsZero() {
		status = "WARMING"
	} else if age > s.maxAge {
		status = "STALE"
	}
	pub := time.Duration(0)
	if n := s.published.Load(); n > 0 {
		pub = now.Sub(time.Unix(0, n))
	}
	stale, silent := 0, 0
	warming := 0
	for _, item := range s.items {
		snap, _ := s.engine.Snapshot(item.Symbol, 15)
		if !snap.Warm {
			warming++
		}
		if snap.LastEvent.IsZero() {
			silent++
		} else if now.Sub(snap.LastEvent) > s.maxAge {
			stale++
		}
	}
	if s.cfg.Scan.WarmupRequired && warming > 0 && status == "READY" {
		status = "WARMING"
	}
	return dashboard.LatencyHealth{Status: status, WebSocket: ws, Market: m, CurrentEventAgeMS: float64(age.Microseconds()) / 1000, BackendPublicationAgeMS: float64(pub.Microseconds()) / 1000, CAVGResultAgeMS: float64(pub.Microseconds()) / 1000, KaneResultAgeMS: float64(now.Sub(s.lastKane).Microseconds()) / 1000, OriginalResultAgeMS: float64(pub.Microseconds()) / 1000, P50MS: p50, P95MS: p95, P99MS: p99, MaxMS: max, StaleSymbols: stale, SilentSymbols: silent, Build: "dev"}
}
