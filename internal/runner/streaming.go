package runner

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"0945-playbook/internal/config"
	"0945-playbook/internal/dashboard"
	"0945-playbook/internal/data"
	"0945-playbook/internal/market"
	"0945-playbook/internal/playbook"
	"0945-playbook/internal/server"
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
	stream              data.StreamProvider
	engine              *market.Engine
	dirtyMu             sync.Mutex
	dirty               map[string]time.Time
	playbookGeneration  atomic.Uint64
	cavgGeneration      atomic.Uint64
	kaneGeneration      atomic.Uint64
	hub                 *server.Hub
	cavgRows            map[string]dashboard.ExtendedRow
	cachedCAVG          dashboard.ExtendedState
	kaneHistory         map[string]dashboard.KaneSnapshot
	published           atomic.Int64
	latency             latencyWindow
	maxAge              time.Duration
	lastKane            time.Time
	backfill            data.Provider
	priorReady          atomic.Int64
	recovering          sync.Map
	priorMu             sync.RWMutex
	prior               map[string]data.SessionSummary
	lastStreamConnected bool
	lastReconnects      uint64
	now                 func() time.Time
	health              map[string]string
	alertSeen           map[string]struct{}
	cavgSide            map[string]int
}

func (s *StreamingLive) SetBackfill(p data.Provider) { s.backfill = p }

func NewStreamingLive(project string, cfg config.Config, loc *time.Location, items []watchlist.Item, stream data.StreamProvider) *StreamingLive {
	base := NewLive(project, cfg, loc, items, nil)
	s := &StreamingLive{LiveRunner: base, stream: stream, dirty: make(map[string]time.Time), hub: server.NewHub(64), cavgRows: make(map[string]dashboard.ExtendedRow), kaneHistory: make(map[string]dashboard.KaneSnapshot), prior: make(map[string]data.SessionSummary), latency: latencyWindow{max: 60000}, maxAge: config.Duration(cfg.Scan.MaxEventAge), now: time.Now, health: make(map[string]string), alertSeen: make(map[string]struct{}), cavgSide: make(map[string]int)}
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
	s.lastStreamConnected = true
	defer s.stream.Close()
	go s.warmFromCache(time.Now().In(s.loc).Format("2006-01-02"))
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-s.stream.Events():
				if !s.engine.Offer(market.Event{Symbol: ev.Symbol, Start: ev.Start, End: ev.End, Open: ev.Open, High: ev.High, Low: ev.Low, Close: ev.Close, Volume: ev.Volume, VWAP: ev.VWAP, AccumulatedVolume: ev.AccumulatedVolume, Received: ev.Received}) {
					go s.recoverGap(ctx, ev.Symbol, ev.Start, ev.End)
				} else if gap, ok := s.engine.GapState(ev.Symbol); ok && (gap.NextAttempt.IsZero() || !ev.End.Before(gap.NextAttempt)) {
					go s.recoverGap(ctx, ev.Symbol, gap.Start, ev.End)
				}
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
			s.checkStreamContinuity()
			s.healthSweep()
			s.publish()
		}
	}
}
func (s *StreamingLive) checkStreamContinuity() {
	h := s.stream.Health()
	if s.lastStreamConnected && !h.Connected {
		s.engine.MarkAllGap(h.LastMessage, time.Now())
	}
	if h.Reconnects > s.lastReconnects {
		s.engine.MarkAllGap(h.LastMessage, time.Now())
		s.lastReconnects = h.Reconnects
	}
	s.lastStreamConnected = h.Connected
}

func healthState(connected, subscribed, priorRequired, priorReady, warm, gap bool, lastEvent, now time.Time, maxAge time.Duration) string {
	if !connected || !subscribed {
		return "DISCONNECTED"
	}
	if gap {
		return "RECOVERING"
	}
	if (priorRequired && !priorReady) || !warm || lastEvent.IsZero() {
		return "WARMING"
	}
	if now.Sub(lastEvent) > maxAge {
		return "STALE"
	}
	return "READY"
}

