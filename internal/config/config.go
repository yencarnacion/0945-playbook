package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App       AppConfig          `yaml:"app"`
	Massive   MassiveConfig      `yaml:"massive"`
	Scan      ScanConfig         `yaml:"scan"`
	Extended  ExtendedScanConfig `yaml:"extended_scan"`
	Session   SessionConfig      `yaml:"session"`
	Signal    SignalConfig       `yaml:"signal"`
	Risk      RiskConfig         `yaml:"risk"`
	ORB       ORBConfig          `yaml:"orb"`
	Branch    BranchConfig       `yaml:"branch"`
	Replay    ReplayConfig       `yaml:"replay"`
	Dashboard DashboardConfig    `yaml:"dashboard"`
}

type AppConfig struct {
	Name     string `yaml:"name"`
	Addr     string `yaml:"addr"`
	Timezone string `yaml:"timezone"`
}

type MassiveConfig struct {
	Mode                     string `yaml:"mode"`
	WebSocketURL             string `yaml:"websocket_url"`
	SubscriptionBatchSize    int    `yaml:"subscription_batch_size"`
	RESTMaxRequestsPerSecond int    `yaml:"rest_max_requests_per_second"`
	RESTMaxConcurrency       int    `yaml:"rest_max_concurrency"`
	RESTRetryLimit           int    `yaml:"rest_retry_limit"`
	ReconnectMinBackoff      string `yaml:"reconnect_min_backoff"`
	ReconnectMaxBackoff      string `yaml:"reconnect_max_backoff"`
	APIKeyEnv                string `yaml:"api_key_env"`
	SourceMultiplier         int    `yaml:"source_multiplier"`
	SourceTimespan           string `yaml:"source_timespan"`
	Adjusted                 bool   `yaml:"adjusted"`
	RequestTimeout           string `yaml:"request_timeout"`
	ConcurrentRequests       int    `yaml:"concurrent_requests"`
}

type ScanConfig struct {
	WatchlistPath          string  `yaml:"watchlist_path"`
	MaxSymbols             int     `yaml:"max_symbols"`
	MinFirst15VolumeFilter float64 `yaml:"min_first15_volume_filter"`
	PollInterval           string  `yaml:"poll_interval"`
	DataDir                string  `yaml:"data_dir"`
	ChartBaseURL           string  `yaml:"chart_base_url"`
	MaxEventAge            string  `yaml:"max_event_age"`
	PublishInterval        string  `yaml:"publish_interval"`
	QueueCapacity          int     `yaml:"queue_capacity"`
	WarmupRequired         bool    `yaml:"warmup_required"`
}

type DashboardConfig struct {
	Transport            string `yaml:"transport"`
	FullSnapshotInterval string `yaml:"full_snapshot_interval"`
}

type ExtendedScanConfig struct {
	Start            string  `yaml:"start"`
	End              string  `yaml:"end"`
	AvgCloseBars     int     `yaml:"avg_close_bars"`
	UpperSignalRatio float64 `yaml:"upper_signal_ratio"`
	LowerSignalRatio float64 `yaml:"lower_signal_ratio"`
	SoundDir         string  `yaml:"sound_dir"`
	SoundFile        string  `yaml:"sound_file"`
}

type SessionConfig struct {
	Open  string `yaml:"open"`
	Close string `yaml:"close"`
}

type SignalConfig struct {
	EntryMinutesAfterOpen int     `yaml:"entry_minutes_after_open"`
	AvgCloseBars          int     `yaml:"avg_close_bars"`
	MinEntryPrice         float64 `yaml:"min_entry_price"`
	MinFirst15Vol         float64 `yaml:"min_first15_vol"`
	MinFirst15DollarVol   float64 `yaml:"min_first15_dollar_vol"`
	UpperSignalRatio      float64 `yaml:"upper_signal_ratio"`
	LowerSignalRatio      float64 `yaml:"lower_signal_ratio"`
}

