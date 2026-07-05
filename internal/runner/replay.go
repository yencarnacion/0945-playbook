package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"0945-playbook/internal/config"
	"0945-playbook/internal/dashboard"
	"0945-playbook/internal/data"
	"0945-playbook/internal/playbook"
	"0945-playbook/internal/watchlist"
)

type ReplayRunner struct {
	project      string
	cfg          config.Config
	loc          *time.Location
	items        []watchlist.Item
	barsBySymbol map[string][]data.Bar
	errors       map[string]string
	set          playbook.Settings
	chartDate    string
	selectedTime time.Time
	startVirtual time.Time
	startReal    time.Time
	speed        float64
	mode         string
	controlled   bool
	mu           sync.RWMutex
}

func NewReplay(project, mode string, cfg config.Config, loc *time.Location, items []watchlist.Item, day string, start string, speed float64) (*ReplayRunner, error) {
	startVirtual, err := parseDayClock(day, start, loc)
	if err != nil {
		return nil, err
	}
	set := playbook.SettingsFromConfig(cfg)
	set.ChartDate = day
	set.ChartTime = chartClock(startVirtual)

	r := &ReplayRunner{
		project:      project,
		cfg:          cfg,
		loc:          loc,
		items:        items,
		barsBySymbol: make(map[string][]data.Bar, len(items)),
		errors:       make(map[string]string),
		set:          set,
		chartDate:    day,
		selectedTime: startVirtual,
		startVirtual: startVirtual,
		startReal:    time.Now(),
		speed:        speed,
		mode:         mode,
		controlled:   true,
	}

	for _, item := range items {
		bars, err := data.LoadBars(cfg.Scan.DataDir, day, item.Symbol)
		if err != nil {
			r.errors[item.Symbol] = data.MissingCacheError(day, item.Symbol).Error()
			continue
		}
		r.barsBySymbol[item.Symbol] = bars
	}
	return r, nil
}

func NewDemo(project string, cfg config.Config, loc *time.Location, items []watchlist.Item) (*ReplayRunner, error) {
	now := time.Now().In(loc)
	day := now.Format("2006-01-02")
	startVirtual, err := parseDayClock(day, "09:38", loc)
	if err != nil {
		return nil, err
	}
	prov := data.DemoProvider{Loc: loc}
	set := playbook.SettingsFromConfig(cfg)
	set.ChartDate = day
	set.ChartTime = chartClock(startVirtual)

	r := &ReplayRunner{
		project:      project,
		cfg:          cfg,
		loc:          loc,
		items:        items,
		barsBySymbol: make(map[string][]data.Bar, len(items)),
		errors:       make(map[string]string),
		set:          set,
		chartDate:    day,
		selectedTime: startVirtual,
		startVirtual: startVirtual,
		startReal:    time.Now(),
		speed:        cfg.Replay.Speed,
		mode:         "demo",
	}
	end := sessionClock(now, loc, cfg.Session.Close)
	for _, item := range items {
		bars, err := prov.FetchBars(context.Background(), item.Symbol, startVirtual, end)
		if err != nil {
			r.errors[item.Symbol] = err.Error()
			continue
		}
		r.barsBySymbol[item.Symbol] = bars
	}
	return r, nil
}

func (r *ReplayRunner) Snapshot(context.Context) dashboard.State {
	now := r.currentTime()
	return r.snapshotAt(now)
}

func (r *ReplayRunner) SeekReplay(ctx context.Context, clock string) (dashboard.State, error) {
	next, err := parseDayClock(r.chartDate, clock, r.loc)
	if err != nil {
		return dashboard.State{}, err
	}
	r.mu.Lock()
	r.selectedTime = r.clampReplayTime(next)
	r.mu.Unlock()
	return r.Snapshot(ctx), nil
}

func (r *ReplayRunner) StepReplay(ctx context.Context, minutes int) (dashboard.State, error) {
	if minutes == 0 {
		return r.Snapshot(ctx), nil
	}
	r.mu.Lock()
	now := r.selectedTime
	if now.IsZero() {
		now = r.startVirtual
	}
	r.selectedTime = r.clampReplayTime(now.Add(time.Duration(minutes) * time.Minute))
	r.mu.Unlock()
	return r.Snapshot(ctx), nil
}

func (r *ReplayRunner) currentTime() time.Time {
	r.mu.RLock()
	if r.controlled {
		now := r.selectedTime
		r.mu.RUnlock()
		if now.IsZero() {
			return r.startVirtual
		}
		return now
	}
	r.mu.RUnlock()
	return r.virtualNow()
}

func (r *ReplayRunner) snapshotAt(now time.Time) dashboard.State {
	set := r.set
	set.ChartTime = chartClock(now)
	rows := make([]playbook.Evaluation, 0, len(r.items))
	for _, item := range r.items {
		if msg := r.errors[item.Symbol]; msg != "" {
			rows = append(rows, errorEval(item, r.cfg, msg, r.chartDate, set.ChartTime))
			continue
		}
		rows = append(rows, playbook.Evaluate(item, r.barsBySymbol[item.Symbol], now, r.loc, set, nil))
	}
	return dashboard.Build(r.project, r.mode, now.In(r.loc).Format("15:04:05"), r.cfg.Scan.ChartBaseURL, r.cfg.Scan.MinFirst15VolumeFilter, time.Now().In(r.loc), rows)
}

func (r *ReplayRunner) virtualNow() time.Time {
	elapsed := time.Since(r.startReal).Seconds() * r.speed
	return r.startVirtual.Add(time.Duration(elapsed * float64(time.Second)))
}

func (r *ReplayRunner) clampReplayTime(t time.Time) time.Time {
	open := sessionClock(t, r.loc, r.cfg.Session.Open)
	closeTime := sessionClock(t, r.loc, r.cfg.Session.Close)
	if t.Before(open) {
		return open
	}
	if t.After(closeTime) {
		return closeTime
	}
	return t
}

func parseDayClock(day, clock string, loc *time.Location) (time.Time, error) {
	if clock == "" {
		clock = "09:30"
	}
	clock = strings.TrimSpace(clock)
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02 15:04:05"} {
		t, err := time.ParseInLocation(layout, day+" "+clock, loc)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse replay clock %q: use HH:MM or HH:MM:SS", clock)
}