// healthSweep is independent of market events. It only dirties symbols whose
// externally visible state changed, so silence ages rows without full rescans.
func (s *StreamingLive) healthSweep() {
	now := s.now()
	stream := s.stream.Health()
	s.dirtyMu.Lock()
	defer s.dirtyMu.Unlock()
	for _, item := range s.items {
		view, _ := s.engine.SymbolStatus(item.Symbol, s.cfg.Extended.AvgCloseBars)
		state := healthState(stream.Connected, stream.Subscriptions >= len(s.items), false, true, view.Warm, view.Gap, view.LastEvent, now, s.maxAge)
		if s.health[item.Symbol] != state {
			s.health[item.Symbol] = state
			s.dirty[item.Symbol] = view.LastEvent
		}
	}
}
func (s *StreamingLive) recoverGap(ctx context.Context, symbol string, start, end time.Time) {
	if s.backfill == nil {
		return
	}
	if _, loaded := s.recovering.LoadOrStore(symbol, struct{}{}); loaded {
		return
	}
	defer s.recovering.Delete(symbol)
	token, ok := s.engine.BeginGapRecovery(symbol)
	if !ok {
		return
	}
	bars, err := s.backfill.FetchBars(ctx, symbol, token.Start.Truncate(time.Minute), token.End.Truncate(time.Minute).Add(time.Minute-time.Nanosecond))
	if err == nil {
		_, err = s.engine.ApplyAuthoritativeBarsAt(token, bars, s.now())
	}
	if errors.Is(err, market.ErrDevelopingGap) {
		// Massive minute aggregates do not carry a precise partial-minute coverage
		// watermark. Keep the gap open and accept later live seconds; verification
		// will retry after the minute closes.
		return
	}
	if err != nil {
		s.engine.AbortGapRecovery(token, err)
		return
	}
	{
		s.dirtyMu.Lock()
		s.dirty[symbol] = end
		s.dirtyMu.Unlock()
	}
}

