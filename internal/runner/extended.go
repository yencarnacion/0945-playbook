package runner

import (
	"context"
	"sort"
	"time"

	"0945-playbook/internal/dashboard"
)

func (r *LiveRunner) recordExtendedSnapshots(now time.Time, started time.Time) {
	windowStart := sessionClock(now, r.loc, r.cfg.Extended.Start)
	windowEnd := sessionClock(now, r.loc, r.cfg.Extended.End)
	dataStart := started.In(r.loc).Truncate(time.Minute)
	if dataStart.Before(windowStart) {
		dataStart = windowStart
	}
	cutoff := now.In(r.loc).Truncate(time.Minute)
	if !cutoff.Before(windowEnd) {
		cutoff = windowEnd.Add(-time.Minute)
	}
	if cutoff.Before(dataStart) {
		return
	}

	first := dataStart
	if len(r.extendedHistory) > 0 {
		first = cutoff.Add(-time.Minute)
		last := time.Unix(r.extendedHistory[len(r.extendedHistory)-1].ID*60, 0).In(r.loc)
		if next := last.Add(time.Minute); next.Before(first) {
			first = next
		}
	}
	if first.Before(dataStart) {
		first = dataStart
	}
	for minute := first; !minute.After(cutoff); minute = minute.Add(time.Minute) {
		snapshot := dashboard.ExtendedSnapshot{
			ID:      minute.Unix() / 60,
			Clock:   minute.Format("15:04"),
			Updated: now.Format(time.RFC3339),
			Rows:    r.extendedRowsAt(minute, dataStart),
		}
		r.upsertExtendedSnapshot(snapshot)
	}
}

func (r *LiveRunner) extendedRowsAt(cutoff time.Time, dataStart time.Time) []dashboard.ExtendedRow {
	rows := make([]dashboard.ExtendedRow, 0)
	for _, item := range r.items {
		bars := r.barsBySymbol[item.Symbol]
		last := sort.Search(len(bars), func(i int) bool {
			return bars[i].Time.In(r.loc).After(cutoff)
		}) - 1
		if last < 0 {
			continue
		}
		closes := make([]float64, 0, r.cfg.Extended.AvgCloseBars)
		for i := last; i >= 0 && len(closes) < r.cfg.Extended.AvgCloseBars; i-- {
			bt := bars[i].Time.In(r.loc)
			if bt.Before(dataStart) {
				break
			}
			if bars[i].Close > 0 {
				closes = append(closes, bars[i].Close)
			}
		}
		if len(closes) < r.cfg.Extended.AvgCloseBars {
			continue
		}
		sum := 0.0
		for _, close := range closes {
			sum += close
		}
		average := sum / float64(len(closes))
		price := bars[last].Close
		if average <= 0 || price <= 0 {
			continue
		}
		ratio := price / average
		if ratio >= r.cfg.Extended.LowerSignalRatio && ratio <= r.cfg.Extended.UpperSignalRatio {
			continue
		}
		side := 1
		if ratio < 1 {
			side = -1
		}
		rows = append(rows, dashboard.ExtendedRow{
			Symbol:   item.Symbol,
			Name:     item.Name,
			Industry: item.Industry,
			Order:    item.Order,
			Price:    price,
			Average:  average,
			Ratio:    ratio,
			DeltaPct: ratio - 1,
			Side:     side,
			Clock:    cutoff.Format("15:04"),
			ChartURL: chartURL(r.cfg.Scan.ChartBaseURL, item.Symbol, cutoff.Format("2006-01-02"), cutoff.Format("1504"), side),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		iDelta := abs(rows[i].DeltaPct)
		jDelta := abs(rows[j].DeltaPct)
		if iDelta == jDelta {
			return rows[i].Order < rows[j].Order
		}
		return iDelta > jDelta
	})
	return rows
}

func (r *LiveRunner) upsertExtendedSnapshot(snapshot dashboard.ExtendedSnapshot) {
	i := sort.Search(len(r.extendedHistory), func(i int) bool {
		return r.extendedHistory[i].ID >= snapshot.ID
	})
	if i < len(r.extendedHistory) && r.extendedHistory[i].ID == snapshot.ID {
		r.extendedHistory[i] = snapshot
		return
	}
	r.extendedHistory = append(r.extendedHistory, dashboard.ExtendedSnapshot{})
	copy(r.extendedHistory[i+1:], r.extendedHistory[i:])
	r.extendedHistory[i] = snapshot
}

func (r *LiveRunner) ExtendedSnapshot(_ context.Context, selectedID int64) dashboard.ExtendedState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	state := dashboard.ExtendedState{
		Available:        true,
		WindowStart:      r.cfg.Extended.Start,
		WindowEnd:        r.cfg.Extended.End,
		AvgCloseBars:     r.cfg.Extended.AvgCloseBars,
		UpperSignalRatio: r.cfg.Extended.UpperSignalRatio,
		LowerSignalRatio: r.cfg.Extended.LowerSignalRatio,
		SoundURL:         "/api/extended/sound",
		History:          make([]dashboard.ExtendedHistoryPoint, 0, len(r.extendedHistory)),
	}
	for _, snapshot := range r.extendedHistory {
		state.History = append(state.History, dashboard.ExtendedHistoryPoint{
			ID: snapshot.ID, Clock: snapshot.Clock, Count: len(snapshot.Rows),
		})
	}
	if len(r.extendedHistory) == 0 {
		return state
	}
	state.LiveID = r.extendedHistory[len(r.extendedHistory)-1].ID
	index := len(r.extendedHistory) - 1
	if selectedID != 0 {
		i := sort.Search(len(r.extendedHistory), func(i int) bool {
			return r.extendedHistory[i].ID >= selectedID
		})
		if i < len(r.extendedHistory) && r.extendedHistory[i].ID == selectedID {
			index = i
		}
	}
	state.Selected = cloneExtendedSnapshot(r.extendedHistory[index])
	return state
}

func cloneExtendedSnapshot(src dashboard.ExtendedSnapshot) dashboard.ExtendedSnapshot {
	src.Rows = append([]dashboard.ExtendedRow(nil), src.Rows...)
	return src
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
