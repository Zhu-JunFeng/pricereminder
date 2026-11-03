package gold

import "time"

// Calendar source: https://sge.com.cn/jjsnotice/10007108 (2026).
// Unknown years fail closed until the exchange calendar is updated.
func sessionState(now time.Time) string {
	t := now.In(beijing)
	if t.Year() != 2026 {
		return "calendar_unknown"
	}
	minute := t.Hour()*60 + t.Minute()
	day := t
	night := false
	switch {
	case minute >= 9*60 && minute < 15*60+30:
	case minute >= 20*60:
		night = true
	case minute < 2*60+30:
		night = true
		day = t.AddDate(0, 0, -1)
	default:
		return "closed"
	}
	if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
		return "closed"
	}
	date := day.Format("2006-01-02")
	holidays := [][2]string{{"2026-01-01", "2026-01-03"}, {"2026-02-15", "2026-02-23"}, {"2026-04-04", "2026-04-06"}, {"2026-05-01", "2026-05-05"}, {"2026-06-19", "2026-06-21"}, {"2026-09-25", "2026-09-27"}, {"2026-10-01", "2026-10-07"}}
	for _, h := range holidays {
		if date >= h[0] && date <= h[1] {
			return "closed"
		}
	}
	if night {
		for _, d := range []string{"2025-12-31", "2026-02-13", "2026-04-03", "2026-04-30", "2026-06-18", "2026-09-24", "2026-09-30"} {
			if date == d {
				return "closed"
			}
		}
	}
	return "open"
}