type RiskConfig struct {
	RiskDollars     float64 `yaml:"risk_dollars"`
	ShareLotSize    int     `yaml:"share_lot_size"`
	MaxShares       int     `yaml:"max_shares"`
	MinRiskPerShare float64 `yaml:"min_risk_per_share"`
	UseTimeExit     bool    `yaml:"use_time_exit"`
	TimeExitWindow  string  `yaml:"time_exit_window"`
}

type ORBConfig struct {
	LookbackSessions          int     `yaml:"lookback_sessions"`
	OrbMinutes                int     `yaml:"orb_minutes"`
	TargetStopMultiplier      float64 `yaml:"target_stop_multiplier"`
	RequireFullLookback       bool    `yaml:"require_full_lookback"`
	UseFirst15FallbackIfNoORB bool    `yaml:"use_first15_fallback_if_no_orb"`
}

type BranchConfig struct {
	ModerateUpperVWAPFade    ModerateUpperVWAPFadeConfig    `yaml:"moderate_upper_vwap_fade"`
	ExtremeUpperHardStopFade ExtremeUpperHardStopFadeConfig `yaml:"extreme_upper_hard_stop_fade"`
	MidPricedUpperLong       MidPricedUpperLongConfig       `yaml:"mid_priced_upper_long"`
	LowerHODStopShort        LowerHODStopShortConfig        `yaml:"lower_hod_stop_short"`
	LowerOneToOneORBFallback LowerOneToOneORBFallbackConfig `yaml:"lower_one_to_one_orb_fallback"`
}

type ModerateUpperVWAPFadeConfig struct {
	MinSignal        float64 `yaml:"min_signal"`
	MaxSignal        float64 `yaml:"max_signal"`
	MinVWAPRewardPct float64 `yaml:"min_vwap_reward_pct"`
	MinHODRiskPct    float64 `yaml:"min_hod_risk_pct"`
	MaxHODRiskPct    float64 `yaml:"max_hod_risk_pct"`
}

type ExtremeUpperHardStopFadeConfig struct {
	MinSignal float64 `yaml:"min_signal"`
	MinPrice  float64 `yaml:"min_price"`
	MaxPrice  float64 `yaml:"max_price"`
}

type MidPricedUpperLongConfig struct {
	MinPrice       float64 `yaml:"min_price"`
	MaxPrice       float64 `yaml:"max_price"`
	MinDistancePct float64 `yaml:"min_distance_pct"`
	MaxDistancePct float64 `yaml:"max_distance_pct"`
	MaxSignal      float64 `yaml:"max_signal"`
}

type LowerHODStopShortConfig struct {
	MinTargetPct        float64 `yaml:"min_target_pct"`
	MaxTargetPct        float64 `yaml:"max_target_pct"`
	MinHODRiskPct       float64 `yaml:"min_hod_risk_pct"`
	MaxHODRiskPct       float64 `yaml:"max_hod_risk_pct"`
	PrioritySignalRatio float64 `yaml:"priority_signal_ratio"`
}

type LowerOneToOneORBFallbackConfig struct {
	MinDistancePct float64 `yaml:"min_distance_pct"`
	MaxDistancePct float64 `yaml:"max_distance_pct"`
}

type ReplayConfig struct {
	DefaultDate  string  `yaml:"default_date"`
	DefaultStart string  `yaml:"default_start"`
	Speed        float64 `yaml:"speed"`
}

