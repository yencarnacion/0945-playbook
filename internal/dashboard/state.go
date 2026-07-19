package dashboard

import (
	"sort"
	"time"

	"0945-playbook/internal/playbook"
)

type Stats struct {
	Total   int `json:"total"`
	Likely  int `json:"likely"`
	Signals int `json:"signals"`
	Active  int `json:"active"`
	Done    int `json:"done"`
	NoTrade int `json:"no_trade"`
	Errors  int `json:"errors"`
	HighEV  int `json:"high_ev"`
}

type State struct {
	ProtocolVersion    int                   `json:"protocol_version"`
	Generation         uint64                `json:"generation"`
	PlaybookGeneration uint64                `json:"playbook_generation"`
	CAVGGeneration     uint64                `json:"cavg_generation"`
	KaneGeneration     uint64                `json:"kane_generation"`
	PublishedAt        string                `json:"published_at,omitempty"`
	Project            string                `json:"project"`
	Mode               string                `json:"mode"`
	Clock              string                `json:"clock"`
	Updated            string                `json:"updated"`
	ChartBaseURL       string                `json:"chart_base_url"`
	VolumeFilter       float64               `json:"volume_filter"`
	Stats              Stats                 `json:"stats"`
	Rows               []playbook.Evaluation `json:"rows"`
	Kane               KaneState             `json:"kane"`
	CAVG               ExtendedState         `json:"cavg"`
}

type LatencyHealth struct {
	Status                     string  `json:"status"`
	WebSocket                  any     `json:"websocket"`
	Market                     any     `json:"market"`
	CurrentEventAgeMS          float64 `json:"current_event_age_ms"`
	BackendPublicationAgeMS    float64 `json:"backend_publication_age_ms"`
	CAVGResultAgeMS            float64 `json:"cavg_result_age_ms"`
	KaneResultAgeMS            float64 `json:"kane_result_age_ms"`
	OriginalResultAgeMS        float64 `json:"original_result_age_ms"`
	P50MS, P95MS, P99MS, MaxMS float64
	StaleSymbols               int    `json:"stale_symbols"`
	SilentSymbols              int    `json:"silent_symbols"`
	Build                      string `json:"build"`
	Gaps                       int    `json:"gaps"`
	PlaybookGeneration         uint64 `json:"playbook_generation"`
	CAVGGeneration             uint64 `json:"cavg_generation"`
	KaneGeneration             uint64 `json:"kane_generation"`
	Browsers                   any    `json:"browsers"`
	PlaybookStatus             string `json:"playbook_status"`
	CAVGStatus                 string `json:"cavg_status"`
	KaneStatus                 string `json:"kane_status"`
}

type Delta struct {
	ProtocolVersion        int                   `json:"protocol_version"`
	Type                   string                `json:"type"`
	Reason                 string                `json:"reason,omitempty"`
	PlaybookGeneration     uint64                `json:"playbook_generation"`
	PlaybookBaseGeneration uint64                `json:"playbook_base_generation"`
	CAVGGeneration         uint64                `json:"cavg_generation"`
	CAVGBaseGeneration     uint64                `json:"cavg_base_generation"`
	KaneGeneration         uint64                `json:"kane_generation"`
	KaneBaseGeneration     uint64                `json:"kane_base_generation"`
	LatestMarketEvent      string                `json:"latest_market_event,omitempty"`
	Health                 string                `json:"health"`
	Generation             uint64                `json:"generation"`
	PublishedAt            string                `json:"published_at"`
	Rows                   []playbook.Evaluation `json:"rows"`
	Kane                   *KaneState            `json:"kane,omitempty"`
	CAVG                   *ExtendedState        `json:"cavg,omitempty"`
	Stats                  Stats                 `json:"stats"`
	Full                   *State                `json:"full,omitempty"`
}

type KaneRow struct {
	AlertEligible      bool    `json:"alert_eligible"`
	AlertID            string  `json:"alert_id,omitempty"`
	Health             string  `json:"health,omitempty"`
	EventAgeMS         float64 `json:"event_age_ms"`
	Symbol             string  `json:"symbol"`
	Name               string  `json:"name"`
	Industry           string  `json:"industry"`
	Setup              string  `json:"setup"`
	Rank               int     `json:"rank"`
	Preferred          bool    `json:"preferred"`
	Preliminary        bool    `json:"preliminary"`
	Price              float64 `json:"price"`
	PriorClose         float64 `json:"prior_close"`
	GapPct             float64 `json:"gap_pct"`
	GapATR             float64 `json:"gap_atr"`
	PriorCloseLocation float64 `json:"prior_close_location"`
	SampleEV           float64 `json:"sample_ev"`
	WinRate            float64 `json:"win_rate"`
	TargetPct          float64 `json:"target_pct"`
	StopPct            float64 `json:"stop_pct"`
	VolumeFrom0400     float64 `json:"volume_from_0400"`
	Reason             string  `json:"reason"`
	ChartURL           string  `json:"chart_url"`
}
type KaneState struct {
	Generation  uint64         `json:"generation"`
	PublishedAt string         `json:"published_at,omitempty"`
	Health      string         `json:"health,omitempty"`
	Available   bool           `json:"available"`
	Preliminary bool           `json:"preliminary"`
	Rows        []KaneRow      `json:"rows"`
	History     []KaneSnapshot `json:"history"`
}

