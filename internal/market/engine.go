package market

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"0945-playbook/internal/data"
)

// Event is a provider-neutral aggregate. EventTime is provider time; Received is
// captured locally before the provider reader enqueues the record.
type Event struct {
	Symbol                               string
	Start, End                           time.Time
	Open, High, Low, Close, Volume, VWAP float64
	AccumulatedVolume                    float64
	Received                             time.Time
}

type SymbolSnapshot struct {
	Symbol                                                 string     `json:"symbol"`
	Bars                                                   []data.Bar `json:"-"`
	Price, SessionVolume, PremarketVolume, High, Low, VWAP float64
	Average15, Ratio                                       float64
	LastEvent                                              time.Time `json:"last_event"`
	LastReceipt                                            time.Time `json:"last_receipt"`
	Warm                                                   bool      `json:"warm"`
	Gap                                                    bool      `json:"gap"`
	GapStart                                               time.Time `json:"gap_start,omitempty"`
	GapEnd                                                 time.Time `json:"gap_end,omitempty"`
	EligibleBarCount                                       int       `json:"eligible_bar_count"`
	SessionDate                                            string    `json:"session_date"`
	OldestWindowMinute                                     time.Time `json:"oldest_window_minute,omitempty"`
	NewestWindowMinute                                     time.Time `json:"newest_window_minute,omitempty"`
	LastCompletedMinute                                    time.Time `json:"last_completed_minute,omitempty"`
	DevelopingMinute                                       time.Time `json:"developing_minute,omitempty"`
}

type Metrics struct {
	Received         uint64    `json:"received"`
	Processed        uint64    `json:"processed"`
	Invalid          uint64    `json:"invalid"`
	Duplicates       uint64    `json:"duplicates"`
	OutOfOrder       uint64    `json:"out_of_order"`
	Dropped          uint64    `json:"dropped"`
	QueueDepth       int       `json:"queue_depth"`
	QueueCapacity    int       `json:"queue_capacity"`
	OldestQueueAgeMS float64   `json:"oldest_queue_age_ms"`
	LastReceive      time.Time `json:"last_receive"`
	LastEvent        time.Time `json:"last_event"`
}
type GapState struct {
	ID                      uint64 `json:"id"`
	Version                 uint64 `json:"version"`
	Start, End              time.Time
	Cause                   string
	CreatedAt, LastExpanded time.Time
	Status                  string
	Attempts                int
	LastError               string
	Granularity             string
	NextAttempt             time.Time
}
type RecoveryToken struct {
	Symbol         string
	GapID, Version uint64
	Start, End     time.Time
}

var ErrDevelopingGap = errors.New("gap intersects developing minute")

// SymbolStatus is the narrow, allocation-free view used by health checks.
type SymbolStatus struct {
	LastEvent time.Time
	Warm      bool
	Gap       bool
	Session   string
}

type eventKey struct{ start, end int64 }

type symbolState struct {
	mu                                                      sync.RWMutex
	bars                                                    []data.Bar
	barLast                                                 map[int64]time.Time
	seen                                                    map[eventKey]struct{}
	seenOrder                                               []eventKey
	authoritative                                           map[int64]struct{}
	lastEvent, lastReceipt                                  time.Time
	price, sessionVolume, premarketVolume, high, low, pv, v float64
	gap                                                     GapState
}

type Engine struct {
	loc                                                           *time.Location
	queue                                                         chan Event
	symbols                                                       map[string]*symbolState
	received, processed, invalid, duplicates, outOfOrder, dropped atomic.Uint64
	lastReceive, lastEvent                                        atomic.Int64
	onChange                                                      func(string, time.Time, time.Time)
	nextGapID                                                     atomic.Uint64
}

func New(symbols []string, loc *time.Location, capacity int, onChange func(string, time.Time, time.Time)) *Engine {
	if capacity < 1 {
		capacity = 8192
	}
	e := &Engine{loc: loc, queue: make(chan Event, capacity), symbols: make(map[string]*symbolState, len(symbols)), onChange: onChange}
	for _, s := range symbols {
		s = strings.ToUpper(strings.TrimSpace(s))
		if s != "" {
			e.symbols[s] = &symbolState{barLast: make(map[int64]time.Time), seen: make(map[eventKey]struct{}, 64), authoritative: make(map[int64]struct{})}
		}
	}
	return e
}

