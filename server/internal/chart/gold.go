package chart

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pricereminder/server/internal/gold"
)

const sgeInstrument = "Au99.99"

type goldTimeline struct {
	Instrument string            `json:"heyue"`
	SourceTime string            `json:"delaystr"`
	Times      []string          `json:"times"`
	Prices     []json.RawMessage `json:"data"`
}

func (s *Service) fetchGold(ctx context.Context, now time.Time, previous Series) (Series, error) {
	var timeline goldTimeline
	if err := requestSGE(ctx, s.gold, "quotations", &timeline); err != nil {
		return Series{}, err
	}
	out, err := parseGold(timeline, now)
	if err != nil {
		return Series{}, err
	}
	if previous.Date == out.Date && previous.FinalClose != "" {
		out.FinalClose = previous.FinalClose
		return out, nil
	}

	// The daily table may publish after 15:30. Match its date exactly.
	local := now.In(beijing)
	if local.Format(time.DateOnly) > out.Date ||
		(local.Format(time.DateOnly) == out.Date && local.Hour()*60+local.Minute() >= 15*60+30) {
		var daily struct {
			Rows [][]json.RawMessage `json:"time"`
		}
		if requestSGE(ctx, s.gold, "Dailyhq", &daily) == nil {
			out.FinalClose = goldClose(daily.Rows, out.Date)
		}
	}
	return out, nil
}

func requestSGE(ctx context.Context, client *http.Client, route string, dest any) error {
	form := url.Values{"instid": {sgeInstrument}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.sge.com.cn/graph/"+route, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "PriceReminder/Chart")
	req.Header.Set("Referer", "https://www.sge.com.cn/sjzx/mrhq")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	return readJSON(client, req, dest)
}

func parseGold(timeline goldTimeline, now time.Time) (Series, error) {
	if timeline.Instrument != sgeInstrument || len(timeline.Times) != len(timeline.Prices) ||
		len(timeline.Times) == 0 || len(timeline.Times) > 1440 {
		return Series{}, fmt.Errorf("invalid SGE timeline")
	}
	stamp, err := time.ParseInLocation("2006年01月02日 15:04:05", timeline.SourceTime, beijing)
	if err != nil || stamp.After(now.Add(5*time.Second)) {
		return Series{}, fmt.Errorf("invalid SGE source time")
	}
	// Exchange responses contain slots for the entire session, including later times.
	minutes := make([]int, len(timeline.Times))
	anchor := -1
	for i, text := range timeline.Times {
		t, err := time.Parse("15:04", text)
		if err != nil {
			return Series{}, fmt.Errorf("invalid SGE minute")
		}
		minutes[i] = t.Hour()*60 + t.Minute()
		if minutes[i] == stamp.Hour()*60+stamp.Minute() {
			if anchor != -1 {
				return Series{}, fmt.Errorf("ambiguous SGE timestamp")
			}
			anchor = i
		}
	}
	if anchor < 0 {
		// During a recess or after closing, anchor to the nearest earlier slot
		// in the current session. Exclude the first evening slot of the next day.
		current := stamp.Hour()*60 + stamp.Minute()
		bestDistance := 24 * 60
		for i, m := range minutes {
			distance := (current - m + 24*60) % (24 * 60)
			if distance < bestDistance && distance <= 5*60 {
				bestDistance, anchor = distance, i
			}
		}
		if anchor == -1 {
			return Series{}, fmt.Errorf("SGE source time outside available session")
		}
	}
	tradingDate := dayStart(stamp)
	clockMinute := stamp.Hour()*60 + stamp.Minute()
	if clockMinute >= 20*60 || clockMinute <= 2*60+30 {
		night := tradingDate
		if clockMinute <= 2*60+30 {
			night = night.AddDate(0, 0, -1)
		}
		var ok bool
		tradingDate, ok = gold.TradingDayAfter(night)
		if !ok {
			return Series{}, fmt.Errorf("unknown SGE trading date")
		}
	}
	previous, knownPrevious := gold.TradingDayBefore(tradingDate)
	out := Series{
		Symbol: gold.Symbol, Date: tradingDate.Format(time.DateOnly), Source: "上海黄金交易所",
		UpdatedAt: stamp.UnixMilli(), IntervalMS: minute,
	}
	for i := 0; i <= anchor; i++ {
		m := minutes[i]
		day := tradingDate
		switch {
		case m >= 20*60:
			if !knownPrevious {
				continue
			}
			day = previous
		case m <= 2*60+30:
			if !knownPrevious {
				continue
			}
			day = previous.AddDate(0, 0, 1)
		case m >= 9*60 && m <= 11*60+30, m >= 13*60+30 && m <= 15*60+30:
		default:
			// SGE can include placeholder prices during the daytime recess.
			continue
		}
		p := price(timeline.Prices[i])
		ms := day.Add(time.Duration(m) * time.Minute).UnixMilli()
		if p != "" && ms <= out.UpdatedAt {
			out.Points = append(out.Points, Point{Time: ms, Close: p})
		}
	}
	if len(out.Points) == 0 {
		return Series{}, fmt.Errorf("no SGE prices")
	}
	for i, point := range out.Points {
		if point.Time > out.UpdatedAt || (i > 0 && point.Time <= out.Points[i-1].Time) {
			return Series{}, fmt.Errorf("invalid SGE timeline order")
		}
	}
	return out, nil
}

func goldClose(rows [][]json.RawMessage, date string) string {
	for _, row := range rows {
		if len(row) >= 3 && string(row[0]) == "\""+date+"\"" {
			return price(row[2])
		}
	}
	return ""
}
