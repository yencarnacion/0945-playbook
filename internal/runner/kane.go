package runner

import (
	"sort"
	"time"

	"0945-playbook/internal/dashboard"
	"0945-playbook/internal/data"
	"0945-playbook/internal/watchlist"
)

// kaneState applies the frozen, out-of-sample rules documented in local/kane.txt.
// Sample EV is descriptive research evidence, not a forecast for this ticker.
func kaneState(items []watchlist.Item, barsBySymbol map[string][]data.Bar, now time.Time, loc *time.Location, chartBase string) dashboard.KaneState {
	openTime := sessionClock(now, loc, "09:30")
	prelim := now.Before(openTime)
	down, up := []dashboard.KaneRow{}, []dashboard.KaneRow{}
	for _, item := range items {
		bars := barsBySymbol[item.Symbol]
		var priorOpen, priorHigh, priorLow, priorClose, price, volumeFrom0400 float64
		priorDay := ""
		for _, b := range bars {
			bt := b.Time.In(loc)
			if bt.Format("2006-01-02") == now.Format("2006-01-02") && !bt.Before(sessionClock(now, loc, "04:00")) && !bt.After(now) {
				volumeFrom0400 += b.Volume
			}
			if bt.Before(openTime) && bt.Hour() >= 9 && (bt.Hour() > 9 || bt.Minute() >= 30) && bt.Hour() < 16 {
				day := bt.Format("2006-01-02")
				if day != now.Format("2006-01-02") {
					if day != priorDay {
						priorDay, priorOpen, priorHigh, priorLow = day, b.Open, b.High, b.Low
					} else {
						if b.High > priorHigh {
							priorHigh = b.High
						}
						if priorLow == 0 || b.Low < priorLow {
							priorLow = b.Low
						}
					}
					priorClose = b.Close
				}
			}
			if !bt.After(now) && bt.Format("2006-01-02") == now.Format("2006-01-02") {
				price = b.Close
			}
			if !prelim && !bt.Before(openTime) && bt.Before(openTime.Add(time.Minute)) {
				price = b.Open
				break
			}
		}
		_ = priorOpen
		if price < 3 || priorClose <= 0 || item.ATR14 <= 0 || priorHigh <= priorLow {
			continue
		}
		gap := price/priorClose - 1
		if gap < -.50 || gap > .50 {
			continue
		}
		gapATR := (price - priorClose) / item.ATR14
		locClose := (priorClose - priorLow) / (priorHigh - priorLow)
		base := dashboard.KaneRow{Symbol: item.Symbol, Name: item.Name, Industry: item.Industry, Preliminary: prelim, Price: price, PriorClose: priorClose, GapPct: gap, GapATR: gapATR, PriorCloseLocation: locClose, VolumeFrom0400: volumeFrom0400, ChartURL: chartURL(chartBase, item.Symbol, now.Format("2006-01-02"), chartClock(now), 1)}
		if gap <= -.10 && gap >= -.25 && -gapATR >= 1.5 && -gapATR <= 2.5 {
			base.Setup = "Gap-down reversal"
			base.SampleEV = .0089
			base.WinRate = .489
			base.TargetPct = .06
			base.StopPct = .04
			base.Reason = "Primary: 10–25% down and 1.5–2.5 ATR; largest gap ranks first"
			if locClose >= .80 {
				base.Reason += "; prior close was in top 20% (selective confirmation)"
			}
			down = append(down, base)
		} else if gap >= .18 && gap <= .35 && price >= 10 && price <= 50 {
			base.Setup = "Gap-up continuation"
			base.SampleEV = .016
			base.WinRate = .44
			base.TargetPct = .10
			base.StopPct = .05
			base.Reason = "Secondary: $10–$50 and 18–35% up; smaller validation sample"
			up = append(up, base)
		}
	}
	sort.SliceStable(down, func(i, j int) bool { return down[i].GapPct < down[j].GapPct })
	sort.SliceStable(up, func(i, j int) bool { return up[i].GapPct > up[j].GapPct })
	for i := range down {
		down[i].Rank = i + 1
		down[i].Preferred = i == 0
	}
	for i := range up {
		up[i].Rank = i + 1
		up[i].Preferred = i == 0
	}
	rows := append(down, up...)
	return dashboard.KaneState{Available: true, Preliminary: prelim, Rows: rows}
}
