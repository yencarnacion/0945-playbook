package data

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type SessionSummary struct {
	Symbol, Date, Source, Adjustment string
	Open, High, Low, Close, Volume   float64
	CreatedAt                        time.Time
}

func SummaryPath(dir, day, symbol string) string {
	return filepath.Join(dir, "prior", day, symbol+".json")
}
func SaveSummary(dir string, s SessionSummary) error {
	p := SummaryPath(dir, s.Date, s.Symbol)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0644)
}
func LoadSummary(dir, day, symbol string) (SessionSummary, error) {
	b, err := os.ReadFile(SummaryPath(dir, day, symbol))
	if err != nil {
		return SessionSummary{}, err
	}
	var s SessionSummary
	err = json.Unmarshal(b, &s)
	if err == nil && (s.Date != day || s.Symbol != symbol) {
		err = errors.New("session summary identity mismatch")
	}
	return s, err
}
func Summarize(symbol, day string, bars []Bar, loc *time.Location) (SessionSummary, bool) {
	s := SessionSummary{Symbol: symbol, Date: day, Source: "massive", Adjustment: "configured", CreatedAt: time.Now().UTC()}
	for _, b := range bars {
		bt := b.Time.In(loc)
		if bt.Format("2006-01-02") != day || bt.Hour() < 9 || bt.Hour() == 9 && bt.Minute() < 30 || bt.Hour() >= 16 {
			continue
		}
		if s.Open == 0 {
			s.Open = b.Open
			s.Low = b.Low
		}
		if b.High > s.High {
			s.High = b.High
		}
		if s.Low == 0 || b.Low < s.Low {
			s.Low = b.Low
		}
		s.Close = b.Close
		s.Volume += b.Volume
	}
	return s, s.Open > 0 && s.High > 0 && s.Low > 0 && s.Close > 0
}
func PreviousTradingDay(day time.Time) time.Time {
	d := day.AddDate(0, 0, -1)
	for isNYSEClosed(d) {
		d = d.AddDate(0, 0, -1)
	}
	return d
}
func isNYSEClosed(d time.Time) bool {
	if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		return true
	}
	y := d.Year()
	date := d.Format("01-02")
	if date == "01-01" || date == "06-19" || date == "07-04" || date == "12-25" {
		return true
	}
	if d.Month() == time.January && d.Weekday() == time.Monday && d.Day() >= 15 && d.Day() <= 21 {
		return true
	}
	if d.Month() == time.February && d.Weekday() == time.Monday && d.Day() >= 15 && d.Day() <= 21 {
		return true
	}
	if d.Month() == time.May && d.Weekday() == time.Monday && d.Day() > 24 {
		return true
	}
	if d.Month() == time.September && d.Weekday() == time.Monday && d.Day() <= 7 {
		return true
	}
	if d.Month() == time.November && d.Weekday() == time.Thursday && d.Day() >= 22 && d.Day() <= 28 {
		return true
	}
	if d.Equal(goodFriday(y, d.Location())) {
		return true
	}
	for _, fixed := range []time.Time{time.Date(y, 1, 1, 0, 0, 0, 0, d.Location()), time.Date(y, 6, 19, 0, 0, 0, 0, d.Location()), time.Date(y, 7, 4, 0, 0, 0, 0, d.Location()), time.Date(y, 12, 25, 0, 0, 0, 0, d.Location())} {
		if fixed.Weekday() == time.Saturday && d.Equal(fixed.AddDate(0, 0, -1)) || fixed.Weekday() == time.Sunday && d.Equal(fixed.AddDate(0, 0, 1)) {
			return true
		}
	}
	return false
}
func goodFriday(y int, loc *time.Location) time.Time {
	a := y % 19
	b := y / 100
	c := y % 100
	dd := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - dd - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Date(y, time.Month(month), day-2, 0, 0, 0, 0, loc)
}
