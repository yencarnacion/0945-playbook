package runner

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
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

	mu              sync.RWMutex
	rows            []playbook.Evaluation
	barsBySymbol    map[string][]data.Bar
	updated         time.Time
	scanning        bool
	extendedStarted time.Time
	extendedHistory []dashboard.ExtendedSnapshot
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
		project:         project,
		cfg:             cfg,
		loc:             loc,
		items:           items,
		prov:            prov,
		set:             set,
		chartDate:       chartDate,
		rows:            rows,
		barsBySymbol:    make(map[string][]data.Bar, len(items)),
		updated:         time.Now(),
		extendedStarted: now,
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
	state := dashboard.Build(r.project, "live", clock, r.cfg.Scan.ChartBaseURL, r.cfg.Scan.MinFirst15VolumeFilter, r.updated, r.rows)
	state.Kane = kaneState(r.items, r.barsBySymbol, now, r.loc, r.cfg.Scan.ChartBaseURL)
	return state
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
	chartDate := now.Format("2006-01-02")
	set := r.set
	set.ChartDate = chartDate
	set.ChartTime = chartClock(now)
	open := sessionClock(now, r.loc, r.cfg.Session.Open)
	extendedOpen := sessionClock(now, r.loc, r.cfg.Extended.Start)
	extendedClose := sessionClock(now, r.loc, r.cfg.Extended.End)

	r.mu.Lock()
	if chartDate != r.chartDate {
		r.chartDate = chartDate
		r.barsBySymbol = make(map[string][]data.Bar, len(r.items))
		r.extendedStarted = now
		r.extendedHistory = nil
	}
	cachedBySymbol := cloneBarsBySymbol(r.barsBySymbol)
	extendedStarted := r.extendedStarted
	r.mu.Unlock()

	if now.Before(extendedOpen) {
		rows := make([]playbook.Evaluation, len(r.items))
		for i, item := range r.items {
			rows[i] = waitEval(item, r.cfg, chartDate, set.ChartTime, "WAIT 09:30", "Regular session has not opened.")
		}
		r.mu.Lock()
		r.rows = rows
		r.updated = time.Now().In(r.loc)
		r.mu.Unlock()
		return
	}
	fetchEnd := now
	if fetchEnd.After(extendedClose) {
		fetchEnd = extendedClose
	}
	// Keep enough history to locate the previous trading day's 16:00 close,
	// including across weekends and market holidays. Today's 04:00 bars are also
	// needed when the process is started after the opening bell.
	fetchBase := extendedOpen.AddDate(0, 0, -7)

	rows := make([]playbook.Evaluation, len(r.items))
	mergedBars := make([][]data.Bar, len(r.items))
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
				rows[i] = errorEval(item, r.cfg, "scan canceled", chartDate, set.ChartTime)
				return
			}

			cached := cachedBySymbol[item.Symbol]
			fetchStart := liveFetchStart(fetchEnd, fetchBase, cached)
			bars, err := r.prov.FetchBars(ctx, item.Symbol, fetchStart, fetchEnd)
			if err != nil {
				if now.Before(open) {
					rows[i] = waitEval(item, r.cfg, chartDate, set.ChartTime, "WAIT 09:30", "Extended scan data error: "+err.Error())
				} else {
					rows[i] = errorEval(item, r.cfg, err.Error(), chartDate, set.ChartTime)
				}
				return
			}
			merged := mergeSessionBars(cached, bars, fetchBase, extendedClose, r.loc)
			mergedBars[i] = merged
			if now.Before(open) {
				rows[i] = waitEval(item, r.cfg, chartDate, set.ChartTime, "WAIT 09:30", "Regular session has not opened.")
			} else {
				rows[i] = playbook.Evaluate(item, merged, now, r.loc, set, nil)
			}
		}()
	}
	wg.Wait()

	r.mu.Lock()
	for i, item := range r.items {
		if len(mergedBars[i]) > 0 {
			r.barsBySymbol[item.Symbol] = mergedBars[i]
		}
	}
	r.recordExtendedSnapshots(now, extendedStarted)
	r.rows = rows
	r.updated = time.Now().In(r.loc)
	r.mu.Unlock()
}

func waitEval(item watchlist.Item, cfg config.Config, chartDate string, chartTime string, status string, reason string) playbook.Evaluation {
	return playbook.Evaluation{
		Symbol:   item.Symbol,
		Name:     item.Name,
		Industry: item.Industry,
		Order:    item.Order,
		Status:   status,
		Action:   "WAIT",
		Branch:   "-",
		Phase:    "wait",
		Reason:   reason,
		ChartURL: chartURL(cfg.Scan.ChartBaseURL, item.Symbol, chartDate, chartTime, 0),
	}
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

func cloneBarsBySymbol(src map[string][]data.Bar) map[string][]data.Bar {
	out := make(map[string][]data.Bar, len(src))
	for symbol, bars := range src {
		if len(bars) == 0 {
			continue
		}
		out[symbol] = append([]data.Bar(nil), bars...)
	}
	return out
}

func liveFetchStart(now time.Time, base time.Time, cached []data.Bar) time.Time {
	if len(cached) == 0 {
		return base
	}
	last := cached[0].Time
	for _, bar := range cached[1:] {
		if bar.Time.After(last) {
			last = bar.Time
		}
	}
	start := last.In(base.Location()).Truncate(time.Minute).Add(-time.Minute)
	if start.Before(base) || start.After(now) {
		return base
	}
	return start
}

func mergeRTHBars(existing []data.Bar, incoming []data.Bar, open time.Time, closeTime time.Time, loc *time.Location) []data.Bar {
	return mergeSessionBars(existing, incoming, open, closeTime, loc)
}

func mergeSessionBars(existing []data.Bar, incoming []data.Bar, open time.Time, closeTime time.Time, loc *time.Location) []data.Bar {
	byMinute := make(map[time.Time]data.Bar, len(existing)+len(incoming))
	add := func(bar data.Bar) {
		bt := bar.Time.In(loc)
		if bt.Before(open) || !bt.Before(closeTime) {
			return
		}
		minute := bt.Truncate(time.Minute)
		key := time.Date(minute.Year(), minute.Month(), minute.Day(), minute.Hour(), minute.Minute(), 0, 0, loc).UTC()
		bar.Time = key
		byMinute[key] = bar
	}
	for _, bar := range existing {
		add(bar)
	}
	for _, bar := range incoming {
		add(bar)
	}

	out := make([]data.Bar, 0, len(byMinute))
	for _, bar := range byMinute {
		out = append(out, bar)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Time.Before(out[j].Time)
	})
	return out
}

func (r *LiveRunner) AlertSoundPath() string {
	return filepath.Join(r.cfg.Extended.SoundDir, r.cfg.Extended.SoundFile)
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