func (e *Engine) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-e.queue:
			e.apply(ev)
		}
	}
}

// Offer never waits on scan or browser work. A full correctness queue is reported
// as a hard data-quality loss so health becomes DEGRADED; it is never hidden.
func (e *Engine) Offer(ev Event) bool {
	if ev.Received.IsZero() {
		ev.Received = time.Now()
	}
	e.received.Add(1)
	e.lastReceive.Store(ev.Received.UnixNano())
	select {
	case e.queue <- ev:
		return true
	default:
		e.dropped.Add(1)
		e.MarkGap(ev.Symbol, ev.Start, ev.End, "queue_overflow")
		return false
	}
}

func (e *Engine) Apply(ev Event) { // deterministic replay/tests
	if ev.Received.IsZero() {
		ev.Received = time.Now()
	}
	e.received.Add(1)
	e.lastReceive.Store(ev.Received.UnixNano())
	e.apply(ev)
}

func (e *Engine) apply(ev Event) {
	s := e.symbols[strings.ToUpper(ev.Symbol)]
	if s == nil || ev.Close <= 0 || ev.Open <= 0 || ev.High <= 0 || ev.Low <= 0 || ev.End.IsZero() {
		e.invalid.Add(1)
		return
	}
	if ev.Start.IsZero() {
		ev.Start = ev.End.Add(-time.Second)
	}
	key := eventKey{start: ev.Start.UnixNano(), end: ev.End.UnixNano()}
	s.mu.Lock()
	if _, ok := s.seen[key]; ok {
		s.mu.Unlock()
		e.duplicates.Add(1)
		return
	}
	s.seen[key] = struct{}{}
	s.seenOrder = append(s.seenOrder, key)
	if len(s.seenOrder) > 256 {
		old := s.seenOrder[0]
		s.seenOrder = s.seenOrder[1:]
		delete(s.seen, old)
	}
	if !s.lastEvent.IsZero() && ev.End.Before(s.lastEvent) {
		e.outOfOrder.Add(1)
	}
	minute := ev.Start.In(e.loc).Truncate(time.Minute).UTC()
	if _, recovered := s.authoritative[minute.Unix()]; recovered {
		s.mu.Unlock()
		e.duplicates.Add(1)
		return
	}
	i := sort.Search(len(s.bars), func(i int) bool { return !s.bars[i].Time.Before(minute) })
	if i == len(s.bars) || !s.bars[i].Time.Equal(minute) {
		b := data.Bar{Time: minute, Open: ev.Open, High: ev.High, Low: ev.Low, Close: ev.Close, Volume: ev.Volume, VWAP: ev.VWAP}
		s.bars = append(s.bars, data.Bar{})
		copy(s.bars[i+1:], s.bars[i:])
		s.bars[i] = b
		s.barLast[minute.Unix()] = ev.End
	} else {
		b := &s.bars[i]
		if ev.High > b.High {
			b.High = ev.High
		}
		if ev.Low < b.Low {
			b.Low = ev.Low
		}
		last := s.barLast[minute.Unix()]
		if last.IsZero() || !ev.End.Before(last) {
			b.Close = ev.Close
			s.barLast[minute.Unix()] = ev.End
		}
		oldVol := b.Volume
		b.Volume += ev.Volume
		if ev.VWAP > 0 && b.Volume > 0 {
			b.VWAP = (b.VWAP*oldVol + ev.VWAP*ev.Volume) / b.Volume
		}
	}
	if len(s.bars) > 3000 {
		s.bars = append([]data.Bar(nil), s.bars[len(s.bars)-3000:]...)
	}
	bt := ev.End.In(e.loc)
	if bt.Hour() >= 4 && (bt.Hour() < 9 || bt.Hour() == 9 && bt.Minute() < 30) {
		s.premarketVolume += ev.Volume
	}
	if bt.Hour() >= 9 && (bt.Hour() > 9 || bt.Minute() >= 30) && bt.Hour() < 16 {
		s.sessionVolume += ev.Volume
		if s.high == 0 || ev.High > s.high {
			s.high = ev.High
		}
		if s.low == 0 || ev.Low < s.low {
			s.low = ev.Low
		}
		s.pv += ev.VWAP * ev.Volume
		s.v += ev.Volume
	}
	if s.lastEvent.IsZero() || !ev.End.Before(s.lastEvent) {
		s.price = ev.Close
	}
	if ev.End.After(s.lastEvent) {
		s.lastEvent = ev.End
		e.lastEvent.Store(ev.End.UnixNano())
	}
	s.lastReceipt = ev.Received
	s.mu.Unlock()
	e.processed.Add(1)
	if e.onChange != nil {
		e.onChange(ev.Symbol, ev.End, ev.Received)
	}
}

