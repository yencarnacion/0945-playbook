package server

import (
	"0945-playbook/internal/dashboard"
	"0945-playbook/internal/playbook"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

type browserProvider struct {
	mu       sync.RWMutex
	s        dashboard.State
	h        *Hub
	interval time.Duration
}

func TestChromiumCAVGAndKaneRender(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("loopback unavailable: %v", err)
	}
	eventAt := time.Now()
	crows := make([]dashboard.ExtendedRow, 2000)
	krows := make([]dashboard.KaneRow, 2000)
	for i := range crows {
		sym := fmt.Sprintf("S%04d", i)
		crows[i] = dashboard.ExtendedRow{Symbol: sym, Order: i, Price: 10, Average: 9, Ratio: 1.11, Health: "READY", MarketEventTime: eventAt.Format(time.RFC3339Nano)}
		krows[i] = dashboard.KaneRow{Symbol: sym, Rank: i + 1, Setup: "Gap-up continuation", Price: 10, VolumeFrom0400: 200000}
	}
	state := dashboard.State{ProtocolVersion: 1, Project: "test", Mode: "live", PlaybookGeneration: 1, CAVGGeneration: 1, KaneGeneration: 1, CAVG: dashboard.ExtendedState{Generation: 1, Available: true, Selected: dashboard.ExtendedSnapshot{Rows: crows}}, Kane: dashboard.KaneState{Generation: 1, Available: true, Rows: krows}}
	p := &browserProvider{h: NewHub(8), s: state}
	srv := httptest.NewUnstartedServer(NewHandler(p))
	srv.Listener = ln
	srv.Start()
	defer srv.Close()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath("/usr/bin/google-chrome"), chromedp.Flag("headless", true), chromedp.Flag("no-sandbox", true), chromedp.Flag("disable-gpu", true))
	alloc, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	browser, cancelBrowser := chromedp.NewContext(alloc)
	defer cancelBrowser()
	browser, timeout := context.WithTimeout(browser, 10*time.Second)
	defer timeout()
	var ccount, kcount int64
	err = chromedp.Run(browser, chromedp.Navigate(srv.URL), chromedp.Click(`button[data-view="extended"]`, chromedp.ByQuery), chromedp.Poll(`document.querySelectorAll('#extendedRows tr').length === 2000`, nil), chromedp.Evaluate(`document.querySelectorAll('#extendedRows tr').length`, &ccount), chromedp.Click(`button[data-view="kane"]`, chromedp.ByQuery), chromedp.Poll(`document.querySelectorAll('#kaneRows tr').length === 2000`, nil), chromedp.Evaluate(`document.querySelectorAll('#kaneRows tr').length`, &kcount))
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(eventAt)
	if ccount != 2000 || kcount != 2000 || elapsed > 10*time.Second {
		t.Fatalf("cavg=%d kane=%d latency=%s", ccount, kcount, elapsed)
	}
	b, _ := json.Marshal(state)
	t.Logf("C/Avg and Kane 2000-row render=%s full_snapshot_bytes=%d", elapsed, len(b))
}

func (p *browserProvider) Snapshot(context.Context) dashboard.State {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.s
}
func (p *browserProvider) Hub() *Hub { return p.h }
func (p *browserProvider) FullSnapshotInterval() time.Duration {
	if p.interval > 0 {
		return p.interval
	}
	return time.Hour
}
func TestPeriodicFullSnapshot(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("loopback unavailable: %v", err)
	}
	p := &browserProvider{h: NewHub(2), interval: 20 * time.Millisecond, s: dashboard.State{ProtocolVersion: 1, Mode: "live"}}
	srv := httptest.NewUnstartedServer(NewHandler(p))
	srv.Listener = ln
	srv.Start()
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	scan := bufio.NewScanner(resp.Body)
	count := 0
	deadline := time.After(time.Second)
	for count < 2 {
		select {
		case <-deadline:
			t.Fatal("periodic full snapshot timeout")
		default:
			if !scan.Scan() {
				t.Fatal("stream ended")
			}
			if strings.Contains(scan.Text(), `"type":"full"`) {
				count++
			}
		}
	}
}
func TestChromiumEventToDOMRender(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("loopback unavailable: %v", err)
	}
	p := &browserProvider{h: NewHub(8), s: dashboard.State{ProtocolVersion: 1, Project: "test", Mode: "live", Rows: []playbook.Evaluation{}}}
	srv := httptest.NewUnstartedServer(NewHandler(p))
	srv.Listener = ln
	srv.Start()
	defer srv.Close()
	eventAt := time.Now()
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for p.h.Stats().Connected == 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		p.mu.Lock()
		p.s.PlaybookGeneration = 1
		p.s.Generation = 1
		rows := make([]playbook.Evaluation, 2000)
		for i := range rows {
			symbol := fmt.Sprintf("S%04d", i)
			if i == 0 {
				symbol = "E2ETEST"
			}
			rows[i] = playbook.Evaluation{Symbol: symbol, Order: i + 1, Status: "READY", Phase: "likely", Health: "READY", MarketEventTime: eventAt.Format(time.RFC3339Nano)}
		}
		p.s.Rows = rows
		p.mu.Unlock()
		p.h.Publish(dashboard.Delta{ProtocolVersion: 1, Type: "delta", Generation: 1, PlaybookGeneration: 1, PlaybookBaseGeneration: 0, Rows: p.s.Rows, PublishedAt: time.Now().Format(time.RFC3339Nano)})
	}()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath("/usr/bin/google-chrome"), chromedp.Flag("headless", true), chromedp.Flag("no-sandbox", true), chromedp.Flag("disable-gpu", true))
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()
	browser, cancelBrowser := chromedp.NewContext(alloc)
	defer cancelBrowser()
	browser, timeout := context.WithTimeout(browser, 10*time.Second)
	defer timeout()
	var symbol string
	var renderedMS float64
	var debug string
	err = chromedp.Run(browser, chromedp.Navigate(srv.URL), chromedp.Sleep(2*time.Second), chromedp.Evaluate(`JSON.stringify({gen:document.body.dataset.playbookGeneration,text:document.body.innerText.slice(0,2000)})`, &debug), chromedp.Evaluate(`document.querySelector('tr[data-key="E2ETEST"] .sym a')?.textContent || ''`, &symbol), chromedp.Evaluate(`Number(document.body.dataset.lastRender)`, &renderedMS))
	if err != nil {
		t.Fatalf("%v debug=%s", err, debug)
	}
	if symbol != "E2ETEST" {
		t.Fatalf("rendered symbol=%q debug=%s", symbol, debug)
	}
	elapsed := time.UnixMilli(int64(renderedMS)).Sub(eventAt)
	if elapsed > 10*time.Second {
		t.Fatalf("event-to-DOM %s exceeds SLO", elapsed)
	}
	t.Logf("event-to-DOM including Chrome startup: %s", elapsed)
}