func Defaults() Config {
	return Config{
		App: AppConfig{
			Name:     "0945-playbook",
			Addr:     ":8080",
			Timezone: "America/New_York",
		},
		Massive: MassiveConfig{
			Mode: "websocket", SubscriptionBatchSize: 200,
			RESTMaxRequestsPerSecond: 80, RESTMaxConcurrency: 8, RESTRetryLimit: 3,
			ReconnectMinBackoff: "1s", ReconnectMaxBackoff: "30s",
			APIKeyEnv:          "MASSIVE_API_KEY",
			SourceMultiplier:   5,
			SourceTimespan:     "second",
			Adjusted:           true,
			RequestTimeout:     "20s",
			ConcurrentRequests: 8,
		},
		Scan: ScanConfig{
			WatchlistPath:          "1000-company-filter.csv",
			MaxSymbols:             0,
			MinFirst15VolumeFilter: 400000,
			PollInterval:           "10s",
			DataDir:                "data",
			ChartBaseURL:           "http://localhost:8081",
			MaxEventAge:            "10s", PublishInterval: "250ms", QueueCapacity: 8192, WarmupRequired: true,
		},
		Extended: ExtendedScanConfig{
			Start:            "04:00",
			End:              "20:00",
			AvgCloseBars:     15,
			UpperSignalRatio: 1.02,
			LowerSignalRatio: 0.98,
			SoundDir:         "sounds",
			SoundFile:        "hey.mp3",
		},
		Session: SessionConfig{
			Open:  "09:30",
			Close: "16:00",
		},
		Signal: SignalConfig{
			EntryMinutesAfterOpen: 15,
			AvgCloseBars:          15,
			MinEntryPrice:         1.00,
			MinFirst15Vol:         500000,
			MinFirst15DollarVol:   0,
			UpperSignalRatio:      1.02,
			LowerSignalRatio:      0.98,
		},
		Risk: RiskConfig{
			RiskDollars:     1000,
			ShareLotSize:    1,
			MaxShares:       1000000,
			MinRiskPerShare: 0.01,
			UseTimeExit:     true,
			TimeExitWindow:  "15:59-16:00",
		},
		ORB: ORBConfig{
			LookbackSessions:          5,
			OrbMinutes:                30,
			TargetStopMultiplier:      0.5,
			RequireFullLookback:       false,
			UseFirst15FallbackIfNoORB: true,
		},
		Branch: BranchConfig{
			ModerateUpperVWAPFade: ModerateUpperVWAPFadeConfig{
				MinSignal:        1.02,
				MaxSignal:        1.05,
				MinVWAPRewardPct: 0.02,
				MinHODRiskPct:    0.02,
				MaxHODRiskPct:    0.08,
			},
			ExtremeUpperHardStopFade: ExtremeUpperHardStopFadeConfig{
				MinSignal: 1.05,
				MinPrice:  5,
				MaxPrice:  100,
			},
			MidPricedUpperLong: MidPricedUpperLongConfig{
				MinPrice:       5,
				MaxPrice:       20,
				MinDistancePct: 0.03,
				MaxDistancePct: 0.08,
				MaxSignal:      1.05,
			},
			LowerHODStopShort: LowerHODStopShortConfig{
				MinTargetPct:        0.02,
				MaxTargetPct:        0.10,
				MinHODRiskPct:       0.08,
				MaxHODRiskPct:       0.16,
				PrioritySignalRatio: 0.96,
			},
			LowerOneToOneORBFallback: LowerOneToOneORBFallbackConfig{
				MinDistancePct: 0.05,
				MaxDistancePct: 0.12,
			},
		},
		Replay: ReplayConfig{
			DefaultStart: "09:30",
			Speed:        1,
		},
		Dashboard: DashboardConfig{Transport: "sse", FullSnapshotInterval: "5m"},
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	if path == "" {
		return cfg, nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.App.Timezone == "" {
		return fmt.Errorf("app.timezone is required")
	}
	if _, err := time.LoadLocation(c.App.Timezone); err != nil {
		return fmt.Errorf("load timezone %q: %w", c.App.Timezone, err)
	}
	if c.Massive.SourceMultiplier <= 0 {
		return fmt.Errorf("massive.source_multiplier must be positive")
	}
	if c.Massive.SourceTimespan == "" {
		return fmt.Errorf("massive.source_timespan is required")
	}
	if c.Massive.ConcurrentRequests <= 0 {
		return fmt.Errorf("massive.concurrent_requests must be positive")
	}
	if c.Massive.Mode != "websocket" && c.Massive.Mode != "rest" {
		return fmt.Errorf("massive.mode must be websocket or rest")
	}
	if c.Massive.SubscriptionBatchSize < 1 || c.Massive.SubscriptionBatchSize > 1000 {
		return fmt.Errorf("massive.subscription_batch_size must be 1..1000")
	}
	if c.Massive.RESTMaxRequestsPerSecond < 1 || c.Massive.RESTMaxRequestsPerSecond > 100 {
		return fmt.Errorf("massive.rest_max_requests_per_second must be 1..100")
	}
	if c.Massive.RESTMaxConcurrency < 1 || c.Massive.RESTMaxConcurrency > 64 {
		return fmt.Errorf("massive.rest_max_concurrency must be 1..64")
	}
	if c.Massive.RESTRetryLimit < 0 || c.Massive.RESTRetryLimit > 10 {
		return fmt.Errorf("massive.rest_retry_limit must be 0..10")
	}
	for name, value := range map[string]string{"massive.reconnect_min_backoff": c.Massive.ReconnectMinBackoff, "massive.reconnect_max_backoff": c.Massive.ReconnectMaxBackoff, "scan.max_event_age": c.Scan.MaxEventAge, "scan.publish_interval": c.Scan.PublishInterval, "dashboard.full_snapshot_interval": c.Dashboard.FullSnapshotInterval} {
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if c.Scan.QueueCapacity < 1 {
		return fmt.Errorf("scan.queue_capacity must be positive")
	}
	if c.Dashboard.Transport != "sse" && c.Dashboard.Transport != "polling" {
		return fmt.Errorf("dashboard.transport must be sse or polling")
	}
	if c.Scan.WatchlistPath == "" {
		return fmt.Errorf("scan.watchlist_path is required")
	}
	if c.Scan.MaxSymbols < 0 {
		return fmt.Errorf("scan.max_symbols must be zero or positive")
	}
	if c.Scan.MinFirst15VolumeFilter < 0 {
		return fmt.Errorf("scan.min_first15_volume_filter must be zero or positive")
	}
	if _, err := time.ParseDuration(c.Scan.PollInterval); err != nil {
		return fmt.Errorf("scan.poll_interval: %w", err)
	}
	if _, err := time.ParseDuration(c.Massive.RequestTimeout); err != nil {
		return fmt.Errorf("massive.request_timeout: %w", err)
	}
	extendedStart, err := time.Parse("15:04", c.Extended.Start)
	if err != nil {
		return fmt.Errorf("extended_scan.start: use HH:MM")
	}
	extendedEnd, err := time.Parse("15:04", c.Extended.End)
	if err != nil {
		return fmt.Errorf("extended_scan.end: use HH:MM")
	}
	if !extendedStart.Before(extendedEnd) {
		return fmt.Errorf("extended_scan.start must be before extended_scan.end")
	}
	if c.Extended.AvgCloseBars < 1 {
		return fmt.Errorf("extended_scan.avg_close_bars must be positive")
	}
	if c.Extended.LowerSignalRatio <= 0 || c.Extended.UpperSignalRatio <= c.Extended.LowerSignalRatio {
		return fmt.Errorf("extended_scan signal ratios are invalid")
	}
	if strings.TrimSpace(c.Extended.SoundDir) == "" || strings.TrimSpace(c.Extended.SoundFile) == "" {
		return fmt.Errorf("extended_scan.sound_dir and sound_file are required")
	}
	if c.Signal.EntryMinutesAfterOpen < 1 {
		return fmt.Errorf("signal.entry_minutes_after_open must be positive")
	}
	if c.Signal.AvgCloseBars < 1 {
		return fmt.Errorf("signal.avg_close_bars must be positive")
	}
	if c.Risk.ShareLotSize < 1 {
		return fmt.Errorf("risk.share_lot_size must be positive")
	}
	if c.Risk.MaxShares < 1 {
		return fmt.Errorf("risk.max_shares must be positive")
	}
	if c.ORB.LookbackSessions < 1 {
		return fmt.Errorf("orb.lookback_sessions must be positive")
	}
	if c.ORB.OrbMinutes < 1 {
		return fmt.Errorf("orb.orb_minutes must be positive")
	}
	if c.Replay.Speed <= 0 {
		return fmt.Errorf("replay.speed must be positive")
	}
	return nil
}

func LoadDotEnv(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		if err := os.Setenv(key, val); err != nil {
			return err
		}
	}
	return nil
}

func Duration(value string) time.Duration {
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0
	}
	return d
}

func EnvInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}