type KaneSnapshot struct {
	Clock       string    `json:"clock"`
	Preliminary bool      `json:"preliminary"`
	Rows        []KaneRow `json:"rows"`
}

type ExtendedRow struct {
	AlertEligible      bool    `json:"alert_eligible"`
	AlertID            string  `json:"alert_id,omitempty"`
	MarketEventTime    string  `json:"market_event_time,omitempty"`
	EventAgeMS         float64 `json:"event_age_ms"`
	Health             string  `json:"health,omitempty"`
	Symbol             string  `json:"symbol"`
	Name               string  `json:"name"`
	Industry           string  `json:"industry"`
	Order              int     `json:"order"`
	Price              float64 `json:"price"`
	Average            float64 `json:"average"`
	Ratio              float64 `json:"ratio"`
	DeltaPct           float64 `json:"delta_pct"`
	ChangePct          float64 `json:"change_pct"`
	Volume             float64 `json:"volume"`
	Side               int     `json:"side"`
	Clock              string  `json:"clock"`
	ChartURL           string  `json:"chart_url"`
	OpenGapPct         float64 `json:"open_gap_pct"`
	GapATR             float64 `json:"gap_atr"`
	PriorCloseLocation float64 `json:"prior_close_location"`
	KaneFit            string  `json:"kane_fit"`
}

type ExtendedSnapshot struct {
	ID      int64         `json:"id"`
	Clock   string        `json:"clock"`
	Updated string        `json:"updated"`
	Rows    []ExtendedRow `json:"rows"`
}

type ExtendedHistoryPoint struct {
	ID    int64  `json:"id"`
	Clock string `json:"clock"`
	Count int    `json:"count"`
}

type ExtendedState struct {
	Generation       uint64                 `json:"generation"`
	PublishedAt      string                 `json:"published_at,omitempty"`
	Health           string                 `json:"health,omitempty"`
	Available        bool                   `json:"available"`
	WindowStart      string                 `json:"window_start"`
	WindowEnd        string                 `json:"window_end"`
	AvgCloseBars     int                    `json:"avg_close_bars"`
	UpperSignalRatio float64                `json:"upper_signal_ratio"`
	LowerSignalRatio float64                `json:"lower_signal_ratio"`
	SoundURL         string                 `json:"sound_url"`
	LiveID           int64                  `json:"live_id"`
	Selected         ExtendedSnapshot       `json:"selected"`
	History          []ExtendedHistoryPoint `json:"history"`
}

func Build(project, mode, clock, chartBaseURL string, volumeFilter float64, updated time.Time, rows []playbook.Evaluation) State {
	out := make([]playbook.Evaluation, len(rows))
	copy(out, rows)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Order < out[j].Order
	})

	stats := Stats{Total: len(out)}
	for _, row := range out {
		if !passesVolumeFilter(row, volumeFilter) {
			continue
		}
		rowHasError := row.Phase == "error" || row.Error != ""
		if row.Signal {
			stats.Signals++
		}
		switch row.Phase {
		case "likely":
			stats.Likely++
		case "active":
			stats.Active++
		case "done":
			stats.Done++
		case "error":
			stats.Errors++
		case "none":
			stats.NoTrade++
		}
		if rowHasError && row.Phase != "error" {
			stats.Errors++
		}
		if row.EVScore >= 80 {
			stats.HighEV++
		}
	}

	return State{
		Project:      project,
		Mode:         mode,
		Clock:        clock,
		Updated:      updated.Format(time.RFC3339),
		ChartBaseURL: chartBaseURL,
		VolumeFilter: volumeFilter,
		Stats:        stats,
		Rows:         out,
	}
}

func passesVolumeFilter(row playbook.Evaluation, volumeFilter float64) bool {
	if volumeFilter <= 0 {
		return true
	}
	if row.Phase == "error" || row.Error != "" {
		return true
	}
	return row.First15Vol >= volumeFilter
}
