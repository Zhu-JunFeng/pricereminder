package gold

import (
	"strings"
	"testing"
	"time"
)

const sample = `var hq_str_gds_AU9999="847.00,0,847.00,847.10,849.99,834.00,11:13:05,836.08,836.00,303420,90.00,5.00,2026-11-03,沪金99";`

func TestParseSourceTime(t *testing.T) {
	now := time.Date(2026, 11, 3, 11, 13, 29, 0, beijing)
	q, err := Parse([]byte(sample), now)
	if err != nil {
		t.Fatal(err)
	}
	if q.Price != "847" || q.PreviousClose != "836.08" || q.ReceivedAt-q.EventTime != 24000 {
		t.Fatalf("%+v", q)
	}
	// Identical responses keep their source time even if fetched later.
	later, err := Parse([]byte(sample), now.Add(time.Minute))
	if err != nil || later.EventTime != q.EventTime {
		t.Fatalf("timestamp fabricated: %+v %v", later, err)
	}
}

func TestRejectInvalidQuote(t *testing.T) {
	now := time.Date(2026, 11, 3, 11, 13, 29, 0, beijing)
	for _, body := range []string{
		`var hq_str_gds_AU9999="";`, "<html>blocked</html>",
		strings.Replace(sample, "gds_AU9999", "gds_AUTD", 1),
		strings.Replace(sample, "847.00,0", "0,0", 1),
		strings.Replace(sample, "11:13:05", "23:13:05", 1),
		strings.Replace(sample, "2026-11-03", "2026-99-03", 1),
	} {
		if _, err := Parse([]byte(body), now); err == nil {
			t.Errorf("accepted invalid quote %s", body)
		}
	}
}

func TestMissingCloseKeepsQuote(t *testing.T) {
	now := time.Date(2026, 11, 3, 11, 13, 29, 0, beijing)
	q, err := Parse([]byte(strings.Replace(sample, "836.08", "0", 1)), now)
	if err != nil || q.PreviousClose != "" || q.Price != "847" {
		t.Fatal(q, err)
	}
}
