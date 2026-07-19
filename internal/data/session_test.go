package data

import (
	"testing"
	"time"
)

func TestPreviousTradingDayWeekendHolidayAndEarlyClose(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	tests := []struct{ day, want string }{{"2026-07-06", "2026-07-02"}, {"2026-12-28", "2026-12-24"}, {"2026-11-30", "2026-11-27"}}
	for _, tc := range tests {
		d, _ := time.ParseInLocation("2006-01-02", tc.day, ny)
		if got := PreviousTradingDay(d).Format("2006-01-02"); got != tc.want {
			t.Fatalf("previous(%s)=%s want %s", tc.day, got, tc.want)
		}
	}
}
