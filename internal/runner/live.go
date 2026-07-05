package runner

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"0945-playbook/internal/config"
	"0945-playbook/internal/dashboard"
	"0945-playbook/internal/data"
	"0945-playbook/internal/playbook"
	"0945-playbook/internal/watchlist"
)

type LiveRunner struct {
	project   string
	cfg       config.Config
	loc       *time.Location
	items     []watchlist.Item
	prov      data.Provider
	set       playbook.Settings
	chartDate string

	mu       sync.RWMutex
	rows     []playbook.Evaluation
	updated  time.Time
	scanning bool
}

func NewLive(project string, cfg config.Config, loc *time.Location, items []watchlist.Item, prov data.Provider) *LiveRunner {
	set := playbook.SettingsFromConfig(cfg)
	now := time.Now().In(loc)
	chartDate := now.Format("2006-01-02")
	set.ChartDate = chartDate
	set.ChartTime = chartClock(now)

	rows := make([]playbook.Evaluation, 0, len(items))
	for _, item := range items {
		rows = append(rows, playbook.Evaluation{
			Symbol:   item.Symbol,
			Name:     item.Name,
			Industry: item.Industry,
			Order:    item.Order,
			Status:   "WAIT DATA",
			Action:   "WAIT",
			Branch:   "-",
			Phase:    "wait",
			ChartURL: chartURL(cfg.Scan.ChartBaseURL, item.Symbol, chartDate, set.ChartTime, 0),
		})
	}
	return &LiveRunner{
		project:   project,
		cfg:       cfg,
		loc:       loc,
		items:     items,
		prov:      prov,
		set:       set,
		chartDate: chartDate,
		rows:      rows,
		updated:   time.Now(),
	}
}

func (r *LiveRunner) Run(ctx context.Context) {
	r.scan(ctx)
	ticker := time.NewTicker(config.Duration(r.cfg.Scan.PollInterval))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.scan(ctx)
		}
	}
}

func (r *LiveRunner) Snapshot(context.Context) dashboard.State {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now().In(r.loc)
	clock := now.Format("15:04:05")
	if r.scanning {
		clock += " scanning"
	}
	return dashboard.Build(r.project, "live", clock, r.cfg.Scan.ChartBaseURL, r.cfg.Scan.MinFirst15VolumeFilter, r.updated, r.rows)
}

func (r *LiveRunner) scan(ctx context.Context) {
	r.mu.Lock()
	if r.scanning {
		r.mu.Unlock()
		return
	}
	r.scanning = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.scanning = false
		r.mu.Unlock()
	}()

	now := time.Now().In(r.loc)
	set := r.set
	set.ChartTime = chartClock(now)
	start := sessionClock(now, r.loc, r.cfg.Session.Open)
	if now.Before(start) {
		start = start.Add(-15 * time.Minute)
	}

	rows := make([]playbook.Evaluation, len(r.items))
	sem := make(chan struct{}, r.cfg.Massive.ConcurrentRequests)
	var wg sync.WaitGroup
	for i, item := range r.items {
		i, item := i, item
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				rows[i] = errorEval(item, r.cfg, "scan canceled", r.chartDate, set.ChartTime)
				return
			}

			bars, err := r.prov.FetchBars(ctx, item.Symbol, start, now)
			if err != nil {
				rows[i] = errorEval(item, r.cfg, err.Error(), r.chartDate, set.ChartTime)
				return
			}
			rows[i] = playbook.Evaluate(item, bars, now, r.loc, set, nil)
		}()
	}
	wg.Wait()

	r.mu.Lock()
	r.rows = rows
	r.updated = time.Now().In(r.loc)
	r.mu.Unlock()
}

func errorEval(item watchlist.Item, cfg config.Config, msg string, chartDate string, chartTime string) playbook.Evaluation {
	return playbook.Evaluation{
		Symbol:   item.Symbol,
		Name:     item.Name,
		Industry: item.Industry,
		Order:    item.Order,
		Status:   "ERROR",
		Action:   "NONE",
		Branch:   "-",
		Phase:    "error",
		Reason:   msg,
		Error:    msg,
		ChartURL: chartURL(cfg.Scan.ChartBaseURL, item.Symbol, chartDate, chartTime, 0),
	}
}

func sessionClock(now time.Time, loc *time.Location, hhmm string) time.Time {
	var hour, minute int
	if _, err := fmt.Sscanf(hhmm, "%d:%d", &hour, &minute); err != nil {
		hour, minute = 9, 30
	}
	n := now.In(loc)
	return time.Date(n.Year(), n.Month(), n.Day(), hour, minute, 0, 0, loc)
}

func chartClock(t time.Time) string {
	if t.IsZero() {
		return "0945"
	}
	return t.Format("1504")
}

func chartURL(base, symbol, day string, clock string, side int) string {
	if base == "" {
		return ""
	}
	base = strings.TrimRight(base, "/")
	if day == "" {
		return base + "/?ticker=" + url.QueryEscape(symbol)
	}
	if clock == "" {
		clock = "0945"
	}
	signal := "buy"
	if side < 0 {
		signal = "sell"
	}
	return base + "/api/open-chart/" + url.PathEscape(symbol) + "/" + url.PathEscape(day) + "/" + url.PathEscape(clock) + "?resolution=1m&signal=" + url.QueryEscape(signal)
}
