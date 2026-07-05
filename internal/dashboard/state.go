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
