package gold

import (
	"testing"
	"time"
)

func TestCalendar(t *testing.T) {
	for stamp, want := range map[string]string{
		"2026-11-03 11:45": "open", "2026-11-03 15:30": "closed", "2026-11-03 20:00": "open",
		"2026-11-07 01:00": "open", "2026-11-07 10:00": "closed", "2026-11-09 01:00": "closed",
		"2026-09-30 21:00": "closed", "2026-10-01 01:00": "closed", "2026-10-06 10:00": "closed",
		"2027-01-04 10:00": "calendar_unknown",
	} {
		now, _ := time.ParseInLocation("2006-01-02 15:04", stamp, beijing)
		if got := sessionState(now); got != want {
			t.Errorf("%s: %s != %s", stamp, got, want)
		}
	}
}