func (s *StreamingLive) warmFromCache(day string) {
	today, _ := time.ParseInLocation("2006-01-02", day, s.loc)
	priorDay := data.PreviousTradingDay(today)
	prior := priorDay.Format("2006-01-02")
	for _, item := range s.items {
		if sum, err := data.LoadSummary(s.cfg.Scan.DataDir, prior, item.Symbol); err == nil {
			s.seedPriorSummary(sum)
			s.priorReady.Add(1)
		} else if priorBars, err := data.LoadBars(s.cfg.Scan.DataDir, prior, item.Symbol); err == nil {
			if sum, ok := data.Summarize(item.Symbol, prior, priorBars, s.loc); ok {
				_ = data.SaveSummary(s.cfg.Scan.DataDir, sum)
				s.seedPriorSummary(sum)
				s.priorReady.Add(1)
			}
		} else if s.backfill != nil {
			start := time.Date(priorDay.Year(), priorDay.Month(), priorDay.Day(), 4, 0, 0, 0, s.loc)
			end := time.Date(priorDay.Year(), priorDay.Month(), priorDay.Day(), 16, 0, 0, 0, s.loc)
			if priorBars, e := s.backfill.FetchBars(context.Background(), item.Symbol, start, end); e == nil {
				if sum, ok := data.Summarize(item.Symbol, prior, priorBars, s.loc); ok {
					_ = data.SaveSummary(s.cfg.Scan.DataDir, sum)
					s.seedPriorSummary(sum)
					s.priorReady.Add(1)
				}
			}
		}
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
func (s *StreamingLive) seedPriorSummary(sum data.SessionSummary) {
	s.priorMu.Lock()
	s.prior[sum.Symbol] = sum
	s.priorMu.Unlock()
}
func (s *StreamingLive) publish() {
	s.dirtyMu.Lock()
	dirty := s.dirty
	s.dirty = make(map[string]time.Time)
	s.dirtyMu.Unlock()
	if len(dirty) == 0 {
		return
	}
	now := s.now()
	changed := make([]playbook.Evaluation, 0, len(dirty))
	cavgChanged := false
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
		ev.Gap = snap.Gap
		streamHealth := s.stream.Health()
		ev.Health = healthState(streamHealth.Connected, streamHealth.Subscriptions >= len(s.items), false, true, snap.Warm, snap.Gap, snap.LastEvent, now, s.maxAge)
		if ev.Health != "READY" && (ev.Signal || ev.Candidate) {
			ev.Signal = false
			ev.Candidate = false
			ev.Active = false
			ev.Status = ev.Health
			ev.Action = "SUPPRESSED"
			ev.Reason = "Market event is older than the configured alert limit."
		}
		if ev.Signal || ev.Candidate {
			eligible, _ := alertEligibility(alertInputs{Scan: "playbook", Symbol: symbol, Session: snap.SessionDate, Condition: ev.Action, Direction: ev.Branch, Health: ev.Health, EventTime: snap.LastEvent, Now: now, MaxAge: s.maxAge, HistoryComplete: snap.Warm, PriorComplete: true, Gap: snap.Gap, Connected: streamHealth.Connected, Subscribed: streamHealth.Subscriptions >= len(s.items)})
			ev.AlertID = "playbook:" + symbol + ":" + snap.SessionDate + ":" + ev.Action + ":" + ev.Branch
			ev.AlertEligible = eligible && s.firstAlert(ev.AlertID)
		}
		s.rows[bySymbol[symbol]] = ev
		s.barsBySymbol[symbol] = snap.Bars
		if ev.Health == "READY" && snap.Average15 > 0 && (snap.Ratio < s.cfg.Extended.LowerSignalRatio || snap.Ratio > s.cfg.Extended.UpperSignalRatio) {
			side := 1
			if snap.Ratio < 1 {
				side = -1
			}
			row := dashboard.ExtendedRow{Symbol: item.Symbol, Name: item.Name, Industry: item.Industry, Order: item.Order, Price: snap.Price, Average: snap.Average15, Ratio: snap.Ratio, DeltaPct: snap.Ratio - 1, Volume: snap.SessionVolume, Side: side, Clock: now.In(s.loc).Format("15:04"), ChartURL: chartURL(s.cfg.Scan.ChartBaseURL, item.Symbol, now.In(s.loc).Format("2006-01-02"), now.In(s.loc).Format("1504"), side), MarketEventTime: snap.LastEvent.Format(time.RFC3339Nano), EventAgeMS: float64(now.Sub(snap.LastEvent).Microseconds()) / 1000, Health: "READY"}
			if s.cavgSide[symbol] != side {
				direction := "above"
				if side < 0 {
					direction = "below"
				}
				eligible, id := alertEligibility(alertInputs{Scan: "cavg", Symbol: symbol, Session: snap.SessionDate, Condition: "threshold", Direction: direction, Health: ev.Health, EventTime: snap.LastEvent, Now: now, MaxAge: s.maxAge, HistoryComplete: snap.Warm, PriorComplete: true, Gap: snap.Gap, Connected: streamHealth.Connected, Subscribed: streamHealth.Subscriptions >= len(s.items)})
				row.AlertID, row.AlertEligible = id, eligible && s.firstAlert(id)
			}
			s.cavgSide[symbol] = side
			s.cavgRows[symbol] = row
		} else {
			delete(s.cavgRows, symbol)
			s.cavgSide[symbol] = 0
		}
		cavgChanged = true
		changed = append(changed, ev)
		s.latency.add(now.Sub(eventTime))
	}
	s.updated = now
	if now.Sub(s.lastKane) >= time.Second {
		s.lastKane = now
		s.precomputeStreamingKaneLocked(now)
		kg := s.kaneGeneration.Add(1)
		s.cachedKane.Generation = kg
		s.cachedKane.PublishedAt = now.Format(time.RFC3339Nano)
		streamHealth := s.stream.Health()
		s.cachedKane.Health = healthState(streamHealth.Connected, streamHealth.Subscriptions >= len(s.items), true, s.priorReady.Load() >= int64(len(s.items)), true, s.engine.GapCount() > 0, s.engine.Metrics().LastEvent, now, s.maxAge)
		lastEvent := s.engine.Metrics().LastEvent
		for i := range s.cachedKane.Rows {
			row := &s.cachedKane.Rows[i]
			row.Health = s.cachedKane.Health
			row.EventAgeMS = float64(now.Sub(lastEvent).Microseconds()) / 1000
			eligible, _ := alertEligibility(alertInputs{Scan: "kane", Symbol: row.Symbol, Session: now.In(s.loc).Format("2006-01-02"), Condition: row.Setup, Direction: "entry", Health: row.Health, EventTime: lastEvent, Now: now, MaxAge: s.maxAge, HistoryComplete: true, PriorComplete: s.priorReady.Load() >= int64(len(s.items)), Gap: s.engine.GapCount() > 0, Connected: streamHealth.Connected, Subscribed: streamHealth.Subscriptions >= len(s.items)})
			row.AlertID = "kane:" + row.Symbol + ":" + now.In(s.loc).Format("2006-01-02") + ":" + row.Setup
			row.AlertEligible = eligible && s.firstAlert(row.AlertID)
		}
	}
	if cavgChanged {
		s.buildCAVGLocked(now)
	}
	state := dashboard.Build(s.project, "live", now.In(s.loc).Format("15:04:05"), s.cfg.Scan.ChartBaseURL, s.cfg.Scan.MinFirst15VolumeFilter, now, s.rows)
	state.Kane = s.cachedKane
	gen := s.playbookGeneration.Add(1)
	state.Generation = gen
	state.ProtocolVersion = 1
	state.PlaybookGeneration = gen
	state.CAVGGeneration = s.cavgGeneration.Load()
	state.KaneGeneration = s.kaneGeneration.Load()
	state.CAVG = s.cachedCAVG
	state.PublishedAt = now.Format(time.RFC3339Nano)
	s.cachedState = state
	s.mu.Unlock()
	s.published.Store(now.UnixNano())
	d := dashboard.Delta{ProtocolVersion: 1, Type: "delta", Generation: gen, PlaybookGeneration: gen, PlaybookBaseGeneration: gen - 1, CAVGGeneration: s.cavgGeneration.Load(), CAVGBaseGeneration: s.cavgGeneration.Load() - 1, KaneGeneration: s.kaneGeneration.Load(), KaneBaseGeneration: s.kaneGeneration.Load() - 1, PublishedAt: state.PublishedAt, LatestMarketEvent: s.engine.Metrics().LastEvent.Format(time.RFC3339Nano), Health: "READY", Rows: changed, Stats: state.Stats, Kane: &s.cachedKane, CAVG: &s.cachedCAVG}
	s.hub.Publish(d)
}

func (s *StreamingLive) firstAlert(id string) bool {
	if id == "" {
		return false
	}
	if _, exists := s.alertSeen[id]; exists {
		return false
	}
	s.alertSeen[id] = struct{}{}
	return true
}
func (s *StreamingLive) precomputeStreamingKaneLocked(now time.Time) {
	kaneBars := cloneBarsBySymbol(s.barsBySymbol)
	s.priorMu.RLock()
	for symbol, sum := range s.prior {
		d, _ := time.ParseInLocation("2006-01-02", sum.Date, s.loc)
		kaneBars[symbol] = append(kaneBars[symbol], data.Bar{Time: time.Date(d.Year(), d.Month(), d.Day(), 9, 30, 0, 0, s.loc).UTC(), Open: sum.Open, High: sum.High, Low: sum.Low, Close: sum.Open, Volume: sum.Volume / 2}, data.Bar{Time: time.Date(d.Year(), d.Month(), d.Day(), 15, 59, 0, 0, s.loc).UTC(), Open: sum.Close, High: sum.High, Low: sum.Low, Close: sum.Close, Volume: sum.Volume / 2})
	}
	s.priorMu.RUnlock()
	rows, prelim := kaneRows(s.items, kaneBars, now, s.loc, s.cfg.Scan.ChartBaseURL)
	state := dashboard.KaneState{Available: true, Preliminary: prelim, Rows: rows}
	day := now.In(s.loc)
	for minute := 25; minute <= 30; minute++ {
		stamp := time.Date(day.Year(), day.Month(), day.Day(), 9, minute, 0, 0, s.loc)
		if stamp.After(now) {
			continue
		}
		key := stamp.Format("2006-01-02T15:04")
		snap, ok := s.kaneHistory[key]
		if !ok {
			r, p := kaneRows(s.items, kaneBars, stamp, s.loc, s.cfg.Scan.ChartBaseURL)
			snap = dashboard.KaneSnapshot{Clock: stamp.Format("15:04"), Preliminary: p, Rows: r}
			s.kaneHistory[key] = snap
		}
		state.History = append(state.History, snap)
	}
	s.cachedKane = state
}
func (s *StreamingLive) buildCAVGLocked(now time.Time) {
	rows := make([]dashboard.ExtendedRow, 0, len(s.cavgRows))
	for _, r := range s.cavgRows {
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		di, dj := abs(rows[i].DeltaPct), abs(rows[j].DeltaPct)
		if di == dj {
			return rows[i].Order < rows[j].Order
		}
		return di > dj
	})
	gen := s.cavgGeneration.Add(1)
	snap := dashboard.ExtendedSnapshot{ID: now.Unix() / 60, Clock: now.In(s.loc).Format("15:04"), Updated: now.Format(time.RFC3339Nano), Rows: rows}
	s.cachedCAVG = dashboard.ExtendedState{Generation: gen, PublishedAt: now.Format(time.RFC3339Nano), Health: "READY", Available: true, WindowStart: s.cfg.Extended.Start, WindowEnd: s.cfg.Extended.End, AvgCloseBars: s.cfg.Extended.AvgCloseBars, UpperSignalRatio: s.cfg.Extended.UpperSignalRatio, LowerSignalRatio: s.cfg.Extended.LowerSignalRatio, SoundURL: "/api/extended/sound", LiveID: snap.ID, Selected: snap, History: []dashboard.ExtendedHistoryPoint{{ID: snap.ID, Clock: snap.Clock, Count: len(rows)}}}
}
func (s *StreamingLive) Snapshot(context.Context) dashboard.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cachedState
}
func (s *StreamingLive) ExtendedSnapshot(context.Context, int64) dashboard.ExtendedState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cachedCAVG
}
func (s *StreamingLive) Hub() *server.Hub { return s.hub }
func (s *StreamingLive) FullSnapshotInterval() time.Duration {
	return config.Duration(s.cfg.Dashboard.FullSnapshotInterval)
}
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
	s.mu.RLock()
	kaneHealth := s.cachedKane.Health
	kaneAge := now.Sub(s.lastKane)
	s.mu.RUnlock()
	return dashboard.LatencyHealth{Status: status, PlaybookStatus: status, CAVGStatus: status, KaneStatus: kaneHealth, WebSocket: ws, Market: m, CurrentEventAgeMS: float64(age.Microseconds()) / 1000, BackendPublicationAgeMS: float64(pub.Microseconds()) / 1000, CAVGResultAgeMS: float64(pub.Microseconds()) / 1000, KaneResultAgeMS: float64(kaneAge.Microseconds()) / 1000, OriginalResultAgeMS: float64(pub.Microseconds()) / 1000, P50MS: p50, P95MS: p95, P99MS: p99, MaxMS: max, StaleSymbols: stale, SilentSymbols: silent, Build: "dev", Gaps: s.engine.GapCount(), PlaybookGeneration: s.playbookGeneration.Load(), CAVGGeneration: s.cavgGeneration.Load(), KaneGeneration: s.kaneGeneration.Load(), Browsers: s.hub.Stats()}
}
