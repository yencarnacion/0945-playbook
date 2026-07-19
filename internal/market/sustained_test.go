package market

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestSustainedOpeningFiveMinutes2000Symbols(t *testing.T) {
	if os.Getenv("PLAYBOOK_SUSTAINED") != "1" {
		t.Skip("set PLAYBOOK_SUSTAINED=1")
	}
	const (
		symbols   = 2000
		perSecond = 3
	)
	seconds := 300
	if raw := os.Getenv("PLAYBOOK_SUSTAINED_SECONDS"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			seconds = value
		}
	}
	names := make([]string, symbols)
	for i := range names {
		names[i] = fmt.Sprintf("S%04d", i)
	}
	// Production default: do not enlarge this queue to make the benchmark pass.
	e := New(names, time.UTC, 8192, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.Run(ctx)
	base := time.Date(2026, 7, 2, 13, 30, 0, 0, time.UTC)
	beforeG := runtime.NumGoroutine()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	pacer := time.NewTicker(time.Second / perSecond)
	defer pacer.Stop()
	for sec := 0; sec < seconds; sec++ {
		for pass := 0; pass < perSecond; pass++ {
			for i, s := range names {
				at := base.Add(time.Duration(sec)*time.Second + time.Duration(pass)*300*time.Millisecond)
				c := 10 + float64((i+sec)%100)/100
				if !e.Offer(Event{Symbol: s, Start: at, End: at.Add(250 * time.Millisecond), Open: c, High: c + .01, Low: c - .01, Close: c, Volume: 100, VWAP: c, Received: time.Now()}) {
					t.Fatalf("queue overflow at second %d", sec)
				}
			}
			<-pacer.C // fixed source rate; never inspect or throttle on queue depth
		}
	}
	want := uint64(symbols * seconds * perSecond)
	deadline := time.Now().Add(10 * time.Second)
	for e.Metrics().Processed < want && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	elapsed := time.Since(started)
	m := e.Metrics()
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if m.Processed != want || m.Dropped != 0 || m.QueueDepth != 0 {
		t.Fatalf("incomplete %+v want=%d", m, want)
	}
	if elapsed > time.Duration(seconds+10)*time.Second {
		t.Fatalf("sustained profile=%s", elapsed)
	}
	if runtime.NumGoroutine() > beforeG+3 {
		t.Fatalf("goroutine growth before=%d after=%d", beforeG, runtime.NumGoroutine())
	}
	t.Logf("events=%d elapsed=%s rate=%.0f/s heap_before=%d heap_after=%d total_alloc=%d goroutines=%d", want, elapsed, float64(want)/elapsed.Seconds(), before.HeapAlloc, after.HeapAlloc, after.TotalAlloc-before.TotalAlloc, runtime.NumGoroutine())
}
