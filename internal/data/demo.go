package data

import (
	"context"
	"hash/fnv"
	"math"
	"time"
)

type DemoProvider struct {
	Loc *time.Location
}

func (p DemoProvider) FetchBars(_ context.Context, symbol string, start, end time.Time) ([]Bar, error) {
	loc := p.Loc
	if loc == nil {
		loc = time.Local
	}
	seed := hashSymbol(symbol)
	day := start.In(loc)
	open := time.Date(day.Year(), day.Month(), day.Day(), 9, 30, 0, 0, loc)
	closeTime := time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, loc)
	if end.After(closeTime) {
		end = closeTime
	}
	if end.Before(open) {
		end = open.Add(20 * time.Minute)
	}

	base := 8 + float64(seed%9000)/100
	drift := (float64(int(seed%11)-5) / 10000)
	amp := 0.004 + float64(seed%17)/2500
	volumeBase := 8000 + float64(seed%40000)

	out := make([]Bar, 0, 390)
	last := base
	for t := open; !t.After(end); t = t.Add(time.Minute) {
		i := t.Sub(open).Minutes()
		wave := math.Sin(i/4+float64(seed%13)) * amp
		shock := 0.0
		if i >= 12 && i <= 16 {
			switch seed % 5 {
			case 0:
				shock = 0.035
			case 1:
				shock = -0.035
			case 2:
				shock = 0.065
			case 3:
				shock = -0.055
			default:
				shock = 0.008
			}
		}
		close := base * (1 + drift*i + wave + shock)
		openPx := last
		high := math.Max(openPx, close) * (1 + 0.002 + float64(seed%7)/10000)
		low := math.Min(openPx, close) * (1 - 0.002 - float64(seed%5)/10000)
		vol := volumeBase * (1 + math.Abs(shock)*20)
		if i < 15 {
			vol *= 3
		}
		out = append(out, Bar{
			Time:   t.UTC(),
			Open:   openPx,
			High:   high,
			Low:    low,
			Close:  close,
			Volume: vol,
			VWAP:   (high + low + close) / 3,
		})
		last = close
	}
	return out, nil
}

func hashSymbol(symbol string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(symbol))
	return h.Sum32()
}
