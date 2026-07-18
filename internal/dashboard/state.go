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
	Project      string                `json:"project"`
	Mode         string                `json:"mode"`
	Clock        string                `json:"clock"`
	Updated      string                `json:"updated"`
	ChartBaseURL string                `json:"chart_base_url"`
	VolumeFilter float64               `json:"volume_filter"`
	Stats        Stats                 `json:"stats"`
	Rows         []playbook.Evaluation `json:"rows"`
	Kane         KaneState             `json:"kane"`
}

type KaneRow struct {
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