func (e *Engine) Snapshot(symbol string, avgN int) (SymbolSnapshot, bool) {
	s := e.symbols[strings.ToUpper(symbol)]
	if s == nil {
		return SymbolSnapshot{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	o := SymbolSnapshot{Symbol: symbol, Bars: append([]data.Bar(nil), s.bars...), Price: s.price, SessionVolume: s.sessionVolume, PremarketVolume: s.premarketVolume, High: s.high, Low: s.low, LastEvent: s.lastEvent, LastReceipt: s.lastReceipt, Gap: s.gap.ID != 0, GapStart: s.gap.Start, GapEnd: s.gap.End}
	if s.v > 0 {
		o.VWAP = s.pv / s.v
	}
	if avgN < 1 {
		avgN = 15
	}
	// C/Avg is session-local: 04:00–20:00 America/New_York for the
	// provider event's trading date. Prior-session summaries are never eligible.
	sessionDay := s.lastEvent.In(e.loc).Format("2006-01-02")
	eligible := make([]data.Bar, 0, avgN)
	for i := len(s.bars) - 1; i >= 0 && len(eligible) < avgN; i-- {
		bt := s.bars[i].Time.In(e.loc)
		clock := bt.Hour()*60 + bt.Minute()
		if bt.Format("2006-01-02") == sessionDay && clock >= 4*60 && clock < 20*60 {
			eligible = append(eligible, s.bars[i])
		}
	}
	o.EligibleBarCount = len(eligible)
	o.SessionDate = sessionDay
	if len(eligible) > 0 {
		o.DevelopingMinute = eligible[0].Time
		o.NewestWindowMinute = eligible[0].Time
		if len(eligible) > 1 {
			o.LastCompletedMinute = eligible[1].Time
		}
		o.OldestWindowMinute = eligible[len(eligible)-1].Time
	}
	if len(eligible) >= avgN {
		sum := 0.0
		for _, b := range eligible[:avgN] {
			sum += b.Close
		}
		o.Average15 = sum / float64(avgN)
		o.Ratio = o.Price / o.Average15
		o.Warm = true
	}
	return o, true
}

func (e *Engine) Seed(symbol string, bars []data.Bar) {
	s := e.symbols[strings.ToUpper(symbol)]
	if s == nil {
		return
	}
	s.mu.Lock()
	byMinute := make(map[int64]data.Bar, len(bars)+len(s.bars))
	for _, b := range bars {
		byMinute[b.Time.UTC().Truncate(time.Minute).Unix()] = b
	}
	for _, b := range s.bars {
		byMinute[b.Time.UTC().Truncate(time.Minute).Unix()] = b
	}
	s.bars = s.bars[:0]
	for _, b := range byMinute {
		s.bars = append(s.bars, b)
	}
	sort.Slice(s.bars, func(i, j int) bool { return s.bars[i].Time.Before(s.bars[j].Time) })
	if len(s.bars) > 3000 {
		s.bars = s.bars[len(s.bars)-3000:]
	}
	s.mu.Unlock()
}

func (e *Engine) MarkAllGap(start, end time.Time) {
	for symbol := range e.symbols {
		e.MarkGap(symbol, start, end, "websocket_interruption")
	}
}
func (e *Engine) MarkGap(symbol string, start, end time.Time, cause string) GapState {
	s := e.symbols[strings.ToUpper(symbol)]
	if s == nil {
		return GapState{}
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gap.ID == 0 {
		s.gap = GapState{ID: e.nextGapID.Add(1), Version: 1, Start: start, End: end, Cause: cause, CreatedAt: now, LastExpanded: now, Status: "unresolved", Granularity: "minute"}
	} else {
		s.gap.Version++
		if start.Before(s.gap.Start) {
			s.gap.Start = start
		}
		if end.After(s.gap.End) {
			s.gap.End = end
		}
		s.gap.LastExpanded = now
		s.gap.Status = "unresolved"
		s.gap.NextAttempt = time.Time{}
	}
	return s.gap
}
func (e *Engine) GapState(symbol string) (GapState, bool) {
	s := e.symbols[strings.ToUpper(symbol)]
	if s == nil {
		return GapState{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gap, s.gap.ID != 0
}
func (e *Engine) HasGap(symbol string) bool { _, ok := e.GapState(symbol); return ok }

func (e *Engine) SymbolStatus(symbol string, required int) (SymbolStatus, bool) {
	s := e.symbols[strings.ToUpper(symbol)]
	if s == nil {
		return SymbolStatus{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := SymbolStatus{LastEvent: s.lastEvent, Gap: s.gap.ID != 0}
	if s.lastEvent.IsZero() {
		return status, true
	}
	status.Session = s.lastEvent.In(e.loc).Format("2006-01-02")
	eligible := 0
	for i := len(s.bars) - 1; i >= 0 && eligible < required; i-- {
		at := s.bars[i].Time.In(e.loc)
		clock := at.Hour()*60 + at.Minute()
		if at.Format("2006-01-02") == status.Session && clock >= 4*60 && clock < 20*60 {
			eligible++
		}
	}
	status.Warm = eligible >= required
	return status, true
}
func (e *Engine) BeginGapRecovery(symbol string) (RecoveryToken, bool) {
	s := e.symbols[strings.ToUpper(symbol)]
	if s == nil {
		return RecoveryToken{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gap.ID == 0 {
		return RecoveryToken{}, false
	}
	s.gap.Status = "recovering"
	s.gap.Attempts++
	return RecoveryToken{Symbol: symbol, GapID: s.gap.ID, Version: s.gap.Version, Start: s.gap.Start, End: s.gap.End}, true
}
func (e *Engine) AbortGapRecovery(token RecoveryToken, err error) {
	s := e.symbols[strings.ToUpper(token.Symbol)]
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gap.ID == token.GapID && s.gap.Version == token.Version {
		s.gap.Status = "failed"
		if err != nil {
			s.gap.LastError = err.Error()
		}
	}
}
func (e *Engine) ApplyAuthoritativeBars(token RecoveryToken, bars []data.Bar) error {
	// Compatibility entry point for callers recovering a known completed range.
	_, err := e.ApplyAuthoritativeBarsAt(token, bars, token.End.UTC().Truncate(time.Minute).Add(time.Minute))
	return err
}

// ApplyAuthoritativeBarsAt only finalizes minutes completed before now. A REST
// aggregate for the current minute has no coverage watermark, so it is ignored
// and the gap remains RECOVERING while newer WebSocket seconds keep merging.
func (e *Engine) ApplyAuthoritativeBarsAt(token RecoveryToken, bars []data.Bar, now time.Time) (bool, error) {
	s := e.symbols[strings.ToUpper(token.Symbol)]
	if s == nil {
		return false, fmt.Errorf("unknown symbol")
	}
	start, end := token.Start.UTC().Truncate(time.Minute), token.End.UTC().Truncate(time.Minute)
	developing := now.UTC().Truncate(time.Minute)
	recoveryEnd := end
	if !recoveryEnd.Before(developing) {
		recoveryEnd = developing.Add(-time.Minute)
	}
	if recoveryEnd.Before(start) {
		s.mu.Lock()
		if s.gap.ID != token.GapID || s.gap.Version != token.Version {
			s.mu.Unlock()
			return false, fmt.Errorf("stale recovery version")
		}
		s.gap.Status = "recovering"
		s.gap.NextAttempt = developing.Add(time.Minute)
		s.mu.Unlock()
		return false, ErrDevelopingGap
	}
	if len(bars) == 0 {
		return false, fmt.Errorf("empty authoritative backfill")
	}
	covered := map[int64]data.Bar{}
	for _, b := range bars {
		m := b.Time.UTC().Truncate(time.Minute)
		if m.Before(start) || m.After(recoveryEnd) {
			continue
		}
		covered[m.Unix()] = b
	}
	for m := start; !m.After(recoveryEnd); m = m.Add(time.Minute) {
		if _, ok := covered[m.Unix()]; !ok {
			return false, fmt.Errorf("partial backfill: missing %s", m.Format(time.RFC3339))
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gap.ID != token.GapID || s.gap.Version != token.Version {
		return false, fmt.Errorf("stale recovery version")
	}
	out := make([]data.Bar, 0, len(s.bars)+len(covered))
	for _, b := range s.bars {
		m := b.Time.UTC().Truncate(time.Minute)
		if m.Before(start) || m.After(recoveryEnd) {
			out = append(out, b)
		}
	}
	for _, b := range covered {
		b.Time = b.Time.UTC().Truncate(time.Minute)
		out = append(out, b)
		s.authoritative[b.Time.Unix()] = struct{}{}
		s.barLast[b.Time.Unix()] = token.End
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	s.bars = out
	e.recomputeDerivedLocked(s)
	if recoveryEnd.Before(end) {
		// Consume this recovery token. The unresolved developing suffix receives a
		// new version so this older operation can never clear it later.
		s.gap.Version++
		s.gap.Start = developing
		s.gap.Status = "recovering"
		s.gap.LastExpanded = now
		s.gap.NextAttempt = developing.Add(time.Minute)
		return false, nil
	}
	s.gap = GapState{}
	return true, nil
}

func (e *Engine) recomputeDerivedLocked(s *symbolState) {
	s.sessionVolume, s.premarketVolume, s.high, s.low, s.pv, s.v = 0, 0, 0, 0, 0, 0
	if len(s.bars) == 0 {
		s.price = 0
		return
	}
	session := s.lastEvent.In(e.loc).Format("2006-01-02")
	for _, b := range s.bars {
		at := b.Time.In(e.loc)
		if at.Format("2006-01-02") != session {
			continue
		}
		if at.Hour() >= 4 && (at.Hour() < 9 || at.Hour() == 9 && at.Minute() < 30) {
			s.premarketVolume += b.Volume
		}
		if at.Hour() >= 9 && (at.Hour() > 9 || at.Minute() >= 30) && at.Hour() < 16 {
			s.sessionVolume += b.Volume
			if s.high == 0 || b.High > s.high {
				s.high = b.High
			}
			if s.low == 0 || b.Low < s.low {
				s.low = b.Low
			}
			if b.VWAP > 0 {
				s.pv += b.VWAP * b.Volume
				s.v += b.Volume
			}
		}
	}
	s.price = s.bars[len(s.bars)-1].Close
}
func (e *Engine) ResolveGap(symbol string, bars []data.Bar) bool {
	token, ok := e.BeginGapRecovery(symbol)
	return ok && e.ApplyAuthoritativeBars(token, bars) == nil
}
func (e *Engine) GapCount() int {
	n := 0
	for _, s := range e.symbols {
		s.mu.RLock()
		if s.gap.ID != 0 {
			n++
		}
		s.mu.RUnlock()
	}
	return n
}

func (e *Engine) Metrics() Metrics {
	m := Metrics{Received: e.received.Load(), Processed: e.processed.Load(), Invalid: e.invalid.Load(), Duplicates: e.duplicates.Load(), OutOfOrder: e.outOfOrder.Load(), Dropped: e.dropped.Load(), QueueDepth: len(e.queue), QueueCapacity: cap(e.queue)}
	if n := e.lastReceive.Load(); n > 0 {
		m.LastReceive = time.Unix(0, n)
	}
	if n := e.lastEvent.Load(); n > 0 {
		m.LastEvent = time.Unix(0, n)
	}
	return m
}
