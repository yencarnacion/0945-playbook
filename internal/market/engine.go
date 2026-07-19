package market

import (
	"context"
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

type symbolState struct {
	mu                                                      sync.RWMutex
	bars                                                    []data.Bar
	barLast                                                 map[int64]time.Time
	seen                                                    map[string]struct{}
	seenOrder                                               []string
	lastEvent, lastReceipt                                  time.Time
	price, sessionVolume, premarketVolume, high, low, pv, v float64
	gap                                                     bool
	gapStart, gapEnd                                        time.Time
}

type Engine struct {
	loc                                                           *time.Location
	queue                                                         chan Event
	symbols                                                       map[string]*symbolState
	received, processed, invalid, duplicates, outOfOrder, dropped atomic.Uint64
	lastReceive, lastEvent                                        atomic.Int64
	onChange                                                      func(string, time.Time, time.Time)
}

func New(symbols []string, loc *time.Location, capacity int, onChange func(string, time.Time, time.Time)) *Engine {
	if capacity < 1 {
		capacity = 8192
	}
	e := &Engine{loc: loc, queue: make(chan Event, capacity), symbols: make(map[string]*symbolState, len(symbols)), onChange: onChange}
	for _, s := range symbols {
		s = strings.ToUpper(strings.TrimSpace(s))
		if s != "" {
			e.symbols[s] = &symbolState{barLast: make(map[int64]time.Time), seen: make(map[string]struct{}, 64)}
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
		if s := e.symbols[strings.ToUpper(ev.Symbol)]; s != nil {
			s.mu.Lock()
			s.gap = true
			if s.gapStart.IsZero() || ev.Start.Before(s.gapStart) {
				s.gapStart = ev.Start
			}
			if ev.End.After(s.gapEnd) {
				s.gapEnd = ev.End
			}
			s.mu.Unlock()
		}
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
	key := ev.Symbol + "/" + ev.Start.UTC().Format(time.RFC3339Nano) + "/" + ev.End.UTC().Format(time.RFC3339Nano)
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
	o := SymbolSnapshot{Symbol: symbol, Bars: append([]data.Bar(nil), s.bars...), Price: s.price, SessionVolume: s.sessionVolume, PremarketVolume: s.premarketVolume, High: s.high, Low: s.low, LastEvent: s.lastEvent, LastReceipt: s.lastReceipt, Gap: s.gap, GapStart: s.gapStart, GapEnd: s.gapEnd}
	if s.v > 0 {
		o.VWAP = s.pv / s.v
	}
	if avgN < 1 {
		avgN = 15
	}
	if len(s.bars) >= avgN {
		sum := 0.0
		for _, b := range s.bars[len(s.bars)-avgN:] {
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
	for _, s := range e.symbols {
		s.mu.Lock()
		s.gap = true
		if s.gapStart.IsZero() || start.Before(s.gapStart) {
			s.gapStart = start
		}
		if end.After(s.gapEnd) {
			s.gapEnd = end
		}
		s.mu.Unlock()
	}
}
func (e *Engine) ResolveGap(symbol string, bars []data.Bar) bool {
	s := e.symbols[strings.ToUpper(symbol)]
	if s == nil || len(bars) == 0 {
		return false
	}
	e.Seed(symbol, bars)
	s.mu.Lock()
	s.gap = false
	s.gapStart = time.Time{}
	s.gapEnd = time.Time{}
	s.mu.Unlock()
	return true
}
func (e *Engine) GapCount() int {
	n := 0
	for _, s := range e.symbols {
		s.mu.RLock()
		if s.gap {
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
